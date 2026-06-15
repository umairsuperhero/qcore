package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/qcore-project/qcore/pkg/simulator"
)

func TestCompareScenarioOutcome(t *testing.T) {
	passDef := &simulator.ScenarioDefinition{
		Name:   "wrong_ki_check",
		Expect: &simulator.ScenarioExpect{Result: "failure", Cause: "wrong_ki", FailedStep: "security_mode_command"},
	}
	pass := CompareScenarioOutcome(passDef, SimulatorStatus{
		State:       SimulatorFailed,
		LastCause:   "wrong_ki",
		FailedStep:  "security_mode_command",
		LastJourney: "j-1",
	})
	if pass.Pass == nil || !*pass.Pass {
		t.Fatalf("expected pass, got %+v", pass)
	}

	fail := CompareScenarioOutcome(passDef, SimulatorStatus{
		State:      SimulatorSuccess,
		LastCause:  "",
		FailedStep: "",
	})
	if fail.Pass == nil || *fail.Pass {
		t.Fatalf("expected fail, got %+v", fail)
	}

	raw := CompareScenarioOutcome(&simulator.ScenarioDefinition{Name: "raw"}, SimulatorStatus{State: SimulatorSuccess})
	if raw.Pass != nil || raw.Actual.Result != "success" {
		t.Fatalf("expected raw success with null pass, got %+v", raw)
	}
}

func TestScenarioHTTPHandlersCRUDAndRun(t *testing.T) {
	store, err := NewScenarioStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewScenarioStore: %v", err)
	}
	srv := &Server{
		scenarios: store,
		scenarioRunHook: func(ctx context.Context, def *simulator.ScenarioDefinition) (SimulatorStatus, error) {
			return SimulatorStatus{
				State:       SimulatorFailed,
				LastJourney: "j-test",
				LastCause:   def.Scenario,
				FailedStep:  "security_mode_command",
				LastError:   "read timed out",
			}, nil
		},
	}

	body := []byte(`
name: wrong_ki_check
description: Wrong Ki should diagnose auth mismatch.
mode: 5g
scenario: wrong_ki
expect:
  result: failure
  cause: wrong_ki
  failed_step: security_mode_command
`)
	req := httptest.NewRequest(http.MethodPost, "/api/scenarios", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleScenarioSave(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("save status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/scenarios", nil)
	rec = httptest.NewRecorder()
	srv.handleScenarioList(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", rec.Code, rec.Body.String())
	}
	var list []ScenarioSummary
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "wrong_ki_check" {
		t.Fatalf("unexpected list: %+v", list)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/scenarios/wrong_ki_check/run", nil)
	req = mux.SetURLVars(req, map[string]string{"name": "wrong_ki_check"})
	rec = httptest.NewRecorder()
	srv.handleScenarioRun(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("run status = %d body=%s", rec.Code, rec.Body.String())
	}
	var result ScenarioRunResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Pass == nil || !*result.Pass || result.JourneyID != "j-test" {
		t.Fatalf("unexpected run result: %+v", result)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/scenarios/wrong_ki_check", nil)
	req = mux.SetURLVars(req, map[string]string{"name": "wrong_ki_check"})
	rec = httptest.NewRecorder()
	srv.handleScenarioDelete(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestScenarioHTTPRunDirect(t *testing.T) {
	srv := &Server{
		scenarioRunHook: func(ctx context.Context, def *simulator.ScenarioDefinition) (SimulatorStatus, error) {
			return SimulatorStatus{
				State:       SimulatorFailed,
				LastJourney: "j-direct",
				LastCause:   def.Scenario,
				FailedStep:  "security_mode_command",
			}, nil
		},
	}

	body := []byte(`
name: direct_check
mode: 5g
scenario: wrong_ki
expect:
  result: failure
  cause: wrong_ki
  failed_step: security_mode_command
`)
	req := httptest.NewRequest(http.MethodPost, "/api/scenarios/run", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleScenarioRunDirect(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("run status = %d body=%s", rec.Code, rec.Body.String())
	}
	var result ScenarioRunResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Pass == nil || !*result.Pass || result.JourneyID != "j-direct" {
		t.Fatalf("unexpected run result: %+v", result)
	}
}
