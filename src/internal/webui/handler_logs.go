package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// @sk-task kvn-web#T2.3: SSE log streaming (AC-003)
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// send current status
	statusJSON, _ := json.Marshal(map[string]string{"status": string(s.state.Status())})
	fmt.Fprintf(w, "event: status\ndata: %s\n\n", statusJSON)
	flusher.Flush()

	logCh := s.state.Subscribe()
	defer s.state.Unsubscribe(logCh)

	statusCh := s.state.SubscribeStatus()
	defer s.state.UnsubscribeStatus(statusCh)

	for {
		select {
		case <-r.Context().Done():
			return
		case entry, ok := <-logCh:
			if !ok {
				return
			}
			data, _ := json.Marshal(entry)
			fmt.Fprintf(w, "event: log\ndata: %s\n\n", data)
			flusher.Flush()
		case st, ok := <-statusCh:
			if !ok {
				return
			}
			data, _ := json.Marshal(map[string]string{"status": string(st)})
			fmt.Fprintf(w, "event: status\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}
