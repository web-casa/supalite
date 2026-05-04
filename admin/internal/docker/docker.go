package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	http    *http.Client
	// stream is a separate client without a read timeout, used for
	// long-lived follows (log tails). Shares the same unix-socket transport.
	stream  *http.Client
	project string
}

type ContainerInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Status string `json:"status"`
}

func NewClient(project string) *Client {
	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", "/var/run/docker.sock")
		},
	}
	return &Client{
		http:    &http.Client{Transport: transport, Timeout: 30 * time.Second},
		stream:  &http.Client{Transport: transport}, // no timeout; caller cancels via context
		project: project,
	}
}

type containerJSON struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
}

func (c *Client) ListContainers() ([]ContainerInfo, error) {
	resp, err := c.http.Get("http://docker/containers/json?all=true")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var containers []containerJSON
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, err
	}

	var result []ContainerInfo
	for _, ct := range containers {
		if ct.Labels["com.docker.compose.project"] != c.project {
			continue
		}
		name := ct.Labels["com.docker.compose.service"]
		if name == "" && len(ct.Names) > 0 {
			name = strings.TrimPrefix(ct.Names[0], "/")
		}
		result = append(result, ContainerInfo{
			ID:     ct.ID[:12],
			Name:   name,
			State:  ct.State,
			Status: ct.Status,
		})
	}
	return result, nil
}

func (c *Client) FindContainerID(service string) (string, error) {
	resp, err := c.http.Get("http://docker/containers/json?all=true")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var containers []containerJSON
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return "", err
	}

	for _, ct := range containers {
		if ct.Labels["com.docker.compose.project"] == c.project &&
			ct.Labels["com.docker.compose.service"] == service {
			return ct.ID, nil
		}
	}
	return "", fmt.Errorf("container not found: %s", service)
}

// InspectContainer returns the full container JSON. Used by restore
// orchestration to read the db container's Image, Mounts, and
// Networks so the one-shot restore container gets the same config.
func (c *Client) InspectContainer(ctx context.Context, id string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://docker/containers/"+id+"/json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("inspect failed (%d): %s", resp.StatusCode, b)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// StopContainer sends SIGTERM, then SIGKILL after timeoutSeconds if
// the container hasn't stopped.
//
// Uses the timeoutless `c.stream` client because Docker blocks the
// HTTP response until the stop completes — which can legitimately
// take up to `timeoutSeconds`. The 30s ceiling on `c.http` would
// return a premature client error for any stop > 30s, even though
// the daemon continues successfully.
func (c *Client) StopContainer(ctx context.Context, id string, timeoutSeconds int) error {
	urlStr := fmt.Sprintf("http://docker/containers/%s/stop?t=%d", id, timeoutSeconds)
	req, err := http.NewRequestWithContext(ctx, "POST", urlStr, nil)
	if err != nil {
		return err
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 204 = stopped, 304 = already stopped. Both are OK.
	if resp.StatusCode >= 300 && resp.StatusCode != 304 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stop failed (%d): %s", resp.StatusCode, body)
	}
	return nil
}

// ListContainersByLabel returns container IDs with the given label=value,
// including stopped containers. Used to detect orphaned one-shots after
// admin restart.
func (c *Client) ListContainersByLabel(ctx context.Context, key, value string) ([]string, error) {
	filter := fmt.Sprintf(`{"label":["%s=%s"]}`, key, value)
	q := url.Values{}
	q.Set("all", "true")
	q.Set("filters", filter)
	reqURL := "http://docker/containers/json?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list-by-label failed (%d): %s", resp.StatusCode, b)
	}
	var out []containerJSON
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out))
	for _, ct := range out {
		ids = append(ids, ct.ID)
	}
	return ids, nil
}

// StartContainer starts an existing stopped container.
func (c *Client) StartContainer(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", "http://docker/containers/"+id+"/start", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != 304 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("start failed (%d): %s", resp.StatusCode, body)
	}
	return nil
}

// OneShotConfig is the input to RunOneShot — mirrors the subset of
// `docker run --rm` flags we need for orchestrated restores.
type OneShotConfig struct {
	Image       string
	Cmd         []string
	Entrypoint  []string          // nil = use image's default entrypoint
	Env         []string          // "KEY=value" pairs
	Binds       []string          // "volume_or_host_path:container_path[:ro]"
	NetworkMode string            // e.g. "supalite_default"
	Labels      map[string]string // attached to container for orphan detection
	MaxLogBytes int               // tail cap per stream; default 256 KiB if 0
}

