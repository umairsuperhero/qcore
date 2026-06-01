package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
)

// genericErrorTrace is a trace that contains a real failure but matches no
// catalog rule, so the engine must escalate to the configured backend.
func genericErrorTrace() []events.Event {
	return []events.Event{
		{
			NF:       "amf",
			Category: events.SignalingRx,
			Severity: events.SeverityInfo,
			Protocol: "ngap",
			Message:  "NGSetupRequest received from gNB",
		},
		{
			NF:       "amf",
			Category: events.ErrorEvent,
			Severity: events.SeverityError,
			Protocol: "ngap",
			Message:  "NGSetupFailure: unknown PLMN in supported TA list",
		},
	}
}

// TestB2_LocalSLMEscalation verifies that when ai.provider == "local", the
// engine builds the grounded prompt, calls the OpenAI-compatible endpoint, and
// surfaces a structured Explanation/RootCause/Fix — fully offline, no API key.
func TestB2_LocalSLMEscalation(t *testing.T) {
	var gotPrompt string
	var gotModel string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("unexpected path %q, want .../chat/completions", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req openAIChatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("request was not valid OpenAI chat JSON: %v", err)
		}
		gotModel = req.Model
		// The trace must reach the model — that is what "grounded" means.
		for _, m := range req.Messages {
			if m.Role == "user" {
				gotPrompt = m.Content
			}
		}

		resp := openAIChatResponse{}
		resp.Choices = append(resp.Choices, struct {
			Message openAIChatMessage `json:"message"`
		}{Message: openAIChatMessage{
			Role: "assistant",
			Content: `{"explanation":"The gNB's NGSetupRequest was rejected because its PLMN is not in the AMF's supported list.",` +
				`"root_cause":"PLMN mismatch between gNB configuration and AMF supported TA list.",` +
				`"fix":"Set the gNB PLMN (MCC/MNC) to match amf.plmn, or add the gNB's PLMN to the AMF's supported TAs."}`,
		}})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	eng := NewEngine(config.AIConfig{
		Provider: "local",
		Model:    "qwen2.5-1.5b-instruct",
		LocalURL: srv.URL + "/v1",
	}, logger.New("error", "console"))

	res, err := eng.Diagnose(context.Background(), genericErrorTrace())
	if err != nil {
		t.Fatalf("Diagnose returned error: %v", err)
	}
	if !res.Matched {
		t.Fatal("expected Matched=true")
	}
	if res.RootCause == "" || res.Explanation == "" || res.Fix == "" {
		t.Fatalf("expected fully populated result, got %+v", res)
	}
	if !strings.Contains(res.RootCause, "PLMN") {
		t.Errorf("root cause should reflect the model's grounded answer, got %q", res.RootCause)
	}
	if gotModel != "qwen2.5-1.5b-instruct" {
		t.Errorf("model name not forwarded, got %q", gotModel)
	}
	if !strings.Contains(gotPrompt, "NGSetupFailure") {
		t.Errorf("trace was not grounded into the prompt; prompt=%q", gotPrompt)
	}
}

// TestB2_LocalSLMUnformatted verifies graceful degradation: if the SLM returns
// prose instead of JSON, the engine still surfaces the analysis rather than
// dropping it or erroring.
func TestB2_LocalSLMUnformatted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIChatResponse{}
		resp.Choices = append(resp.Choices, struct {
			Message openAIChatMessage `json:"message"`
		}{Message: openAIChatMessage{Role: "assistant", Content: "The PLMN does not match. Fix the gNB config."}})
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	eng := NewEngine(config.AIConfig{
		Provider: "local",
		LocalURL: srv.URL + "/v1",
	}, logger.New("error", "console"))

	res, err := eng.Diagnose(context.Background(), genericErrorTrace())
	if err != nil {
		t.Fatalf("Diagnose returned error: %v", err)
	}
	if !res.Matched {
		t.Fatal("expected Matched=true even for unformatted output")
	}
	if !strings.Contains(res.Fix, "PLMN") {
		t.Errorf("raw model text should be surfaced in Fix, got %q", res.Fix)
	}
}

// TestB2_CatalogStillWinsOffline verifies the local provider never overrides a
// deterministic catalog hit: the catalog runs first, so a known symptom is
// answered without ever calling the SLM.
func TestB2_CatalogStillWinsOffline(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	eng := NewEngine(config.AIConfig{
		Provider: "local",
		LocalURL: srv.URL + "/v1",
	}, logger.New("error", "console"))

	// Unknown-subscriber is a typed catalog rule (5g_unknown_subscriber).
	trace := []events.Event{
		{
			NF:       "amf",
			Category: events.ErrorEvent,
			Severity: events.SeverityError,
			Protocol: "nas5g",
			Message:  "Registration rejected: unknown subscriber",
			Payload: events.RegistrationFailurePayload{
				Cause: "unknown_subscriber",
				SUPI:  "imsi-001019999999999",
			},
		},
	}

	res, err := eng.Diagnose(context.Background(), trace)
	if err != nil {
		t.Fatalf("Diagnose returned error: %v", err)
	}
	if !res.Matched {
		t.Fatal("expected catalog match")
	}
	if called {
		t.Error("SLM must not be called when the catalog already matched")
	}
}

// TestB2_LocalSLMUnreachable verifies graceful degradation when the sidecar is
// down: the engine returns a helpful answer (naming the cause + the `make up-ai`
// fix) instead of erroring, so the dashboard never 500s just because the model
// isn't running.
func TestB2_LocalSLMUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL + "/v1"
	srv.Close() // close immediately so the address refuses connections

	eng := NewEngine(config.AIConfig{
		Provider: "local",
		LocalURL: url,
	}, logger.New("error", "console"))

	res, err := eng.Diagnose(context.Background(), genericErrorTrace())
	if err != nil {
		t.Fatalf("Diagnose should degrade gracefully, not error: %v", err)
	}
	if !res.Matched {
		t.Fatal("expected Matched=true on graceful degradation")
	}
	if !strings.Contains(res.Fix, "up-ai") {
		t.Errorf("fallback should tell the user how to start the model, got %q", res.Fix)
	}
}

// TestB2_NoBackendFallback verifies that with no usable backend the engine
// returns the safe fallback rather than erroring.
func TestB2_NoBackendFallback(t *testing.T) {
	eng := NewEngine(config.AIConfig{Provider: "local", LocalURL: ""}, logger.New("error", "console"))
	res, err := eng.Diagnose(context.Background(), genericErrorTrace())
	if err != nil {
		t.Fatalf("Diagnose returned error: %v", err)
	}
	if !res.Matched || res.Fix == "" {
		t.Fatalf("expected populated fallback, got %+v", res)
	}
}
