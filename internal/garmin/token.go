// Package garmin talks to the Garmin Connect Personal API.
package garmin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"garmin-mcp/internal/config"
)

// TokenSource exchanges the personal API key for an access token and
// caches it until shortly before it expires. Safe for concurrent use.
type TokenSource struct {
	tokenURL string
	apiKey   string
	httpc    *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// NewTokenSource returns a TokenSource for the given key.
func NewTokenSource(tokenURL, apiKey string, httpc *http.Client) *TokenSource {
	return &TokenSource{tokenURL: tokenURL, apiKey: apiKey, httpc: httpc}
}

// Token returns a valid access token, fetching a new one if needed.
// The mutex is held during the fetch so concurrent callers share one request.
func (t *TokenSource) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.accessToken != "" && time.Until(t.expiresAt) > config.TokenRefreshMargin {
		return t.accessToken, nil
	}
	token, expiresAt, err := t.fetch(ctx)
	if err != nil {
		return "", err
	}
	t.accessToken, t.expiresAt = token, expiresAt
	return token, nil
}

// ExpiresAt returns when the cached token expires (zero if none).
func (t *TokenSource) ExpiresAt() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.expiresAt
}

// Invalidate drops the cached token so the next call fetches a new one.
func (t *TokenSource) Invalidate() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.accessToken = ""
	t.expiresAt = time.Time{}
}

func (t *TokenSource) fetch(ctx context.Context) (string, time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.tokenURL, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Authorization", config.AuthSchemeAPIKey+" "+t.apiKey)

	resp, err := t.httpc.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("request garmin token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("read garmin token response: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return "", time.Time{}, fmt.Errorf("garmin rejected the API key (status %d): it may be wrong, deleted or expired; create a new key in Garmin Connect Web", resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return "", time.Time{}, &APIError{StatusCode: resp.StatusCode, Body: truncate(string(body), 500)}
	}

	token, ttl, err := parseTokenResponse(body)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, time.Now().Add(ttl), nil
}

// parseTokenResponse pulls the token and lifetime out of Garmin's reply,
// using the field names from config.
func parseTokenResponse(body []byte) (string, time.Duration, error) {
	trimmed := bytes.TrimSpace(body)

	var payload map[string]any
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		// Not a JSON object: accept a bare token string as a fallback.
		if len(trimmed) > 0 && !bytes.ContainsAny(trimmed, " {}[]\"\n") {
			return string(trimmed), config.DefaultTokenTTL, nil
		}
		return "", 0, fmt.Errorf("token response is not JSON: %w", err)
	}

	token, _ := payload[config.TokenFieldAccessToken].(string)
	if token == "" {
		keys := make([]string, 0, len(payload))
		for k := range payload {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return "", 0, fmt.Errorf("token response has no %q field; fields present: [%s]; update config.TokenFieldAccessToken",
			config.TokenFieldAccessToken, strings.Join(keys, ", "))
	}

	ttl := config.DefaultTokenTTL
	if secs, ok := payload[config.TokenFieldExpiresIn].(float64); ok && secs > 0 {
		ttl = time.Duration(secs) * time.Second
	}
	return token, ttl, nil
}
