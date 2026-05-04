package handler

import (
	"bufio"
	"net/http"
	"strconv"
	"time"

	"github.com/supalite/admin/internal/docker"
	"github.com/supalite/admin/internal/sse"
)

var allowedServices = map[string]bool{
	"db": true, "rest": true, "gotrue": true, "gateway": true, "admin": true,
}

func (d *Deps) HandleLogs(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	if !allowedServices[service] {
		writeError(w, 400, "invalid service name")
		return
	}
	lines := 100
	if n, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && n > 0 && n <= 1000 {
		lines = n
	}

	id, err := d.Docker.FindContainerID(service)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	logs, err := d.Docker.GetLogs(id, lines)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"logs": logs})
}

// HandleLogsStream opens an SSE connection that tails a service's
// container logs in real time. Client disconnect is detected via the
// request context; cancelling it closes the underlying Docker stream.
func (d *Deps) HandleLogsStream(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	if !allowedServices[service] {
		writeError(w, 400, "invalid service name")
		return
	}
	tail := 100
	if n, err := strconv.Atoi(r.URL.Query().Get("tail")); err == nil && n >= 0 && n <= 1000 {
		tail = n
	}

	id, err := d.Docker.FindContainerID(service)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}

	body, err := d.Docker.StreamLogs(r.Context(), id, tail)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer body.Close()

	stream, err := sse.Start(w, r)
	if err != nil {
		writeError(w, 500, "streaming unsupported")
		return
	}
	defer stream.Close()

	// Scanner reads demuxed payload bytes and splits on newlines.
	// Bump max line size to 1MB for noisy log entries.
	scanner := bufio.NewScanner(docker.NewLogFrameReader(body))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	lines := make(chan string, 256)
	go func() {
		defer close(lines)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-r.Context().Done():
				return
			}
		}
	}()

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if err := stream.Ping(); err != nil {
				return
			}
		case line, ok := <-lines:
			if !ok {
				return
			}
			if err := stream.Send("log", line); err != nil {
				return
			}
		}
	}
}
