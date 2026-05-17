package llm

import (
	"fmt"
	"os"
	"strings"
)

// Config is the user-facing knobs for picking a provider.
type Config struct {
	Provider string // openrouter | openai | anthropic | gemini | "" (env default)
	Model    string
	APIKey   string
	BaseURL  string
}

// ResolvedConfig reports the values that ended up in effect.
type ResolvedConfig struct {
	Provider     string
	Model        string
	APIKeySource string
	APIKeyLen    int
	BaseURL      string
}

const DefaultProvider = "openrouter"

// Resolve picks a provider, model and API key from flags + env. It returns the
// concrete Provider along with the resolved metadata for diagnostics.
func Resolve(cfg Config) (Provider, ResolvedConfig, error) {
	var rc ResolvedConfig
	provider := pick(cfg.Provider, os.Getenv("RR_PROVIDER"))
	if provider == "" {
		provider = DefaultProvider
	}
	provider = strings.ToLower(provider)
	rc.Provider = provider

	rc.Model = pick(cfg.Model, os.Getenv("RR_MODEL"))
	if rc.Model == "" {
		rc.Model = defaultModel(provider)
	}

	rc.BaseURL = pick(cfg.BaseURL, os.Getenv("RR_BASE_URL"))

	apiKey, source := resolveAPIKey(provider, cfg.APIKey)
	if apiKey == "" {
		return nil, rc, fmt.Errorf("no API key for provider %q: set RR_API_KEY or %s, or pass --api-key", provider, providerEnvHint(provider))
	}
	rc.APIKeySource = source
	rc.APIKeyLen = len(apiKey)

	switch provider {
	case "openrouter":
		base := rc.BaseURL
		if base == "" {
			base = "https://openrouter.ai/api/v1"
		}
		p, err := NewOpenAICompatible(OpenAICompatibleConfig{
			Name:    "openrouter",
			BaseURL: base,
			APIKey:  apiKey,
			Headers: map[string]string{
				"HTTP-Referer": "https://github.com/alejandroSuch/review-replay",
				"X-Title":      "review-replay",
			},
		})
		rc.BaseURL = base
		return p, rc, err
	case "openai":
		base := rc.BaseURL
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		p, err := NewOpenAICompatible(OpenAICompatibleConfig{
			Name:    "openai",
			BaseURL: base,
			APIKey:  apiKey,
		})
		rc.BaseURL = base
		return p, rc, err
	case "anthropic":
		base := rc.BaseURL
		if base == "" {
			base = "https://api.anthropic.com/v1"
		}
		p, err := NewAnthropic(AnthropicConfig{
			BaseURL: base,
			APIKey:  apiKey,
		})
		rc.BaseURL = base
		return p, rc, err
	case "gemini":
		base := rc.BaseURL
		if base == "" {
			base = "https://generativelanguage.googleapis.com/v1beta"
		}
		p, err := NewGemini(GeminiConfig{
			BaseURL: base,
			APIKey:  apiKey,
		})
		rc.BaseURL = base
		return p, rc, err
	default:
		return nil, rc, fmt.Errorf("unknown provider %q (try openrouter | openai | anthropic | gemini)", provider)
	}
}

func defaultModel(provider string) string {
	switch provider {
	case "openrouter":
		return "openai/gpt-4o-mini"
	case "openai":
		return "gpt-4o-mini"
	case "anthropic":
		return "claude-haiku-4-5"
	case "gemini":
		return "gemini-2.5-flash"
	default:
		return ""
	}
}

func providerEnvHint(provider string) string {
	switch provider {
	case "openrouter":
		return "OPENROUTER_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "gemini":
		return "GEMINI_API_KEY"
	}
	return "RR_API_KEY"
}

func resolveAPIKey(provider, override string) (string, string) {
	if override != "" {
		return override, "--api-key"
	}
	if v := os.Getenv("RR_API_KEY"); v != "" {
		return v, "RR_API_KEY"
	}
	switch provider {
	case "openrouter":
		if v := os.Getenv("OPENROUTER_API_KEY"); v != "" {
			return v, "OPENROUTER_API_KEY"
		}
	case "openai":
		if v := os.Getenv("OPENAI_API_KEY"); v != "" {
			return v, "OPENAI_API_KEY"
		}
	case "anthropic":
		if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
			return v, "ANTHROPIC_API_KEY"
		}
	case "gemini":
		if v := os.Getenv("GEMINI_API_KEY"); v != "" {
			return v, "GEMINI_API_KEY"
		}
		if v := os.Getenv("GOOGLE_API_KEY"); v != "" {
			return v, "GOOGLE_API_KEY"
		}
	}
	return "", ""
}

func pick(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
