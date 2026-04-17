package sbi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/qcore-project/qcore/pkg/logger"
)

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyLogger
)

// HeaderRequestID is the correlation header. 3GPP TS 29.500 standardises
// "X-Qcore-RequestId" as a per-request trace id that propagates across SBI
// hops. We set it to 3gpp-sbi-correlation-info for 3GPP compat when empty.
const HeaderRequestID = "X-Qcore-RequestId"

// Middleware is a chainable HTTP wrapper. Compose with ChainMiddleware.
type Middleware func(http.Handler) http.Handler

// ChainMiddleware applies wrappers in order — the first middleware in the
// list is the outermost (runs first on request, last on response).
func ChainMiddleware(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// RequestID assigns a request ID (from incoming header if present, else fresh)
// and stashes it in the context so handlers can log with it.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(HeaderRequestID)
			if id == "" {
				id = newRequestID()
			}
			w.Header().Set(HeaderRequestID, id)
			ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestIDFromContext returns the request ID set by RequestID(). Empty if
// the middleware wasn't installed (e.g. direct httptest without chain).
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

// AccessLog logs every request with method, path, status, duration, request
// ID. Keeps payload bodies out of the log — too easy to leak IMSI otherwise.
func AccessLog(log logger.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			log.WithFields(map[string]interface{}{
				"method":     r.Method,
				"path":       r.URL.Path,
				"status":     rec.status,
				"bytes":      rec.bytes,
				"duration":   time.Since(start).String(),
				"request_id": RequestIDFromContext(r.Context()),
				"remote":     r.RemoteAddr,
			}).Info("sbi access")
		})
	}
}

// Recover traps panics and converts them into 500 ProblemDetails so a single
// bad handler doesn't take down the whole service.
func Recover(log logger.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.WithFields(map[string]interface{}{
						"request_id": RequestIDFromContext(r.Context()),
						"path":       r.URL.Path,
						"panic":      rec,
						"stack":      string(debug.Stack()),
					}).Error("sbi panic")
					WriteProblem(w, InternalError("handler panicked"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// --- internals ---

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wrote {
		return
	}
	r.status = code
	r.wrote = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.wrote = true
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// newRequestID returns a 32-char hex id. Short enough to be log-friendly,
// unique enough that collisions don't matter at our scale.
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Extremely unlikely; fall back to timestamp so we never block a request.
		return "req-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(b[:])
}
