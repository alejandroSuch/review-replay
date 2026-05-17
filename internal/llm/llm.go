// Package llm defines the provider-agnostic chat completion interface used by
// the classifier and the concrete adapters for each supported backend.
package llm

import (
	"context"
	"errors"
)

// Role of a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single chat turn.
type Message struct {
	Role    Role
	Content string
}

// Usage reports token counts when the provider exposes them.
type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
}

// Request is a generic chat completion request.
type Request struct {
	System      string
	Messages    []Message
	Model       string
	MaxTokens   int
	Temperature float64
	// JSON requests the provider to coerce the response into a JSON object
	// when supported. Adapters that do not support strict JSON mode should
	// still honour this flag by adding stronger prompt instructions.
	JSON bool
}

// Response is the normalized completion result.
type Response struct {
	Text  string
	Usage Usage
}

// Provider is the chat completion abstraction the classifier depends on.
type Provider interface {
	Complete(ctx context.Context, req Request) (*Response, error)
	Name() string
}

// ErrEmptyKey is returned by adapters that require an API key but did not
// receive one.
var ErrEmptyKey = errors.New("llm: missing API key")