// RunOneShot creates, starts, waits for, collects logs from, and
// removes a container. Analogous to `docker run --rm`. Returns the
// container's stdout/stderr + exit code.
//
// We always remove the container on return, even on error paths,
// so the daemon doesn't accumulate dead one-shots.
func (c *Client) RunOneShot(ctx context.Context, cfg OneShotConfig) (*ExecResult, error) {
	maxBytes := cfg.MaxLogBytes
	if maxBytes <= 0 {
		maxBytes = 256 * 1024
	}

	// 1. Create.
	createBody := map[string]any{
		"Image":        cfg.Image,
		"Cmd":          cfg.Cmd,
		"AttachStdout": true,
		"AttachStderr": true,
		"HostConfig": map[string]any{
			"Binds":         cfg.Binds,
			"NetworkMode":   cfg.NetworkMode,
			"AutoRemove":    false, // we delete explicitly so we can fetch logs first
			"RestartPolicy": map[string]any{"Name": "no"},
		},
	}
	if len(cfg.Entrypoint) > 0 {
		createBody["Entrypoint"] = cfg.Entrypoint
	}
	if len(cfg.Env) > 0 {
		createBody["Env"] = cfg.Env
	}
	if len(cfg.Labels) > 0 {
		createBody["Labels"] = cfg.Labels
	}
	bodyJSON, _ := json.Marshal(createBody)
	createReq, err := http.NewRequestWithContext(ctx, "POST", "http://docker/containers/create", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := c.http.Do(createReq)
	if err != nil {
		return nil, err
	}
	defer createResp.Body.Close()
	if createResp.StatusCode >= 300 {
		b, _ := io.ReadAll(createResp.Body)
		return nil, fmt.Errorf("one-shot create failed (%d): %s", createResp.StatusCode, b)
	}
	var created struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		return nil, err
	}

	// Guaranteed cleanup — force-remove the container on return,
	// regardless of success/failure. Uses a background ctx so it
	// still runs if the parent ctx was canceled. A failed remove
	// leaves a running container — log loudly so orphan detection
	// (via ListContainersByLabel) can catch it.
	defer func() {
		rmCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		rmReq, _ := http.NewRequestWithContext(rmCtx, "DELETE", "http://docker/containers/"+created.ID+"?force=true", nil)
		rmResp, rmErr := c.http.Do(rmReq)
		if rmErr != nil {
			log.Printf("[docker] one-shot %s force-remove failed: %v (orphan may remain)", created.ID[:12], rmErr)
			return
		}
		defer rmResp.Body.Close()
		if rmResp.StatusCode >= 300 && rmResp.StatusCode != 404 {
			body, _ := io.ReadAll(rmResp.Body)
			log.Printf("[docker] one-shot %s force-remove HTTP %d: %s (orphan may remain)",
				created.ID[:12], rmResp.StatusCode, body)
		}
	}()

	// 2. Start.
	startReq, err := http.NewRequestWithContext(ctx, "POST", "http://docker/containers/"+created.ID+"/start", nil)
	if err != nil {
		return nil, err
	}
	startResp, err := c.http.Do(startReq)
	if err != nil {
		return nil, err
	}
	startResp.Body.Close()
	if startResp.StatusCode >= 300 && startResp.StatusCode != 304 {
		return nil, fmt.Errorf("one-shot start failed (%d)", startResp.StatusCode)
	}

	// 3. Wait for exit (uses the timeoutless stream client).
	waitReq, err := http.NewRequestWithContext(ctx, "POST", "http://docker/containers/"+created.ID+"/wait", nil)
	if err != nil {
		return nil, err
	}
	waitResp, err := c.stream.Do(waitReq)
	if err != nil {
		return nil, err
	}
	defer waitResp.Body.Close()
	if waitResp.StatusCode >= 300 {
		b, _ := io.ReadAll(waitResp.Body)
		return nil, fmt.Errorf("one-shot wait failed (%d): %s", waitResp.StatusCode, b)
	}
	var waitOut struct {
		StatusCode int `json:"StatusCode"`
	}
	if err := json.NewDecoder(waitResp.Body).Decode(&waitOut); err != nil {
		return nil, err
	}

	// 4. Collect logs (non-streaming; now that the container has exited,
	// fetch stdout+stderr with reasonable caps).
	logsReq, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("http://docker/containers/%s/logs?stdout=true&stderr=true&tail=2000", created.ID), nil)
	if err != nil {
		return nil, err
	}
	logsResp, err := c.http.Do(logsReq)
	if err != nil {
		return nil, err
	}
	defer logsResp.Body.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	if err := readMultiplexed(logsResp.Body, &stdoutBuf, &stderrBuf, maxBytes); err != nil {
		return nil, fmt.Errorf("read logs: %w", err)
	}

	return &ExecResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: waitOut.StatusCode,
	}, nil
}

