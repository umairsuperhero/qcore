package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// NFStatus is one network function's reported state. We deliberately keep
// this small — the dashboard's job is to surface "is the core up?", not to
// inspect every NF's internals.
type NFStatus struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	Reachable bool   `json:"reachable"`
	Status    string `json:"status,omitempty"` // body of upstream /health, if any
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// CoreHealth is the aggregate over all NFs.
type CoreHealth struct {
	AllUp       bool       `json:"all_up"`
	CheckedAt   time.Time  `json:"checked_at"`
	NFs         []NFStatus `json:"nfs"`
	ReadyForUse bool       `json:"ready_for_use"` // alias of AllUp; named for frontend copy
}

// handleHealth probes every NF in parallel and returns a combined status.
// Total latency is bounded by the slowest NF, plus a 2s ceiling per probe.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	targets := []struct {
		name string
		base string
		path string
	}{
		{"hss", s.cfg.Dashboard.HSSURL, "/api/v1/health"},
		{"mme", s.cfg.Dashboard.MMEURL, "/api/v1/health"},
		{"spgw", s.cfg.Dashboard.SPGWURL, "/api/v1/health"},
		{"collector", s.cfg.Dashboard.CollectorURL, "/health"},
	}

	statuses := make([]NFStatus, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, name, base, path string) {
			defer wg.Done()
			statuses[i] = probe(r.Context(), name, base, path)
		}(i, t.name, t.base, t.path)
	}
	wg.Wait()

	allUp := true
	for _, st := range statuses {
		if !st.Reachable {
			allUp = false
			break
		}
	}

	out := CoreHealth{
		AllUp:       allUp,
		ReadyForUse: allUp,
		CheckedAt:   time.Now().UTC(),
		NFs:         statuses,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func probe(parentCtx context.Context, name, base, path string) NFStatus {
	st := NFStatus{Name: name, URL: base + path}
	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, st.URL, nil)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	st.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		st.Error = err.Error()
		return st
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		st.Reachable = true
		st.Status = http.StatusText(resp.StatusCode)
	} else {
		st.Error = http.StatusText(resp.StatusCode)
	}
	return st
}
