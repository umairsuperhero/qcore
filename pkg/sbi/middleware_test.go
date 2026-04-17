package sbi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qcore-project/qcore/pkg/logger"
)

func TestRequestID_GeneratesWhenAbsent(t *testing.T) {
	var seen string
	h := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rec, req)

	if len(seen) != 32 {
		t.Errorf("auto-generated request id should be 32 hex chars; got %q", seen)
	}
	if rec.Header().Get(HeaderRequestID) != seen {
		t.Error("response header should echo request id")
	}
}

func TestRequestID_HonoursIncomingHeader(t *testing.T) {
	var seen string
	h := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(HeaderRequestID, "caller-supplied-id")
	h.ServeHTTP(rec, req)

	if seen != "caller-supplied-id" {
		t.Errorf("should propagate caller's id; got %q", seen)
	}
	if rec.Header().Get(HeaderRequestID) != "caller-supplied-id" {
		t.Error("response header should echo caller's id")
	}
}

func TestRequestIDFromContext_EmptyWhenNotSet(t *testing.T) {
	if id := RequestIDFromContext(context.Background()); id != "" {
		t.Errorf("empty context should yield empty id; got %q", id)
	}
}

func TestRecover_CatchesPanic(t *testing.T) {
	log := logger.New("error", "text") // keep logs quiet in test output

	h := ChainMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("boom")
		}),
		Recover(log),
	)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/explode", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("panicking handler should 500, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("recover should emit problem+json, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "panicked") {
		t.Errorf("problem body should mention 'panicked'; got %q", rec.Body.String())
	}
}

func TestChainMiddleware_Order(t *testing.T) {
	// First-in-list wraps outermost; verify via append order.
	var order []string
	mw := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "pre:"+name)
				next.ServeHTTP(w, r)
				order = append(order, "post:"+name)
			})
		}
	}
	h := ChainMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
		}),
		mw("A"), mw("B"),
	)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	want := []string{"pre:A", "pre:B", "handler", "post:B", "post:A"}
	if len(order) != len(want) {
		t.Fatalf("order: want %v, got %v", want, order)
	}
	for i, s := range want {
		if order[i] != s {
			t.Fatalf("order[%d]: want %q, got %q (full: %v)", i, s, order[i], order)
		}
	}
}
