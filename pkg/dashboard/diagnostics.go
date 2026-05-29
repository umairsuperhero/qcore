package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/qcore-project/qcore/pkg/events"
)

// handleDiagnoseJourney proxies the request to the collector to fetch the journey
// trace, feeds it into the AI engine, and returns a DiagnosticResult.
func (s *Server) handleDiagnoseJourney(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	journeyID := vars["id"]
	if journeyID == "" {
		http.Error(w, "missing journey ID", http.StatusBadRequest)
		return
	}

	// Fetch trace from collector
	trace, err := s.fetchJourneyTrace(r, journeyID)
	if err != nil {
		s.log.Errorf("Failed to fetch trace for journey %s: %v", journeyID, err)
		http.Error(w, "failed to fetch trace from collector", http.StatusInternalServerError)
		return
	}

	if len(trace) == 0 {
		http.Error(w, "journey trace is empty or not found", http.StatusNotFound)
		return
	}

	// Pass to AI Engine
	ctx := r.Context()
	result, err := s.aiEngine.Diagnose(ctx, trace)
	if err != nil {
		s.log.Errorf("AI engine failed to diagnose journey %s: %v", journeyID, err)
		http.Error(w, "ai diagnosis failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) fetchJourneyTrace(r *http.Request, journeyID string) ([]events.Event, error) {
	url := fmt.Sprintf("%s/api/v1/journeys/%s/events", s.collectorURL.String(), journeyID)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("collector returned status %d", resp.StatusCode)
	}

	var trace []events.Event
	if err := json.NewDecoder(resp.Body).Decode(&trace); err != nil {
		return nil, err
	}

	return trace, nil
}
