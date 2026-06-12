package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/qcore-project/qcore/pkg/events"
)

func TestFetchJourneyTraceUsesCollectorJourneyRoute(t *testing.T) {
	const journeyID = "j-test"
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/journeys/"+journeyID+"/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]events.Event{{
			JourneyID: journeyID,
			NF:        "simulator",
			Category:  events.ErrorEvent,
			Severity:  events.SeverityError,
			Message:   "Scenario wrong_ki failed at security_mode_command: read timed out after 3s",
		}})
	}))
	defer collector.Close()

	collectorURL, err := url.Parse(collector.URL)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{collectorURL: collectorURL}
	trace, err := s.fetchJourneyTrace(httptest.NewRequest(http.MethodGet, "/", nil), journeyID)
	if err != nil {
		t.Fatalf("fetchJourneyTrace returned error: %v", err)
	}
	if len(trace) != 1 {
		t.Fatalf("trace length = %d, want 1", len(trace))
	}
	if trace[0].JourneyID != journeyID {
		t.Fatalf("journey id = %q, want %q", trace[0].JourneyID, journeyID)
	}
}
