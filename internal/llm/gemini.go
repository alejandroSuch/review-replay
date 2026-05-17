package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GeminiConfig configures the native Gemini generateContent adapter.
type GeminiConfig struct {
	BaseURL    string // defaults to https://generativelanguage.googleapis.com/v1beta
	APIKey     string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// NewGemini builds a Provider for the Gemini generateContent API.
func NewGemini(cfg GeminiConfig) (Provider, error) {
	if cfg.APIKey == "" {
		return nil, ErrEmptyKey
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.HTTPClient == nil {
		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = 60 * time.Second
		}
		cfg.HTTPClient = &http.Client{Timeout: timeout}
	}
	return &geminiProvider{cfg: cfg}, nil
}

type geminiProvider struct {
	cfg GeminiConfig
}

func (p *geminiProvider) Name() string { return "gemini" }

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	MaxOutputTokens  int      `json:"maxOutputTokens,omitempty"`
	ResponseMimeType string   `json:"responseMimeType,omitempty"`
}

type geminiRequest struct {
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	Contents          []geminiContent         `json:"contents"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata geminiUsage       `json:"usageMetadata"`
	Error         *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *geminiProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	contents := make([]geminiContent, 0, len(req.Messages))
	for _, m := range req.Messages {
		role := "user"
		if m.Role == RoleAssistant {
			role = "model"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: m.Content}},
		})
	}
	body := geminiRequest{
		Contents: contents,
		GenerationConfig: &geminiGenerationConfig{
			MaxOutputTokens: req.MaxTokens,
		},
	}
	if req.System != "" {
		body.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: req.System}},
		}
	}
	if req.Temperature != 0 {
		t := req.Temperature
		body.GenerationConfig.Temperature = &t
	} else {
		zero := 0.0
		body.GenerationConfig.Temperature = &zero
	}
	if req.JSON {
		body.GenerationConfig.ResponseMimeType = "application/json"
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/models/%s:generateContent", p.cfg.BaseURL, url.PathEscape(req.Model))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", p.cfg.APIKey)

	resp, err := p.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gemini HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var parsed geminiResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("gemini decode: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("gemini: %s", parsed.Error.Message)
	}
	if len(parsed.Candidates) == 0 {
		return nil, fmt.Errorf("gemini: no candidates in response")
	}
	var text strings.Builder
	for _, part := range parsed.Candidates[0].Content.Parts {
		text.WriteString(part.Text)
	}
	return &Response{
		Text: text.String(),
		Usage: Usage{
			InputTokens:  parsed.UsageMetadata.PromptTokenCount,
			OutputTokens: parsed.UsageMetadata.CandidatesTokenCount,
		},
	}, nil
}
