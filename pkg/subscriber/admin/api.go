// Package admin exposes the operator-facing REST API for managing
// subscribers (CRUD, CSV import/export, on-demand auth vector generation).
// It is explicitly not a 3GPP SBI surface — Nudr and Nudm live elsewhere.
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/metrics"
	"github.com/qcore-project/qcore/pkg/subscriber"
)

// Store is the subset of *subscriber.Service that this admin API needs.
// Narrowing to an interface keeps the handlers testable without a real
// database — a struct satisfying Store is enough.
type Store interface {
	ListSubscribers(ctx context.Context, page, limit int, search string) ([]subscriber.Subscriber, int64, error)
	GetSubscriber(ctx context.Context, imsi string) (*subscriber.Subscriber, error)
	CreateSubscriber(ctx context.Context, sub *subscriber.Subscriber) error
	UpdateSubscriber(ctx context.Context, imsi string, updates *subscriber.Subscriber) error
	DeleteSubscriber(ctx context.Context, imsi string) error
	GenerateAuthVector(ctx context.Context, imsi string) (*subscriber.AuthVector, error)
	ImportCSV(ctx context.Context, reader io.Reader) (int, error)
	ExportCSV(ctx context.Context, writer io.Writer) error
}

// HealthCheckFunc lets the caller plug in the liveness probe — typically
// a closure over *gorm.DB.PingContext. Tests pass a stub.
type HealthCheckFunc func(ctx context.Context) error

type API struct {
	store   Store
	health  HealthCheckFunc
	log     logger.Logger
	metrics *metrics.HSSMetrics
	router  *mux.Router
}

type APIResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
	Page  int         `json:"page,omitempty"`
	Limit int         `json:"limit,omitempty"`
	Total int64       `json:"total,omitempty"`
}

func NewAPI(store Store, health HealthCheckFunc, log logger.Logger, m *metrics.HSSMetrics) *API {
	a := &API{
		store:   store,
		health:  health,
		log:     log.WithField("component", "subscriber-admin"),
		metrics: m,
		router:  mux.NewRouter(),
	}
	a.registerRoutes()
	return a
}

func (a *API) Router() *mux.Router {
	return a.router
}

func (a *API) registerRoutes() {
	api := a.router.PathPrefix("/api/v1").Subrouter()
	api.Use(a.loggingMiddleware, a.metricsMiddleware, a.recoveryMiddleware)

	api.HandleFunc("/subscribers", a.listSubscribers).Methods("GET")
	api.HandleFunc("/subscribers/import", a.importCSV).Methods("POST")
	api.HandleFunc("/subscribers/export", a.exportCSV).Methods("GET")
	api.HandleFunc("/subscribers/{imsi}", a.getSubscriber).Methods("GET")
	api.HandleFunc("/subscribers", a.createSubscriber).Methods("POST")
	api.HandleFunc("/subscribers/{imsi}", a.updateSubscriber).Methods("PUT")
	api.HandleFunc("/subscribers/{imsi}", a.deleteSubscriber).Methods("DELETE")
	api.HandleFunc("/subscribers/{imsi}/auth-vector", a.generateAuthVector).Methods("POST")
	api.HandleFunc("/health", a.healthCheck).Methods("GET")
}

func (a *API) listSubscribers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	search := r.URL.Query().Get("search")

	subscribers, total, err := a.store.ListSubscribers(r.Context(), page, limit, search)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, APIResponse{
		Data:  subscribers,
		Page:  page,
		Limit: limit,
		Total: total,
	})
}

func (a *API) getSubscriber(w http.ResponseWriter, r *http.Request) {
	imsi := mux.Vars(r)["imsi"]
	sub, err := a.store.GetSubscriber(r.Context(), imsi)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, APIResponse{Data: sub})
}

func (a *API) createSubscriber(w http.ResponseWriter, r *http.Request) {
	var sub subscriber.Subscriber
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := a.store.CreateSubscriber(r.Context(), &sub); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			respondError(w, http.StatusConflict, err.Error())
			return
		}
		if strings.Contains(err.Error(), "validation") {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, APIResponse{Data: sub})
}

func (a *API) updateSubscriber(w http.ResponseWriter, r *http.Request) {
	imsi := mux.Vars(r)["imsi"]

	var updates subscriber.Subscriber
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := a.store.UpdateSubscriber(r.Context(), imsi, &updates); err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sub, _ := a.store.GetSubscriber(r.Context(), imsi)
	respondJSON(w, http.StatusOK, APIResponse{Data: sub})
}

func (a *API) deleteSubscriber(w http.ResponseWriter, r *http.Request) {
	imsi := mux.Vars(r)["imsi"]
	if err := a.store.DeleteSubscriber(r.Context(), imsi); err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, APIResponse{Data: map[string]bool{"deleted": true}})
}

func (a *API) generateAuthVector(w http.ResponseWriter, r *http.Request) {
	imsi := mux.Vars(r)["imsi"]
	av, err := a.store.GenerateAuthVector(r.Context(), imsi)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, APIResponse{Data: av})
}

func (a *API) importCSV(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "missing file field: "+err.Error())
		return
	}
	defer file.Close()

	count, err := a.store.ImportCSV(r.Context(), file)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, APIResponse{
		Data: map[string]int{"imported": count},
	})
}

func (a *API) exportCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=subscribers.csv")

	if err := a.store.ExportCSV(r.Context(), w); err != nil {
		a.log.WithError(err).Error("Failed to export CSV")
	}
}

func (a *API) healthCheck(w http.ResponseWriter, r *http.Request) {
	if a.health == nil {
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if err := a.health(r.Context()); err != nil {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Middleware

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}

func (a *API) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(sr, r)
		a.log.Infof("%s %s %d %v", r.Method, r.URL.Path, sr.statusCode, time.Since(start))
	})
}

func (a *API) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.metrics == nil {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(sr, r)
		duration := time.Since(start).Seconds()
		a.metrics.APIRequests.WithLabelValues(r.Method, r.URL.Path, strconv.Itoa(sr.statusCode)).Inc()
		a.metrics.APILatency.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
	})
}

func (a *API) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rv := recover(); rv != nil {
				a.log.Errorf("Panic recovered: %v", rv)
				respondError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		fmt.Fprintf(w, `{"error":"failed to encode response"}`)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, APIResponse{Error: message})
}
