package sbi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteProblem_StatusAndContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteProblem(rec, BadRequest("imsi must be 15 digits"))
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("content-type: want application/problem+json, got %q", ct)
	}

	var pd ProblemDetails
	if err := json.NewDecoder(resp.Body).Decode(&pd); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if pd.Status != 400 {
		t.Errorf("body status: want 400, got %d", pd.Status)
	}
	if pd.Title != "Bad Request" {
		t.Errorf("title: want Bad Request, got %q", pd.Title)
	}
	if pd.Detail != "imsi must be 15 digits" {
		t.Errorf("detail: want 'imsi must be 15 digits', got %q", pd.Detail)
	}
}

func TestWriteProblem_NilDefaultsTo500(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteProblem(rec, nil)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("nil problem should default to 500, got %d", rec.Code)
	}
}

func TestWriteProblem_ZeroStatusDefaultsTo500(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteProblem(rec, &ProblemDetails{Title: "oops"})
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("zero status should default to 500, got %d", rec.Code)
	}
}

func TestProblemDetails_Error(t *testing.T) {
	cases := []struct {
		name string
		p    *ProblemDetails
		want string
	}{
		{"nil", nil, "<nil ProblemDetails>"},
		{"title only", &ProblemDetails{Title: "Not Found"}, "Not Found"},
		{"with cause", &ProblemDetails{Title: "Unauthorized", Cause: "INVALID_KI"}, "Unauthorized: INVALID_KI"},
	}
	for _, tt := range cases {
		if got := tt.p.Error(); got != tt.want {
			t.Errorf("%s: want %q, got %q", tt.name, tt.want, got)
		}
	}
}
