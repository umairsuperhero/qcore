package ai

import (
	"strings"

	"github.com/qcore-project/qcore/pkg/events"
)

// DiagnosticResult represents a known symptom mapped to a root cause and fix.
type DiagnosticResult struct {
	Matched     bool
	Explanation string
	RootCause   string
	Fix         string
}

// Catalog is the structured diagnostic knowledge layer.
// It maps sequences or specific events to known root causes and fixes.
type Catalog struct{}

// NewCatalog returns a new diagnostic catalog.
func NewCatalog() *Catalog {
	return &Catalog{}
}

// Analyze runs the catalog heuristics over a sequence of events.
// It returns a DiagnosticResult if a known heuristic matches.
func (c *Catalog) Analyze(trace []events.Event) DiagnosticResult {
	if len(trace) == 0 {
		return DiagnosticResult{Matched: false}
	}

	for i := len(trace) - 1; i >= 0; i-- {
		ev := trace[i]

		// Heuristic 1: MAC Failure (wrong Ki or OPc)
		if ev.Category == events.SignalingRx && (strings.Contains(ev.Message, "Authentication Failure") || strings.Contains(ev.Message, "MAC failure")) {
			return DiagnosticResult{
				Matched:     true,
				Explanation: "The network sent an Authentication Request, but the UE rejected it with a MAC failure.",
				RootCause:   "Cryptographic mismatch. The Ki or OPc programmed into the UE/Simulator does not match the credentials provisioned in the HSS/UDR for this IMSI.",
				Fix:         "Verify the Ki and OPc values in the UDR/HSS match exactly with the UE's USIM configuration. Ensure OP is correctly derived to OPc if applicable.",
			}
		}

		// Heuristic 2: Unknown PLMN in S1/NG Setup
		if ev.Category == events.ErrorEvent && strings.Contains(strings.ToLower(ev.Message), "plmn") {
			return DiagnosticResult{
				Matched:     true,
				Explanation: "The base station (eNB/gNB) attempted to connect to the core network but was rejected due to an unrecognised PLMN.",
				RootCause:   "The Mobile Country Code (MCC) and Mobile Network Code (MNC) broadcasted by the base station are not supported by this AMF/MME.",
				Fix:         "Update the PLMN configuration in the AMF/MME to match the base station, or reconfigure the base station to use the core's PLMN (e.g. 00101).",
			}
		}

		// Heuristic 3: Unprovisioned IMSI
		if ev.Category == events.ErrorEvent && (strings.Contains(strings.ToLower(ev.Message), "imsi not found") || strings.Contains(strings.ToLower(ev.Message), "unknown imsi")) {
			return DiagnosticResult{
				Matched:     true,
				Explanation: "The core network received an Attach/Registration Request for an IMSI that does not exist in the subscriber database.",
				RootCause:   "The subscriber (IMSI) is not provisioned in the UDR/HSS.",
				Fix:         "Provision the subscriber in the UDR/HSS via the API or dashboard before attempting to attach.",
			}
		}

		// Heuristic 4: Connection refused / Timeout
		if ev.Category == events.ErrorEvent && (strings.Contains(strings.ToLower(ev.Message), "connection refused") || strings.Contains(strings.ToLower(ev.Message), "timeout")) {
			return DiagnosticResult{
				Matched:     true,
				Explanation: "The simulator or base station failed to establish a network connection to the core network.",
				RootCause:   "The target IP address or port for the MME/AMF is incorrect, or the service is down.",
				Fix:         "Verify that the MME/AMF is running and listening on the configured port. Check firewall rules and the simulator's target address.",
			}
		}
	}

	return DiagnosticResult{Matched: false}
}
