// Package client is a thin HTTP client for the Watchpoint REST API
// (https://api.everwatch-labs.io). It handles authentication via the
// X-API-Key header, JSON (de)serialization, tenant-path scoping, and maps
// non-2xx responses onto a typed APIError so resources can branch on 404.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the production API root. The path already includes the
// /api/v1 version segment; service prefixes (/monitoring, /identity, /billing,
// /status, /rum, /regions) are appended by callers.
const DefaultBaseURL = "https://api.everwatch-labs.io/api/v1"

// Client talks to the Watchpoint API for a single tenant.
type Client struct {
	baseURL    string
	apiKey     string
	tenantID   string
	httpClient *http.Client
	userAgent  string
	workspace  string
}

// Config configures a Client.
type Config struct {
	BaseURL   string
	APIKey    string
	TenantID  string
	UserAgent string
	// Workspace is sent on managed writes; empty means "default".
	Workspace string
	// HTTPClient is optional; a sane default is used when nil.
	HTTPClient *http.Client
}

// New constructs a Client from Config.
func New(cfg Config) *Client {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	ua := cfg.UserAgent
	if ua == "" {
		ua = "terraform-provider-watchpoint"
	}
	ws := strings.TrimSpace(cfg.Workspace)
	if ws == "" {
		ws = "default"
	}
	return &Client{
		baseURL:    base,
		apiKey:     cfg.APIKey,
		tenantID:   cfg.TenantID,
		httpClient: hc,
		userAgent:  ua,
		workspace:  ws,
	}
}

// TenantID returns the tenant this client is scoped to.
func (c *Client) TenantID() string { return c.tenantID }

// Workspace returns the Terraform workspace sent on managed writes.
func (c *Client) Workspace() string { return c.workspace }

// APIError is returned for any non-2xx response.
type APIError struct {
	StatusCode int
	Method     string
	Path       string
	Message    string
	Body       string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = e.Body
	}
	return fmt.Sprintf("watchpoint API %s %s returned %d: %s", e.Method, e.Path, e.StatusCode, msg)
}

// IsNotFound reports whether err is an APIError with a 404 status.
func IsNotFound(err error) bool {
	ae, ok := err.(*APIError)
	return ok && ae.StatusCode == http.StatusNotFound
}

// Tenantf builds a tenant-scoped path. The leading service prefix is supplied
// by the caller, e.g. Tenantf("/monitoring", "/monitors/%s", id) yields
// /monitoring/tenants/<tenant>/monitors/<id>.
//
// The prefix must name the service that OWNS the resource, not the prefix the
// path used to be dispatched from. Alerting resources (alert-profiles,
// alert-rules, escalation-policies, oncall-schedules, sgl-alert-rules) use
// "/alerting"; dashboards and reports use "/analytics". Those all used
// "/monitoring" until the API gateway stopped splitting that single prefix
// across three services via hand-maintained path regexes. The gateway still
// rewrites the old paths for provider versions already in the wild, so this is
// backward compatible, but new resources should use the owning service.
func (c *Client) Tenantf(servicePrefix, format string, args ...any) string {
	suffix := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s/tenants/%s%s", servicePrefix, c.tenantID, suffix)
}

// Do issues an HTTP request. body, when non-nil, is JSON-encoded. When out is
// non-nil and the response carries a JSON body, it is decoded into out. query,
// when non-nil, is appended as the query string.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	return c.do(ctx, method, path, query, body, out, nil)
}

// Managed-write headers. The server only honours them on API-key calls.
const (
	HeaderManagedBy = "X-Watchpoint-Managed-By"
	HeaderWorkspace = "X-Watchpoint-Terraform-Workspace"
	HeaderAddress   = "X-Watchpoint-Terraform-Address"
)

// DoManaged is Do plus the headers that mark a monitor as managed by Terraform.
// address is the resource type name (Terraform does not pass addresses to providers).
func (c *Client) DoManaged(ctx context.Context, method, path string, query url.Values, body, out any, address string) error {
	return c.do(ctx, method, path, query, body, out, map[string]string{
		HeaderManagedBy: "terraform",
		HeaderWorkspace: c.workspace,
		HeaderAddress:   address,
	})
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any, extra map[string]string) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("performing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Method:     method,
			Path:       path,
			Message:    extractMessage(respBody),
			Body:       string(respBody),
		}
	}

	if out != nil && len(bytes.TrimSpace(respBody)) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decoding response from %s %s: %w", method, path, err)
		}
	}
	return nil
}

// Get is a convenience wrapper for GET requests.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	return c.Do(ctx, http.MethodGet, path, query, nil, out)
}

// Post is a convenience wrapper for POST requests.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.Do(ctx, http.MethodPost, path, nil, body, out)
}

// Patch is a convenience wrapper for PATCH requests.
func (c *Client) Patch(ctx context.Context, path string, body, out any) error {
	return c.Do(ctx, http.MethodPatch, path, nil, body, out)
}

// Put is a convenience wrapper for PUT requests.
func (c *Client) Put(ctx context.Context, path string, body, out any) error {
	return c.Do(ctx, http.MethodPut, path, nil, body, out)
}

// Delete is a convenience wrapper for DELETE requests.
func (c *Client) Delete(ctx context.Context, path string) error {
	return c.Do(ctx, http.MethodDelete, path, nil, nil, nil)
}

// PostManaged is Post with the managed-by-Terraform headers.
func (c *Client) PostManaged(ctx context.Context, path string, body, out any, address string) error {
	return c.DoManaged(ctx, http.MethodPost, path, nil, body, out, address)
}

// PatchManaged is Patch with the managed-by-Terraform headers.
func (c *Client) PatchManaged(ctx context.Context, path string, body, out any, address string) error {
	return c.DoManaged(ctx, http.MethodPatch, path, nil, body, out, address)
}

// PutManaged is Put with the managed-by-Terraform headers.
func (c *Client) PutManaged(ctx context.Context, path string, body, out any, address string) error {
	return c.DoManaged(ctx, http.MethodPut, path, nil, body, out, address)
}

// DeleteManaged is Delete with the managed-by-Terraform headers.
func (c *Client) DeleteManaged(ctx context.Context, path, address string) error {
	return c.DoManaged(ctx, http.MethodDelete, path, nil, nil, nil, address)
}

// extractMessage tries to pull a human-readable message out of a JSON error
// body of the common shapes {"error":"..."} or {"message":"..."}.
func extractMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	for _, key := range []string{"error", "message", "detail", "title"} {
		if raw, ok := m[key]; ok {
			var s string
			if json.Unmarshal(raw, &s) == nil && s != "" {
				return s
			}
		}
	}
	return ""
}
