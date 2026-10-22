package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func capture(t *testing.T) (*httptest.Server, *http.Header) {
	t.Helper()
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestManagedWritesSendTerraformHeaders(t *testing.T) {
	srv, got := capture(t)
	c := New(Config{BaseURL: srv.URL, APIKey: "k", TenantID: "t", Workspace: "prod"})
	calls := map[string]func() error{
		"post": func() error {
			return c.PostManaged(context.Background(), "/x", map[string]any{}, nil, "watchpoint_http_monitor")
		},
		"patch": func() error {
			return c.PatchManaged(context.Background(), "/x", map[string]any{}, nil, "watchpoint_http_monitor")
		},
		"put": func() error {
			return c.PutManaged(context.Background(), "/x", map[string]any{}, nil, "watchpoint_http_monitor")
		},
		"delete": func() error { return c.DeleteManaged(context.Background(), "/x", "watchpoint_http_monitor") },
	}
	for name, call := range calls {
		if err := call(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Get(HeaderManagedBy) != "terraform" || got.Get(HeaderWorkspace) != "prod" ||
			got.Get(HeaderAddress) != "watchpoint_http_monitor" || got.Get("X-API-Key") != "k" {
			t.Fatalf("%s headers = %v", name, *got)
		}
	}
}

func TestUnmanagedWritesOmitTerraformHeaders(t *testing.T) {
	srv, got := capture(t)
	c := New(Config{BaseURL: srv.URL, APIKey: "k", TenantID: "t"})
	if err := c.Patch(context.Background(), "/x", map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	if got.Get(HeaderManagedBy) != "" || got.Get(HeaderWorkspace) != "" || got.Get(HeaderAddress) != "" {
		t.Fatalf("plain writes must not carry managed headers: %v", *got)
	}
}

func TestWorkspaceDefaults(t *testing.T) {
	if ws := New(Config{}).Workspace(); ws != "default" {
		t.Fatalf("Workspace() = %q, want default", ws)
	}
	if ws := New(Config{Workspace: "  staging "}).Workspace(); ws != "staging" {
		t.Fatalf("Workspace() = %q, want staging", ws)
	}
}
