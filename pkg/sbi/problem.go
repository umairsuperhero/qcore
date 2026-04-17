package sbi

import (
	"encoding/json"
	"net/http"
)

// ProblemDetails is the 3GPP TS 29.571 / RFC 7807 error body every 5G SBI
// endpoint emits on non-2xx. The full 3GPP schema has more optional fields
// (accessTokenError, invalidParams, supportedApiVersions, ...) — we add them
// as concrete NFs need them rather than carrying dead weight.
//
// The "type" field is a URI that identifies the problem class. Per RFC 7807
// it should resolve to human-readable documentation; for now we use relative
// QCore URNs like "qcore:auth/invalid-ki" and wire them to docs later.
type ProblemDetails struct {
	Type     string `json:"type,omitempty"`
	Title    string `json:"title,omitempty"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
	// Cause is 3GPP-specific: a short machine-readable enum, e.g.
	// "USER_NOT_FOUND", "AUTHENTICATION_REJECTED".
	Cause string `json:"cause,omitempty"`
}

// Error lets ProblemDetails satisfy the error interface so handlers can
// propagate it through normal Go error plumbing.
func (p *ProblemDetails) Error() string {
	if p == nil {
		return "<nil ProblemDetails>"
	}
	if p.Cause != "" {
		return p.Title + ": " + p.Cause
	}
	return p.Title
}

// WriteProblem serialises a ProblemDetails as application/problem+json per
// RFC 7807. If p.Status is zero, it defaults to 500 to avoid surprising
// 200-with-error-body responses.
func WriteProblem(w http.ResponseWriter, p *ProblemDetails) {
	if p == nil {
		p = &ProblemDetails{Status: http.StatusInternalServerError, Title: "Internal Server Error"}
	}
	if p.Status == 0 {
		p.Status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	// Encoding can't meaningfully fail for a struct of strings/ints; ignore
	// the error to keep handler code clean.
	_ = json.NewEncoder(w).Encode(p)
}

// Common helpers. Specific NFs will add their own (e.g. AuthorizationRejected,
// N1MsgDeliveryFailure) as the API surface grows.

func BadRequest(detail string) *ProblemDetails {
	return &ProblemDetails{Status: http.StatusBadRequest, Title: "Bad Request", Detail: detail}
}

func NotFound(detail string) *ProblemDetails {
	return &ProblemDetails{Status: http.StatusNotFound, Title: "Not Found", Detail: detail}
}

func InternalError(detail string) *ProblemDetails {
	return &ProblemDetails{Status: http.StatusInternalServerError, Title: "Internal Server Error", Detail: detail}
}
