// Package llm is a minimal OpenAI chat-completions client.
// ponytail: single non-streaming POST, no retries/rate-limit handling —
// enough for one manual translation at a time. Upgrade path: add retry
// with backoff + token usage reporting if bulk translation ever happens.
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

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	http    *http.Client
}

// New builds a client for any OpenAI-compatible /chat/completions endpoint.
func New(baseURL, apiKey, model string) *Client {
	return &Client{BaseURL: baseURL, APIKey: apiKey, Model: model,
		http: &http.Client{Timeout: 120 * time.Second}}
}

// Chat sends the messages and returns choices[0].message.content.
// Non-2xx responses become a structured error with status + body excerpt.
func (c *Client) Chat(ctx context.Context, msgs []Message) (string, error) {
	payload, err := json.Marshal(map[string]any{"model": c.Model, "messages": msgs})
	if err != nil {
		return "", err
	}
	url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	// ponytail: cap response body at 1 MiB — translations of one post
	// fit far below; larger means a broken/abusive endpoint.
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return "", fmt.Errorf("llm: HTTP %d: %s", res.StatusCode, excerpt(string(raw), 300))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("llm: bad response JSON: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", errors.New("llm: response has no choices")
	}
	return out.Choices[0].Message.Content, nil
}

func excerpt(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
