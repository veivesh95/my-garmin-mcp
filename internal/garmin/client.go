package garmin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"garmin-mcp/internal/config"
)

// APIError is returned when Garmin answers with a non-2xx status.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("garmin API returned status %d: %s", e.StatusCode, e.Body)
}

// Client makes authenticated GET requests to the Garmin data endpoints.
type Client struct {
	baseURL string
	tokens  *TokenSource
	httpc   *http.Client
}

// NewClient returns a Client for baseURL.
func NewClient(baseURL string, tokens *TokenSource, httpc *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), tokens: tokens, httpc: httpc}
}

// GetJSON calls GET baseURL+path?query and decodes the JSON body.
// On a 401 it drops the cached token and retries once with a fresh one.
func (c *Client) GetJSON(ctx context.Context, path string, query url.Values) (any, error) {
	if config.IsPlaceholder(c.baseURL) {
		return nil, errors.New("GarminAPIBaseURL is still a placeholder in internal/config/config.go")
	}
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	for attempt := 1; ; attempt++ {
		token, err := c.tokens.Token(ctx)
		if err != nil {
			return nil, err
		}
		status, body, err := c.get(ctx, u, token)
		if err != nil {
			return nil, err
		}
		if status == http.StatusUnauthorized && attempt == 1 {
			c.tokens.Invalidate()
			continue
		}
		if status < 200 || status > 299 {
			return nil, &APIError{StatusCode: status, Body: truncate(string(body), 500)}
		}
		if len(bytes.TrimSpace(body)) == 0 {
			return nil, nil
		}
		var out any
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("decode garmin response: %w", err)
		}
		return out, nil
	}
}

func (c *Client) get(ctx context.Context, u, token string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", config.AuthSchemeData+" "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("call garmin: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBytes+1))
	if err != nil {
		return 0, nil, fmt.Errorf("read garmin response: %w", err)
	}
	if len(body) > config.MaxResponseBytes {
		return 0, nil, fmt.Errorf("garmin response is larger than %d bytes", config.MaxResponseBytes)
	}
	return resp.StatusCode, body, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
