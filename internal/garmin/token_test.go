package garmin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"garmin-mcp/internal/config"
)

func TestTokenSourceCachesAndRefreshes(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if got := r.Header.Get("Authorization"); got != "ApiKey test-key" {
			t.Errorf("Authorization header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"tok-123","expires_in":3600}`))
	}))
	defer srv.Close()

	ts := NewTokenSource(srv.URL, "test-key", srv.Client())
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		tok, err := ts.Token(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if tok != "tok-123" {
			t.Fatalf("token = %q, want tok-123", tok)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("token requests = %d, want 1 (should be cached)", n)
	}

	ts.Invalidate()
	if _, err := ts.Token(ctx); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("token requests after Invalidate = %d, want 2", n)
	}
}

func TestTokenSourceRejectedKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := NewTokenSource(srv.URL, "bad", srv.Client()).Token(context.Background())
	if err == nil || !strings.Contains(err.Error(), "rejected the API key") {
		t.Fatalf("err = %v, want rejected-key error", err)
	}
}

func TestParseTokenResponse(t *testing.T) {
	tok, ttl, err := parseTokenResponse([]byte(`{"access_token":"abc"}`))
	if err != nil || tok != "abc" || ttl != config.DefaultTokenTTL {
		t.Fatalf("got (%q, %v, %v), want (abc, default TTL, nil)", tok, ttl, err)
	}

	_, ttl, _ = parseTokenResponse([]byte(`{"access_token":"abc","expires_in":60}`))
	if ttl != 60*time.Second {
		t.Fatalf("ttl = %v, want 60s", ttl)
	}

	_, _, err = parseTokenResponse([]byte(`{"token":"abc","validity":86400}`))
	if err == nil || !strings.Contains(err.Error(), "token, validity") {
		t.Fatalf("err = %v, want it to list the fields present", err)
	}
}
