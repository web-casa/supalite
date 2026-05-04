package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/supalite/admin/internal/sse"
)

func (d *Deps) HandleStatus(w http.ResponseWriter, r *http.Request) {
	containers, err := d.Docker.ListContainers()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"services": containers})
}

// HandleStatusStream pushes a `snapshot` SSE event whenever the set of
// containers or their states changes. Polls Docker every 2s (compose
// doesn't expose a state-change event stream, and hooking the Docker
// events API would be heavier for little benefit at this scale).
func (d *Deps) HandleStatusStream(w http.ResponseWriter, r *http.Request) {
	stream, err := sse.Start(w, r)
	if err != nil {
		writeError(w, 500, "streaming unsupported")
		return
	}
	defer stream.Close()

	var lastPayload string
	send := func() bool {
		containers, err := d.Docker.ListContainers()
		if err != nil {
			// Transient Docker errors — emit as event so the UI can surface
			// them, but don't tear down the stream.
			return stream.Send("error", err.Error()) == nil
		}
		buf, _ := json.Marshal(map[string]any{"services": containers})
		if string(buf) == lastPayload {
			return true
		}
		lastPayload = string(buf)
		return stream.Send("snapshot", string(buf)) == nil
	}

	if !send() {
		return
	}

	poll := time.NewTicker(2 * time.Second)
	defer poll.Stop()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
			if !send() {
				return
			}
		case <-ping.C:
			if err := stream.Ping(); err != nil {
				return
			}
		}
	}
}
