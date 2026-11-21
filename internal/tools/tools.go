// Package tools defines the MCP tools exposed to the AI client.
package tools

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"garmin-mcp/internal/config"
	"garmin-mcp/internal/garmin"
)

const dateLayout = "2006-01-02"

var activityIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)

// NoInput is the input type for tools that take no arguments.
type NoInput struct{}

// ConnectionStatus is the output of check_garmin_connection.
type ConnectionStatus struct {
	TokenOK               bool     `json:"token_ok" jsonschema:"true if a Garmin access token was obtained"`
	TokenExpiresAt        string   `json:"token_expires_at,omitempty" jsonschema:"when the cached token expires, RFC 3339 UTC"`
	Error                 string   `json:"error,omitempty" jsonschema:"why the token could not be obtained"`
	UnconfiguredConstants []string `json:"unconfigured_constants" jsonschema:"config constants that still hold TODO placeholders"`
}

// ListActivitiesInput is the input of list_activities.
type ListActivitiesInput struct {
	StartDate string `json:"start_date,omitempty" jsonschema:"inclusive start date YYYY-MM-DD; defaults to 30 days before end_date"`
	EndDate   string `json:"end_date,omitempty" jsonschema:"inclusive end date YYYY-MM-DD; defaults to today"`
	Limit     int    `json:"limit,omitempty" jsonschema:"maximum number of activities, 1-50; defaults to 10"`
}

// GetActivityInput is the input of get_activity.
type GetActivityInput struct {
	ActivityID string `json:"activity_id" jsonschema:"numeric Garmin activity ID, as returned by list_activities"`
}

// RawOutput wraps Garmin's JSON unchanged. Replace with typed structs
// once you know the response shapes from Swagger.
type RawOutput struct {
	Data any `json:"data" jsonschema:"raw JSON returned by the Garmin endpoint"`
}

type handlers struct {
	client *garmin.Client
	tokens *garmin.TokenSource
}

// Register adds all tools to the server.
func Register(s *mcp.Server, client *garmin.Client, tokens *garmin.TokenSource) {
	h := &handlers{client: client, tokens: tokens}
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "check_garmin_connection",
		Description: "Check that the server can get a Garmin access token and list config constants that still need real values. Use this first when another Garmin tool fails.",
		Annotations: readOnly,
	}, h.checkConnection)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_activities",
		Description: "List the user's Garmin activities (runs, walks, rides and so on) in a date range. Returns Garmin's raw JSON.",
		Annotations: readOnly,
	}, h.listActivities)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_activity",
		Description: "Get the details of one Garmin activity by its ID from list_activities. Returns Garmin's raw JSON.",
		Annotations: readOnly,
	}, h.getActivity)
}

func (h *handlers) checkConnection(ctx context.Context, _ *mcp.CallToolRequest, _ NoInput) (*mcp.CallToolResult, ConnectionStatus, error) {
	status := ConnectionStatus{UnconfiguredConstants: config.Placeholders()}
	if _, err := h.tokens.Token(ctx); err != nil {
		status.Error = err.Error()
		return nil, status, nil
	}
	status.TokenOK = true
	status.TokenExpiresAt = h.tokens.ExpiresAt().UTC().Format(time.RFC3339)
	return nil, status, nil
}

func (h *handlers) listActivities(ctx context.Context, _ *mcp.CallToolRequest, in ListActivitiesInput) (*mcp.CallToolResult, RawOutput, error) {
	if config.IsPlaceholder(config.ActivitiesListPath) {
		return nil, RawOutput{}, notConfigured("ActivitiesListPath")
	}

	limit := in.Limit
	if limit == 0 {
		limit = config.DefaultActivityLimit
	}
	if limit < 1 || limit > config.MaxActivityLimit {
		return nil, RawOutput{}, fmt.Errorf("limit must be between 1 and %d", config.MaxActivityLimit)
	}

	end := time.Now()
	if in.EndDate != "" {
		d, err := time.Parse(dateLayout, in.EndDate)
		if err != nil {
			return nil, RawOutput{}, fmt.Errorf("end_date must be YYYY-MM-DD: %w", err)
		}
		end = d
	}
	start := end.AddDate(0, 0, -config.DefaultLookbackDays)
	if in.StartDate != "" {
		d, err := time.Parse(dateLayout, in.StartDate)
		if err != nil {
			return nil, RawOutput{}, fmt.Errorf("start_date must be YYYY-MM-DD: %w", err)
		}
		start = d
	}
	if start.After(end) {
		return nil, RawOutput{}, fmt.Errorf("start_date is after end_date")
	}

	q := url.Values{}
	q.Set(config.ParamStartDate, start.Format(dateLayout))
	q.Set(config.ParamEndDate, end.Format(dateLayout))
	q.Set(config.ParamStart, "0")
	q.Set(config.ParamLimit, strconv.Itoa(limit))

	data, err := h.client.GetJSON(ctx, config.ActivitiesListPath, q)
	if err != nil {
		return nil, RawOutput{}, err
	}
	return nil, RawOutput{Data: data}, nil
}

func (h *handlers) getActivity(ctx context.Context, _ *mcp.CallToolRequest, in GetActivityInput) (*mcp.CallToolResult, RawOutput, error) {
	id := strings.TrimSpace(in.ActivityID)
	if !activityIDPattern.MatchString(id) {
		return nil, RawOutput{}, fmt.Errorf("activity_id must be numeric")
	}
	if config.IsPlaceholder(config.ActivityDetailPath) {
		return nil, RawOutput{}, notConfigured("ActivityDetailPath")
	}
	if !strings.Contains(config.ActivityDetailPath, config.ActivityIDToken) {
		return nil, RawOutput{}, fmt.Errorf("ActivityDetailPath must contain %s where the ID goes", config.ActivityIDToken)
	}

	path := strings.ReplaceAll(config.ActivityDetailPath, config.ActivityIDToken, url.PathEscape(id))
	data, err := h.client.GetJSON(ctx, path, nil)
	if err != nil {
		return nil, RawOutput{}, err
	}
	return nil, RawOutput{Data: data}, nil
}

func notConfigured(name string) error {
	return fmt.Errorf("%s is still a placeholder in internal/config/config.go; fill it in from Garmin's Personal API Swagger docs", name)
}