// readMultiplexed demuxes Docker's 8-byte-header frame format into
// stdout/stderr buffers, tail-biased capped at maxBytes per stream.
// Shared between Exec() and RunOneShot().
func readMultiplexed(src io.Reader, stdout, stderr *bytes.Buffer, maxBytes int) error {
	trimTail := func(buf *bytes.Buffer) {
		if buf.Len() <= 2*maxBytes {
			return
		}
		tail := append([]byte{}, buf.Bytes()[buf.Len()-maxBytes:]...)
		buf.Reset()
		buf.Write(tail)
	}
	var header [8]byte
	for {
		if _, err := io.ReadFull(src, header[:]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return err
		}
		size := int(binary.BigEndian.Uint32(header[4:8]))
		if size == 0 {
			continue
		}
		var dst *bytes.Buffer
		switch header[0] {
		case 1:
			dst = stdout
		case 2:
			dst = stderr
		default:
			if _, err := io.CopyN(io.Discard, src, int64(size)); err != nil {
				return err
			}
			continue
		}
		if _, err := io.CopyN(dst, src, int64(size)); err != nil {
			return err
		}
		trimTail(dst)
	}
	for _, b := range []*bytes.Buffer{stdout, stderr} {
		if b.Len() > maxBytes {
			tail := append([]byte{}, b.Bytes()[b.Len()-maxBytes:]...)
			b.Reset()
			b.Write(tail)
		}
	}
	return nil
}

func (c *Client) RestartContainer(id string) error {
	req, _ := http.NewRequest("POST", "http://docker/containers/"+id+"/restart?t=10", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("restart failed (%d): %s", resp.StatusCode, body)
	}
	return nil
}

func (c *Client) GetLogs(containerID string, lines int) (string, error) {
	url := fmt.Sprintf("http://docker/containers/%s/logs?stdout=true&stderr=true&tail=%d", containerID, lines)
	resp, err := c.http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("logs failed (%d): %s", resp.StatusCode, body)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return stripDockerLogHeaders(raw), nil
}

