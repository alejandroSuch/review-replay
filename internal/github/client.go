package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	gh "github.com/google/go-github/v66/github"
)

// Client is a thin wrapper around go-github plus a hand-rolled GraphQL helper.
type Client struct {
	REST    *gh.Client
	graphql *http.Client
	token   string
}

// NewClient builds a GitHub client authenticated against the GITHUB_TOKEN or
// GH_TOKEN environment variables.
func NewClient(ctx context.Context) (*Client, error) {
	token := firstNonEmpty(os.Getenv("GITHUB_TOKEN"), os.Getenv("GH_TOKEN"))
	if token == "" {
		return nil, fmt.Errorf("no GitHub token found: set GITHUB_TOKEN or GH_TOKEN")
	}
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &bearerTransport{token: token, base: http.DefaultTransport},
	}
	return &Client{
		REST:    gh.NewClient(httpClient),
		graphql: httpClient,
		token:   token,
	}, nil
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", "Bearer "+t.token)
	r.Header.Set("User-Agent", "review-replay-go")
	return t.base.RoundTrip(r)
}

// graphqlError is the shape of a GraphQL error entry.
type graphqlError struct {
	Message string `json:"message"`
}

// graphqlEnvelope is the top-level response shape.
type graphqlEnvelope struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphqlError  `json:"errors"`
}

// graphqlQuery executes a query against api.github.com/graphql and unmarshals
// the data field into out.
func (c *Client) graphqlQuery(ctx context.Context, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.github.com/graphql", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.graphql.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("graphql HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var env graphqlEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("graphql decode: %w", err)
	}
	if len(env.Errors) > 0 {
		msgs := make([]string, len(env.Errors))
		for i, e := range env.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("graphql errors: %v", msgs)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("graphql data decode: %w", err)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
