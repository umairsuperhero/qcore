package hss

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/qcore-project/qcore/pkg/logger"
	"github.com/qcore-project/qcore/pkg/metrics"
	"github.com/qcore-project/qcore/pkg/subscriber"
	"gorm.io/gorm"
)

type API struct {
	service *subscriber.Service
	log     logger.Logger
	metrics *metrics.HSSMetrics
	router  *mux.Router
	db      *gorm.DB
}

type APIResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
	Page  int         `json:"page,omitempty"`
	Limit int         `json:"limit,omitempty"`
	Total int64       `json:"total,omitempty"`
}

func NewAPI(service *subscriber.Service, db *gorm.DB, log logger.Logger, m *metrics.HSSMetrics) *API {
	a := &API{
		service: service,
		log:     log.WithField("component", "hss-api"),
		metrics: m,
		router:  mux.NewRouter(),
		db:      db,
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

	subscribers, total, err := a.service.ListSubscribers(r.Context(), page, limit, search)
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
	sub, err := a.service.GetSubscriber(r.Context(), imsi)
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

	if err := a.service.CreateSubscriber(r.Context(), &sub); err != nil {
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

	if err := a.service.UpdateSubscriber(r.Context(), imsi, &updates); err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sub, _ := a.service.GetSubscriber(r.Context(), imsi)
	respondJSON(w, http.StatusOK, APIResponse{Data: sub})
}

func (a *API) deleteSubscriber(w http.ResponseWriter, r *http.Request) {
	imsi := mux.Vars(r)["imsi"]
	if err := a.service.DeleteSubscriber(r.Context(), imsi); err != nil {
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
	av, err := a.service.GenerateAuthVector(r.Context(), imsi)
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

	count, err := a.service.ImportCSV(r.Context(), file)
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

	if err := a.service.ExportCSV(r.Context(), w); err != nil {
		a.log.WithError(err).Error("Failed to export CSV")
	}
}

func (a *API) healthCheck(w http.ResponseWriter, r *http.Request) {
	sqlDB, err := a.db.DB()
	if err != nil {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}
	if err := sqlDB.Ping(); err != nil {
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
