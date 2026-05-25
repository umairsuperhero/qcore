package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/simulator"
)

// SimulatorState is the current state of the built-in simulator. We keep
// the state machine small because the simulator is one-shot per run: each
// /simulator/start is a fresh attach attempt, not a long-running daemon.
type SimulatorState string

const (
	SimulatorIdle    SimulatorState = "idle"
	SimulatorRunning SimulatorState = "running"
	SimulatorSuccess SimulatorState = "success"
	SimulatorFailed  SimulatorState = "failed"
)

// SimulatorStatus is what the frontend renders.
type SimulatorStatus struct {
	State        SimulatorState `json:"state"`
	LastJourney  string         `json:"last_journey,omitempty"`
	LastScenario string         `json:"last_scenario,omitempty"` // empty for normal start
	LastError    string         `json:"last_error,omitempty"`
	FailedStep   string         `json:"failed_step,omitempty"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	EndedAt      *time.Time     `json:"ended_at,omitempty"`
}

// SimulatorController owns the built-in simulator. It serializes Start /
// Inject calls (one attach at a time) and updates SimulatorStatus from the
// background runner so the dashboard can poll it.
type SimulatorController struct {
	template simulator.Options
	emitter  events.Emitter
	log      logger.Logger

	mu     sync.Mutex
	status SimulatorStatus
}

// NewSimulatorController binds the controller to a pre-built Options
// template (MME addr, PLMN, TAC, demo subscriber credentials). The
// dashboard binary builds the template from config and passes it in.
func NewSimulatorController(template simulator.Options, emitter events.Emitter, log logger.Logger) *SimulatorController {
	if emitter == nil {
		emitter = &events.NoopEmitter{}
	}
	return &SimulatorController{
		template: template,
		emitter:  emitter,
		log:      log.WithField("component", "simulator"),
		status:   SimulatorStatus{State: SimulatorIdle},
	}
}

// Status returns a snapshot of the controller's state.
func (c *SimulatorController) Status() SimulatorStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Start kicks off a happy-path attach.
func (c *SimulatorController) Start() {
	c.run("")
}

// Inject kicks off an attach with the named error-injection scenario.
func (c *SimulatorController) Inject(scenario string) {
	c.run(scenario)
}

// Stop is a no-op in B4: each attach runs to completion in seconds. We keep
// the endpoint because the React UI calls it on tab-change and we may add
// cancellation in a later phase.
func (c *SimulatorController) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status.State == SimulatorRunning {
		// Leave as-is — runner will update on completion. We don't
		// cancel mid-flight in B4 because there's no clean rollback path
		// for a half-attached UE on the MME side.
		return
	}
	c.status = SimulatorStatus{State: SimulatorIdle}
}

func (c *SimulatorController) run(scenario string) {
	c.mu.Lock()
	if c.status.State == SimulatorRunning {
		c.mu.Unlock()
		return
	}
	now := time.Now().UTC()
	c.status = SimulatorStatus{
		State:        SimulatorRunning,
		LastScenario: scenario,
		StartedAt:    &now,
	}
	c.mu.Unlock()

	go func() {
		opts := c.template
		opts.Scenario = scenario

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		res := simulator.RunExclusive(ctx, opts, c.emitter, c.log)

		c.mu.Lock()
		defer c.mu.Unlock()
		c.status.LastJourney = res.JourneyID
		ended := res.EndedAt
		c.status.EndedAt = &ended
		if res.Success {
			c.status.State = SimulatorSuccess
		} else {
			c.status.State = SimulatorFailed
			c.status.FailedStep = res.FailedStep
			if res.Err != nil {
				c.status.LastError = res.Err.Error()
			}
		}
	}()
}

// --- HTTP handlers ---

func (s *Server) handleSimulatorStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.sim.Status())
}

func (s *Server) handleSimulatorStart(w http.ResponseWriter, r *http.Request) {
	s.sim.Start()
	writeJSON(w, http.StatusAccepted, s.sim.Status())
}

func (s *Server) handleSimulatorStop(w http.ResponseWriter, r *http.Request) {
	s.sim.Stop()
	writeJSON(w, http.StatusOK, s.sim.Status())
}

func (s *Server) handleSimulatorInject(w http.ResponseWriter, r *http.Request) {
	scenario := mux.Vars(r)["scenario"]
	if !validScenario(scenario) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "unknown scenario: " + scenario,
		})
		return
	}
	s.sim.Inject(scenario)
	writeJSON(w, http.StatusAccepted, s.sim.Status())
}

func validScenario(name string) bool {
	switch name {
	case "wrong_ki", "wrong_plmn", "unprovisioned_imsi", "wrong_mme_address":
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