// ExecResult is the outcome of an exec run: captured stdout and
// stderr (demuxed from Docker's multiplexed stream) plus the
// process's exit code.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Exec runs a command inside a container identified by compose
// service name. Returns stdout, stderr, and exit code. Output is
// capped at maxBytes per stream to bound memory.
//
// Intended for short administrative commands (pgbackrest stanza-create,
// pgbackrest info, etc.). For long-running operations, use ExecStream.
func (c *Client) Exec(ctx context.Context, service string, cmd []string, maxBytes int) (*ExecResult, error) {
	id, err := c.FindContainerID(service)
	if err != nil {
		return nil, err
	}

	// 1. Create exec instance.
	createReq := struct {
		AttachStdout bool     `json:"AttachStdout"`
		AttachStderr bool     `json:"AttachStderr"`
		Cmd          []string `json:"Cmd"`
		User         string   `json:"User,omitempty"`
	}{AttachStdout: true, AttachStderr: true, Cmd: cmd}
	body, _ := json.Marshal(createReq)
	req, err := http.NewRequestWithContext(ctx, "POST", "http://docker/containers/"+id+"/exec", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("exec create failed (%d): %s", resp.StatusCode, b)
	}
	var created struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, err
	}

	// 2. Start + stream. Not detached, Tty=false → multiplexed stdout/stderr.
	startBody, _ := json.Marshal(struct {
		Detach bool `json:"Detach"`
		Tty    bool `json:"Tty"`
	}{Detach: false, Tty: false})
	startReq, err := http.NewRequestWithContext(ctx, "POST", "http://docker/exec/"+created.ID+"/start", bytes.NewReader(startBody))
	if err != nil {
		return nil, err
	}
	startReq.Header.Set("Content-Type", "application/json")
	startResp, err := c.stream.Do(startReq)
	if err != nil {
		return nil, err
	}
	defer startResp.Body.Close()
	// Surface Docker API errors BEFORE treating the body as multiplex
	// frames — otherwise an error JSON gets parsed as a frame header.
	if startResp.StatusCode >= 300 {
		b, _ := io.ReadAll(startResp.Body)
		return nil, fmt.Errorf("exec start failed (%d): %s", startResp.StatusCode, b)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	if err := readMultiplexed(startResp.Body, &stdoutBuf, &stderrBuf, maxBytes); err != nil {
		return nil, err
	}

	// 3. Inspect exec to pick up ExitCode. Docker sometimes reports
	// Running=true for a brief window after the attached stream closes;
	// poll a handful of times before giving up so callers don't see a
	// bogus ExitCode=0 for a still-running process.
	var inspect struct {
		ExitCode int  `json:"ExitCode"`
		Running  bool `json:"Running"`
	}
	for attempt := 0; attempt < 20; attempt++ {
		inspectReq, err := http.NewRequestWithContext(ctx, "GET", "http://docker/exec/"+created.ID+"/json", nil)
		if err != nil {
			return nil, err
		}
		inspectResp, err := c.http.Do(inspectReq)
		if err != nil {
			return nil, err
		}
		if err := json.NewDecoder(inspectResp.Body).Decode(&inspect); err != nil {
			inspectResp.Body.Close()
			return nil, err
		}
		inspectResp.Body.Close()
		if !inspect.Running {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if inspect.Running {
		return nil, fmt.Errorf("exec still running after stream closed")
	}

	return &ExecResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: inspect.ExitCode,
	}, nil
}

// StreamLogs opens a follow=true log tail. Caller must Close the
// returned reader when done (or cancel ctx) to release the connection.
// The response body is raw Docker multiplex frames — wrap with
// NewLogFrameReader to get demuxed line-oriented output.
func (c *Client) StreamLogs(ctx context.Context, containerID string, tail int) (io.ReadCloser, error) {
	url := fmt.Sprintf("http://docker/containers/%s/logs?stdout=true&stderr=true&follow=true&tail=%d", containerID, tail)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("logs follow failed (%d): %s", resp.StatusCode, body)
	}
	return resp.Body, nil
}

// LogFrameReader decodes Docker's multiplexed log stream, producing
// the raw stdout/stderr bytes without the 8-byte frame headers.
type LogFrameReader struct {
	src io.Reader
	buf []byte // pending payload bytes not yet returned to caller
}

func NewLogFrameReader(src io.Reader) *LogFrameReader {
	return &LogFrameReader{src: src}
}

func (r *LogFrameReader) Read(p []byte) (int, error) {
	if len(r.buf) > 0 {
		n := copy(p, r.buf)
		r.buf = r.buf[n:]
		return n, nil
	}
	var header [8]byte
	if _, err := io.ReadFull(r.src, header[:]); err != nil {
		return 0, err
	}
	size := binary.BigEndian.Uint32(header[4:8])
	if size == 0 {
		return 0, nil
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r.src, payload); err != nil {
		return 0, err
	}
	n := copy(p, payload)
	if n < len(payload) {
		r.buf = payload[n:]
	}
	return n, nil
}

// Docker multiplexed stream: each frame has 8-byte header [type(1) + padding(3) + size(4)]
func stripDockerLogHeaders(data []byte) string {
	var sb strings.Builder
	for len(data) >= 8 {
		size := binary.BigEndian.Uint32(data[4:8])
		data = data[8:]
		if int(size) > len(data) {
			sb.Write(data)
			break
		}
		sb.Write(data[:size])
		data = data[size:]
	}
	if sb.Len() == 0 {
		return string(data)
	}
	return sb.String()
}
