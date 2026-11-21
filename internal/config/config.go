// Package config holds every value you may need to edit by hand (as
// constants) and loads secrets from environment variables at startup.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// =====================================================================
// MANUAL CONFIGURATION
//
// Constants marked "TODO(manual)" must be filled in from Garmin's
// Personal API Swagger docs:
// https://developer.garmin.com/gc-developer-program/personal/swagger-docs/
//
// Any value starting with PlaceholderPrefix counts as "not configured
// yet": the server still starts, logs a warning, and the tool that needs
// it returns a clear error instead of calling Garmin.
// =====================================================================

// PlaceholderPrefix marks a constant that still needs a real value.
const PlaceholderPrefix = "TODO_"

// Server identity, shown to MCP clients.
const (
	ServerName    = "garmin-mcp"
	ServerVersion = "v0.1.0"
)

// Garmin endpoints.
const (
	// GarminTokenURL exchanges the personal API key for an access token.
	// Taken from Garmin's Personal API FAQ; change only if Garmin moves it.
	GarminTokenURL = "https://personal.connectapi.garmin.com/personalapitoken-service/personal-api/personal-api-key/token"

	// Base URL for the data endpoints, no trailing slash (from Swagger "Servers").
	GarminAPIBaseURL = "https://personal.connectapi.garmin.com"

	// Search activities endpoint (Swagger: Activity > searchActivities).
	ActivitiesListPath = "/activitylist-service/activities/search/activities"

	// Get one activity (Swagger: /activity-service/activity/{activityId}).
	// ActivityIDToken marks where the ID goes.
	ActivityDetailPath = "/activity-service/activity/{id}"

	// ActivityIDToken is replaced with the activity ID in ActivityDetailPath.
	ActivityIDToken = "{id}"
)

// TODO(manual): query parameter names for ActivitiesListPath. The Swagger
// PDF had this row collapsed, so these are UNVERIFIED guesses. Expand the
// searchActivities row in Swagger and rename each to match.
const (
	ParamStartDate = "startDate"
	ParamEndDate   = "endDate"
	ParamStart     = "start"
	ParamLimit     = "limit"
)

// Authorization header schemes.
const (
	// AuthSchemeAPIKey is used on the token request ("ApiKey <key>"), per the FAQ.
	AuthSchemeAPIKey = "ApiKey"
	// TODO(manual): confirm in Swagger. "Bearer" is standard for OAuth2 access tokens.
	AuthSchemeData = "Bearer"
)

// TODO(manual): JSON field names in the token response. Run the curl from
// the README once to check. If the access-token field is wrong, the error
// message lists the fields Garmin actually returned.
const (
	TokenFieldAccessToken = "access_token"
	TokenFieldExpiresIn   = "expires_in" // seconds; optional
)

// Token lifetime handling.
const (
	// DefaultTokenTTL is used when the token response has no expiry field.
	// Garmin's FAQ says access tokens last 24 hours.
	DefaultTokenTTL = 24 * time.Hour
	// TokenRefreshMargin fetches a new token this long before expiry.
	TokenRefreshMargin = 10 * time.Minute
)

// Tool defaults and limits.
const (
	DefaultActivityLimit = 10
	MaxActivityLimit     = 50
	DefaultLookbackDays  = 30
)

// HTTP behaviour.
const (
	DefaultListenAddr = "127.0.0.1:8080"
	MCPEndpointPath   = "/mcp"
	HealthPath        = "/healthz"
	HTTPTimeout       = 20 * time.Second
	MaxResponseBytes  = 5 << 20 // 5 MB cap on Garmin responses
	MinSecretLength   = 32
)

// Environment variable names. Secret values live in the environment,
// never in code.
const (
	EnvGarminAPIKey = "GARMIN_API_KEY"
	EnvTransport    = "MCP_TRANSPORT"
	EnvListenAddr   = "LISTEN_ADDR"
	EnvBearerToken  = "MCP_BEARER_TOKEN"
	EnvURLSecret    = "MCP_URL_SECRET"
)

// Values for EnvTransport.
const (
	TransportHTTP  = "http"
	TransportStdio = "stdio"
)

// =====================================================================
// END OF MANUAL CONFIGURATION
// =====================================================================

var urlSafe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Config is the runtime configuration read from the environment.
type Config struct {
	GarminAPIKey string
	Transport    string
	ListenAddr   string
	BearerToken  string
	URLSecret    string
}

// Load reads and validates the environment.
func Load() (Config, error) {
	c := Config{
		GarminAPIKey: strings.TrimSpace(os.Getenv(EnvGarminAPIKey)),
		Transport:    strings.ToLower(strings.TrimSpace(os.Getenv(EnvTransport))),
		ListenAddr:   strings.TrimSpace(os.Getenv(EnvListenAddr)),
		BearerToken:  strings.TrimSpace(os.Getenv(EnvBearerToken)),
		URLSecret:    strings.TrimSpace(os.Getenv(EnvURLSecret)),
	}
	if c.Transport == "" {
		c.Transport = TransportHTTP
	}
	if c.ListenAddr == "" {
		c.ListenAddr = DefaultListenAddr
	}
	if c.GarminAPIKey == "" {
		return Config{}, fmt.Errorf("%s is not set", EnvGarminAPIKey)
	}

	switch c.Transport {
	case TransportStdio:
		// Local only; no endpoint to protect.
	case TransportHTTP:
		if c.BearerToken == "" && c.URLSecret == "" {
			return Config{}, fmt.Errorf("http mode needs %s or %s (or both) so the endpoint is not public", EnvBearerToken, EnvURLSecret)
		}
		if c.BearerToken != "" && len(c.BearerToken) < MinSecretLength {
			return Config{}, fmt.Errorf("%s must be at least %d characters", EnvBearerToken, MinSecretLength)
		}
		if c.URLSecret != "" {
			if len(c.URLSecret) < MinSecretLength {
				return Config{}, fmt.Errorf("%s must be at least %d characters", EnvURLSecret, MinSecretLength)
			}
			if !urlSafe.MatchString(c.URLSecret) {
				return Config{}, fmt.Errorf("%s may only contain letters, digits, '-' and '_'", EnvURLSecret)
			}
		}
	default:
		return Config{}, fmt.Errorf("%s must be %q or %q, got %q", EnvTransport, TransportHTTP, TransportStdio, c.Transport)
	}
	return c, nil
}

// IsPlaceholder reports whether v still holds a TODO placeholder.
func IsPlaceholder(v string) bool {
	return strings.HasPrefix(v, PlaceholderPrefix)
}

// Placeholders returns the names of constants that still need real values.
func Placeholders() []string {
	all := []struct{ name, value string }{
		{"GarminAPIBaseURL", GarminAPIBaseURL},
		{"ActivitiesListPath", ActivitiesListPath},
		{"ActivityDetailPath", ActivityDetailPath},
	}
	out := []string{}
	for _, c := range all {
		if IsPlaceholder(c.value) {
			out = append(out, c.name)
		}
	}
	return out
}
