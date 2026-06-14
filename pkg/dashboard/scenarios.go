package dashboard

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/qcore-project/qcore/pkg/simulator"
)

type ScenarioActualOutcome struct {
	Result     string `json:"result"`
	Cause      string `json:"cause,omitempty"`
	FailedStep string `json:"failed_step,omitempty"`
	Error      string `json:"error,omitempty"`
}

type ScenarioRunResult struct {
	Scenario   string                    `json:"scenario"`
	Pass       *bool                     `json:"pass"`
	Expected   *simulator.ScenarioExpect `json:"expected,omitempty"`
	Actual     ScenarioActualOutcome     `json:"actual"`
	JourneyID  string                    `json:"journey_id,omitempty"`
	FailedStep string                    `json:"failed_step,omitempty"`
	Error      string                    `json:"error,omitempty"`
}

func (s *Server) handleScenarioList(w http.ResponseWriter, r *http.Request) {
	items, err := s.scenarios.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleScenarioSave(w http.ResponseWriter, r *http.Request) {
	def, err := readScenarioDefinition(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	rec, err := s.scenarios.Save(def)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) handleScenarioGet(w http.ResponseWriter, r *http.Request) {
	rec, err := s.scenarios.Get(mux.Vars(r)["name"])
	if err != nil {
		writeScenarioStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) handleScenarioDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.scenarios.Delete(mux.Vars(r)["name"]); err != nil {
		writeScenarioStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleScenarioRun(w http.ResponseWriter, r *http.Request) {
	rec, err := s.scenarios.Get(mux.Vars(r)["name"])
	if err != nil {
		writeScenarioStoreError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	status, err := s.runScenario(ctx, rec.Definition)
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, ErrSimulatorBusy) {
			code = http.StatusConflict
		} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			code = http.StatusGatewayTimeout
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, CompareScenarioOutcome(rec.Definition, status))
}

func readScenarioDefinition(w http.ResponseWriter, r *http.Request) (*simulator.ScenarioDefinition, error) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return simulator.LoadScenario(bytes.NewReader(body))
}

func writeScenarioStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

func (s *Server) runScenario(ctx context.Context, def *simulator.ScenarioDefinition) (SimulatorStatus, error) {
	if s.scenarioRunHook != nil {
		return s.scenarioRunHook(ctx, def)
	}
	return s.sim.RunCustomAndWait(ctx, def)
}

func CompareScenarioOutcome(def *simulator.ScenarioDefinition, status SimulatorStatus) ScenarioRunResult {
	actualResult := strings.ToLower(string(status.State))
	if actualResult == string(SimulatorSuccess) {
		actualResult = "success"
	} else if actualResult == string(SimulatorFailed) {
		actualResult = "failure"
	}
	actual := ScenarioActualOutcome{
		Result:     actualResult,
		Cause:      status.LastCause,
		FailedStep: status.FailedStep,
		Error:      status.LastError,
	}
	if actual.Cause == "" {
		actual.Cause = status.FailedStep
	}

	var pass *bool
	if def.Expect != nil && strings.TrimSpace(def.Expect.Result) != "" {
		ok := strings.EqualFold(def.Expect.Result, actual.Result)
		if def.Expect.Cause != "" {
			ok = ok && strings.EqualFold(def.Expect.Cause, actual.Cause)
		}
		if def.Expect.FailedStep != "" {
			ok = ok && strings.EqualFold(def.Expect.FailedStep, actual.FailedStep)
		}
		pass = &ok
	}

	return ScenarioRunResult{
		Scenario:   def.Name,
		Pass:       pass,
		Expected:   def.Expect,
		Actual:     actual,
		JourneyID:  status.LastJourney,
		FailedStep: status.FailedStep,
		Error:      status.LastError,
	}
}
