// Package llm provides a minimal client for a local Ollama LLM server.
// It is intentionally decoupled from the rest of the app: callers map its
// errors (ErrUnavailable) onto their own domain errors.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrUnavailable indicates the LLM backend is unreachable, disabled, slow,
// or returned an error. Callers typically map this to a 503.
var ErrUnavailable = errors.New("llm: backend unavailable")

// Client generates a text completion from a prompt.
type Client interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// Config configures the Ollama client.
type Config struct {
	BaseURL              string
	Model                string
	Timeout              time.Duration
	CFAccessClientID     string
	CFAccessClientSecret string
}

// New returns a Client. If BaseURL is empty the feature is disabled and the
// returned client always reports ErrUnavailable (clean 503, never a 500).
func New(cfg Config) Client {
	if cfg.BaseURL == "" {
		return disabled{}
	}
	return &ollama{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		model:   cfg.Model,
		cfID:    cfg.CFAccessClientID,
		cfKey:   cfg.CFAccessClientSecret,
		http:    &http.Client{Timeout: cfg.Timeout},
	}
}

type disabled struct{}

func (disabled) Generate(context.Context, string) (string, error) { return "", ErrUnavailable }

type ollama struct {
	baseURL string
	model   string
	cfID    string
	cfKey   string
	http    *http.Client
}

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type generateResponse struct {
	Response string `json:"response"`
}

func (o *ollama) Generate(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(generateRequest{Model: o.model, Prompt: prompt, Stream: false})
	if err != nil {
		return "", fmt.Errorf("%w: marshal request: %v", ErrUnavailable, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("%w: build request: %v", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if o.cfID != "" {
		req.Header.Set("CF-Access-Client-Id", o.cfID)
		req.Header.Set("CF-Access-Client-Secret", o.cfKey)
	}

	resp, err := o.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("%w: status %d: %s", ErrUnavailable, resp.StatusCode, string(b))
	}

	var gr generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return "", fmt.Errorf("%w: decode response: %v", ErrUnavailable, err)
	}
	if strings.TrimSpace(gr.Response) == "" {
		return "", fmt.Errorf("%w: empty response", ErrUnavailable)
	}
	return gr.Response, nil
}
