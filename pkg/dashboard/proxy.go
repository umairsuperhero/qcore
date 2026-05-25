package dashboard

import (
	"context"
	"io"
	"net/http"

	"github.com/gorilla/mux"
)

// handleEventStream proxies the collector's SSE stream untransformed. We
// deliberately do not parse/reformat events here so the same wire shape
// feeds the React UI and (later) the Phase C diagnostic layer.
func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	upstream := s.collectorURL.String() + "/events/stream"

	// SSE has no natural timeout; tie the upstream request lifetime to the
	// browser's connection so closing the tab cleans up everything.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream, nil)
	if err != nil {
		http.Error(w, "build upstream request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "collector unreachable: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Mirror SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(resp.StatusCode)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			flusher.Flush()
		}
		if err == io.EOF || err != nil {
			return
		}
	}
}

func (s *Server) handleJourneyList(w http.ResponseWriter, r *http.Request) {
	s.proxyOnce(w, r, s.collectorURL.String()+"/journeys")
}

func (s *Server) handleJourneyEvents(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	s.proxyOnce(w, r, s.collectorURL.String()+"/journeys/"+id+"/events")
}

// proxyOnce is a single-shot GET proxy for simple JSON endpoints. Used for
// the journey-query endpoints where we don't need to keep a connection open.
func (s *Server) proxyOnce(w http.ResponseWriter, r *http.Request, upstream string) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstream, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "upstream unavailable: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
