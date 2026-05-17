package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AnthropicConfig configures the native Anthropic Messages API adapter.
type AnthropicConfig struct {
	BaseURL    string        // defaults to https://api.anthropic.com/v1
	APIKey     string
	Version    string        // anthropic-version header, defaults to 2023-06-01
	Timeout    time.Duration // defaults to 60s
	HTTPClient *http.Client
}

// NewAnthropic builds a Provider for the Anthropic Messages API.
func NewAnthropic(cfg AnthropicConfig) (Provider, error) {
	if cfg.APIKey == "" {
		return nil, ErrEmptyKey
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com/v1"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Version == "" {
		cfg.Version = "2023-06-01"
	}
	if cfg.HTTPClient == nil {
		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = 60 * time.Second
		}
		cfg.HTTPClient = &http.Client{Timeout: timeout}
	}
	return &anthropicProvider{cfg: cfg}, nil
}

type anthropicProvider struct {
	cfg AnthropicConfig
}

func (p *anthropicProvider) Name() string { return "anthropic" }

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Usage   struct {
		InputTokens             int `json:"input_tokens"`
		OutputTokens            int `json:"output_tokens"`
		CacheReadInputTokens    int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *anthropicProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	msgs := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		// Anthropic accepts user/assistant; map any system message that slipped
		// in to user content.
		role := string(m.Role)
		if role == string(RoleSystem) {
			role = string(RoleUser)
		}
		msgs = append(msgs, anthropicMessage{Role: role, Content: m.Content})
	}
	body := anthropicRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		System:    req.System,
		Messages:  msgs,
	}
	if body.MaxTokens == 0 {
		body.MaxTokens = 1024
	}
	if req.Temperature != 0 {
		t := req.Temperature
		body.Temperature = &t
	} else {
		zero := 0.0
		body.Temperature = &zero
	}
	if req.JSON {
		// Anthropic has no response_format flag yet; reinforce the instruction
		// inline so JSON-mode requests still bias toward strict JSON output.
		body.System = strings.TrimSpace(body.System) + "\n\nIMPORTANT: respond with a single JSON object and nothing else. No prose, no markdown fences."
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/messages", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", p.cfg.Version)

	resp, err := p.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("anthropic HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var parsed anthropicResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("anthropic decode: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("anthropic: %s", parsed.Error.Message)
	}
	var text strings.Builder
	for _, blk := range parsed.Content {
		if blk.Type == "text" {
			text.WriteString(blk.Text)
		}
	}
	return &Response{
		Text: text.String(),
		Usage: Usage{
			InputTokens:      parsed.Usage.InputTokens,
			OutputTokens:     parsed.Usage.OutputTokens,
			CacheReadTokens:  parsed.Usage.CacheReadInputTokens,
			CacheWriteTokens: parsed.Usage.CacheCreationInputTokens,
		},
	}, nil
}
