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

// OpenAICompatibleConfig configures any provider that speaks the OpenAI
// chat-completions wire format. This covers OpenRouter, the upstream OpenAI
// API, Together, Groq, Fireworks, vLLM, LM Studio, Ollama (via /v1), and
// most self-hosted gateways.
type OpenAICompatibleConfig struct {
	Name       string        // friendly name used in logs
	BaseURL    string        // e.g. https://openrouter.ai/api/v1
	APIKey     string        // bearer token
	Timeout    time.Duration // optional, defaults to 60s
	HTTPClient *http.Client  // optional override
	// Headers appended to every request. Useful for OpenRouter's
	// HTTP-Referer / X-Title or for Anthropic's anthropic-version when used
	// via its OpenAI-compatible endpoint.
	Headers map[string]string
}

// NewOpenAICompatible builds an OpenAI-compatible Provider.
func NewOpenAICompatible(cfg OpenAICompatibleConfig) (Provider, error) {
	if cfg.APIKey == "" {
		return nil, ErrEmptyKey
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("llm: empty base URL for provider %q", cfg.Name)
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.HTTPClient == nil {
		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = 60 * time.Second
		}
		cfg.HTTPClient = &http.Client{Timeout: timeout}
	}
	name := cfg.Name
	if name == "" {
		name = "openai-compatible"
	}
	return &openAICompatible{cfg: cfg, name: name}, nil
}

type openAICompatible struct {
	cfg  OpenAICompatibleConfig
	name string
}

func (p *openAICompatible) Name() string { return p.name }

type oaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type oaiResponseFormat struct {
	Type string `json:"type"`
}

type oaiRequest struct {
	Model          string             `json:"model"`
	Messages       []oaiMessage       `json:"messages"`
	MaxTokens      int                `json:"max_tokens,omitempty"`
	Temperature    *float64           `json:"temperature,omitempty"`
	ResponseFormat *oaiResponseFormat `json:"response_format,omitempty"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type oaiChoiceMessage struct {
	Content string `json:"content"`
}

type oaiChoice struct {
	Message oaiChoiceMessage `json:"message"`
}

type oaiResponse struct {
	Choices []oaiChoice `json:"choices"`
	Usage   oaiUsage    `json:"usage"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

func (p *openAICompatible) Complete(ctx context.Context, req Request) (*Response, error) {
	msgs := make([]oaiMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, oaiMessage{Role: string(RoleSystem), Content: req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, oaiMessage{Role: string(m.Role), Content: m.Content})
	}
	body := oaiRequest{
		Model:     req.Model,
		Messages:  msgs,
		MaxTokens: req.MaxTokens,
	}
	if req.Temperature != 0 {
		t := req.Temperature
		body.Temperature = &t
	} else {
		zero := 0.0
		body.Temperature = &zero
	}
	if req.JSON {
		body.ResponseFormat = &oaiResponseFormat{Type: "json_object"}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range p.cfg.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := p.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s HTTP %d: %s", p.name, resp.StatusCode, string(respBody))
	}
	var parsed oaiResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("%s decode: %w", p.name, err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("%s: %s", p.name, parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("%s: no choices in response", p.name)
	}
	return &Response{
		Text: parsed.Choices[0].Message.Content,
		Usage: Usage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
		},
	}, nil
}
