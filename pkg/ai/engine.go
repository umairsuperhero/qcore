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

	// 2. Escalate to a configured backend.
	switch e.cfg.Provider {
	case "local":
		// Offline embedded SLM (charter §9.3): no cloud, no key required.
		if e.cfg.LocalURL != "" {
			e.log.Infof("AI Engine: escalating diagnosis to local SLM for %d events", len(trace))
			return e.escalateToLocal(ctx, trace)
		}
	case "gemini":
		if e.cfg.APIKey != "" {
			e.log.Infof("AI Engine: escalating diagnosis to Gemini for %d events", len(trace))
			return e.escalateToGemini(ctx, trace)
		}
	}

	// 3. Fallback if no LLM and no heuristic matched
	return &DiagnosticResult{
		Matched:     true,
		Explanation: "The trace contains errors, but no local heuristic matched and no AI provider is configured.",
		RootCause:   "Unknown root cause.",
		Fix:         "Review the event trace manually or configure an AI provider in config.yaml for advanced diagnosis.",
	}, nil
}

// buildPrompt renders the journey trace into the grounded diagnostic prompt.
// Both the cloud (Gemini) and offline (SLM) backends use the same prompt so
// the model always reasons over the structured event trace — never free-floats
// on parametric knowledge (charter §9.4).
func buildPrompt(trace []events.Event) string {
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
	return prompt.String()
}

// parseModelResult converts a model's free-text JSON answer into a
// DiagnosticResult. If the model didn't return clean JSON, the raw text is
// surfaced in Fix rather than dropped, so the user still sees the analysis.
func parseModelResult(text string) *DiagnosticResult {
	// Clean up markdown wrapping if present.
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var parsed struct {
		Explanation string `json:"explanation"`
		RootCause   string `json:"root_cause"`
		Fix         string `json:"fix"`
	}

	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		// Fallback if the model didn't return perfect JSON.
		return &DiagnosticResult{
			Matched:     true,
			Explanation: "The AI analysed the trace but returned unformatted text.",
			RootCause:   "See explanation",
			Fix:         text,
		}
	}

	return &DiagnosticResult{
		Matched:     true,
		Explanation: parsed.Explanation,
		RootCause:   parsed.RootCause,
		Fix:         parsed.Fix,
	}
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
	prompt := buildPrompt(trace)

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: prompt},
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
	return parseModelResult(text), nil
}

// --- Offline SLM backend (OpenAI-compatible /chat/completions) -------------
//
// Works with any server speaking the OpenAI chat API — llama.cpp's server and
// ollama both do. QCore ships a llama.cpp sidecar (qcore-slm) with a small
// instruct model baked in, so the diagnostic AI works with no cloud and no key.

type openAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	// Temperature kept low: this is diagnosis, not creative writing.
	Temperature float64 `json:"temperature"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message openAIChatMessage `json:"message"`
	} `json:"choices"`
}

func (e *Engine) escalateToLocal(ctx context.Context, trace []events.Event) (*DiagnosticResult, error) {
	prompt := buildPrompt(trace)

	model := e.cfg.Model
	if model == "" {
		model = "qwen2.5-1.5b-instruct"
	}

	reqBody := openAIChatRequest{
		Model:       model,
		Stream:      false,
		Temperature: 0.1,
		Messages: []openAIChatMessage{
			{Role: "system", Content: "You are QCore's Diagnostic AI. Reason only over the provided event trace. Respond with a single JSON object and nothing else."},
			{Role: "user", Content: prompt},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal local SLM request: %w", err)
	}

	url := strings.TrimRight(e.cfg.LocalURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create local SLM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// The sidecar isn't reachable. Degrade gracefully: the catalog already
		// ran, so we return a result that names the cause and the fix rather
		// than failing the whole diagnosis (charter: every error names its fix).
		e.log.Warnf("AI Engine: local SLM unreachable at %s: %v", url, err)
		return e.localUnavailable(url), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		e.log.Warnf("AI Engine: local SLM returned status %d: %s", resp.StatusCode, string(bodyBytes))
		return e.localUnavailable(url), nil
	}

	var chatResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		e.log.Warnf("AI Engine: failed to decode local SLM response: %v", err)
		return e.localUnavailable(url), nil
	}

	if len(chatResp.Choices) == 0 {
		e.log.Warnf("AI Engine: local SLM returned no choices")
		return e.localUnavailable(url), nil
	}

	return parseModelResult(chatResp.Choices[0].Message.Content), nil
}

// localUnavailable is the graceful answer when the offline SLM sidecar is not
// reachable. The deterministic catalog has already run by this point, so the
// trace simply didn't match a known rule; we say so plainly and tell the user
// how to turn the model on.
func (e *Engine) localUnavailable(url string) *DiagnosticResult {
	return &DiagnosticResult{
		Matched:     true,
		Explanation: "The trace contains errors, but no built-in diagnostic rule matched it.",
		RootCause:   "Offline AI model is not reachable, so deeper analysis is unavailable.",
		Fix: "Start the offline diagnostic model with `make up-ai` (it serves at " + url +
			"), or review the event trace manually. No cloud account or API key is required.",
	}
}
