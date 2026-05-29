package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/events"
	"github.com/qcore-project/qcore/pkg/logger"
)

// Engine is the diagnostic AI engine.
type Engine struct {
	cfg     config.AIConfig
	log     logger.Logger
	catalog *Catalog
}

// NewEngine creates a new diagnostic engine.
func NewEngine(cfg config.AIConfig, log logger.Logger) *Engine {
	return &Engine{
		cfg:     cfg,
		log:     log,
		catalog: NewCatalog(),
	}
}

// Diagnose evaluates a journey trace and returns a DiagnosticResult.
// It first attempts to use the local catalog (heuristics).
// If it fails, and the provider is 'gemini', it escalates to the LLM.
func (e *Engine) Diagnose(ctx context.Context, trace []events.Event) (*DiagnosticResult, error) {
	if len(trace) == 0 {
		return nil, fmt.Errorf("empty trace provided")
	}

	// 1. Try the local structured catalog first
	res := e.catalog.Analyze(trace)
	if res.Matched {
		e.log.Infof("AI Engine: matched local catalog heuristic for %d events", len(trace))
		return &res, nil
	}

	// 2. Escalate to Gemini if configured
	if e.cfg.Provider == "gemini" && e.cfg.APIKey != "" {
		e.log.Infof("AI Engine: escalating diagnosis to Gemini for %d events", len(trace))
		return e.escalateToGemini(ctx, trace)
	}

	// 3. Fallback if no LLM and no heuristic matched
	return &DiagnosticResult{
		Matched:     true,
		Explanation: "The trace contains errors, but no local heuristic matched and no AI provider is configured.",
		RootCause:   "Unknown root cause.",
		Fix:         "Review the event trace manually or configure an AI provider in config.yaml for advanced diagnosis.",
	}, nil
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (e *Engine) escalateToGemini(ctx context.Context, trace []events.Event) (*DiagnosticResult, error) {
	// Build prompt
	var prompt bytes.Buffer
	prompt.WriteString("You are QCore's Diagnostic AI. Analyze the following 4G/5G core network event trace. The trace resulted in an error or failure to attach.\n")
	prompt.WriteString("Provide your response as a JSON object with three keys: 'explanation' (plain English narration of what happened), 'root_cause' (the technical reason for the failure), and 'fix' (how to resolve it). Do not use markdown blocks for the JSON.\n\n")
	prompt.WriteString("Event Trace:\n")

	for _, ev := range trace {
		prompt.WriteString(fmt.Sprintf("- [%s] %s (NF: %s, Severity: %s, Protocol: %s)\n", ev.Category, ev.Message, ev.NF, ev.Severity, ev.Protocol))
		if ev.Payload != nil {
			payloadBytes, _ := json.Marshal(ev.Payload)
			prompt.WriteString(fmt.Sprintf("  Payload: %s\n", string(payloadBytes)))
		}
	}

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: prompt.String()},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal gemini request: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", e.cfg.Model, e.cfg.APIKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call gemini api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini api returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var geminiResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini returned no content")
	}

	text := geminiResp.Candidates[0].Content.Parts[0].Text

	// Clean up markdown wrapping if present
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")

	var parsed struct {
		Explanation string `json:"explanation"`
		RootCause   string `json:"root_cause"`
		Fix         string `json:"fix"`
	}

	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		// Fallback if the LLM didn't return perfect JSON
		return &DiagnosticResult{
			Matched:     true,
			Explanation: "The AI analysed the trace but returned unformatted text.",
			RootCause:   "See explanation",
			Fix:         text,
		}, nil
	}

	return &DiagnosticResult{
		Matched:     true,
		Explanation: parsed.Explanation,
		RootCause:   parsed.RootCause,
		Fix:         parsed.Fix,
	}, nil
}
