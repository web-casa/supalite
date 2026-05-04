package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"
)

// RunPgDump streams `pg_dump -Fc` of dbURL into the S3 bucket as
// objectName. Used by both the on-demand admin handler and the
// scheduled-backup runner.
//
// On any failure the partial S3 object is best-effort deleted; if that
// delete itself fails, its error is appended to the returned error so
// the caller can surface it.
//
// Returns the wall-clock duration of the operation. A pgDumpTimeout-style
// ceiling should be enforced by the CALLER's ctx — this function does
// not impose its own timeout.
func RunPgDump(ctx context.Context, dbURL string, client *Client, objectName string) (time.Duration, error) {
	cmdCtx, cancelCmd := context.WithCancel(ctx)
	defer cancelCmd()

	cmd := exec.CommandContext(cmdCtx, "pg_dump",
		"-d", dbURL,
		"-Fc",
		"--no-owner",
		"--no-acl",
		"-f", "-",
	)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return 0, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("pg_dump start: %w", err)
	}

	var stderrBuf bytes.Buffer
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&stderrBuf, io.LimitReader(stderr, 64*1024))
		_, _ = io.Copy(io.Discard, stderr)
		close(stderrDone)
	}()

	start := time.Now()
	uploadErr := client.Upload(ctx, objectName, stdout)
	if uploadErr != nil {
		// Kill pg_dump to unblock its stdout-writer goroutine.
		cancelCmd()
	}
	waitErr := cmd.Wait()
	<-stderrDone
	elapsed := time.Since(start)

	if uploadErr == nil && waitErr == nil {
		return elapsed, nil
	}

	// Failure path: attempt cleanup with a background ctx so the delete
	// survives even if our parent ctx is already canceled.
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cleanupCancel()
	delErr := client.Delete(cleanupCtx, objectName)

	stderrTail := tailString(stderrBuf.String(), 4096)
	var msg string
	switch {
	case waitErr != nil && uploadErr != nil:
		msg = fmt.Sprintf("pg_dump failed (%v) and upload failed (%v)\n%s", waitErr, uploadErr, stderrTail)
	case waitErr != nil:
		msg = fmt.Sprintf("pg_dump failed: %v\n%s", waitErr, stderrTail)
	default:
		msg = fmt.Sprintf("upload failed: %v", uploadErr)
	}
	if delErr != nil {
		msg += fmt.Sprintf("\n(cleanup of %q also failed: %v — delete manually)", objectName, delErr)
	}
	return elapsed, fmt.Errorf("%s", msg)
}

func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
