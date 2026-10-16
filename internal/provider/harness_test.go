package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Acceptance-style harness. A resource runs its real Create/Read/Update/Delete
// through the plugin framework (plan and state go through the schema, so a
// tfsdk tag or type mismatch fails here exactly as it would under Terraform)
// against an httptest server standing in for the Watchpoint API. Each test
// declares the routes it expects and asserts on the bodies the provider sent.

const testTenant = "t1"

type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Body   map[string]any
	Raw    string
}

type fakeAPI struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	routes   map[string]func(r recordedRequest) (int, any)
	requests []recordedRequest
}

// newFakeAPI starts a server; routes are keyed "METHOD /path" with the /api/v1
// base already stripped (the client is built with the server URL as base).
func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{t: t, routes: map[string]func(recordedRequest) (int, any){}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) on(methodAndPath string, h func(r recordedRequest) (int, any)) {
	f.routes[methodAndPath] = h
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	rec := recordedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Raw: string(raw)}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &rec.Body)
	}
	f.mu.Lock()
	f.requests = append(f.requests, rec)
	h, ok := f.routes[r.Method+" "+r.URL.Path]
	f.mu.Unlock()
	if !ok {
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
	status, body := h(rec)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

// last returns the most recent request for "METHOD /path", failing the test when
// none was made.
func (f *fakeAPI) last(methodAndPath string) recordedRequest {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.requests) - 1; i >= 0; i-- {
		if f.requests[i].Method+" "+f.requests[i].Path == methodAndPath {
			return f.requests[i]
		}
	}
	f.t.Fatalf("no %s request was made", methodAndPath)
	return recordedRequest{}
}

func (f *fakeAPI) count(methodAndPath string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if r.Method+" "+r.Path == methodAndPath {
			n++
		}
	}
	return n
}

func (f *fakeAPI) client() *client.Client {
	return client.New(client.Config{BaseURL: f.srv.URL, APIKey: "wp_live_test", TenantID: testTenant})
}

// ok is a route handler returning a fixed 200 body.
func ok(body any) func(recordedRequest) (int, any) {
	return func(recordedRequest) (int, any) { return http.StatusOK, body }
}

func configured(t *testing.T, f *fakeAPI, r resource.Resource) resource.Resource {
	t.Helper()
	var resp resource.ConfigureResponse
	r.(resource.ResourceWithConfigure).Configure(context.Background(),
		resource.ConfigureRequest{ProviderData: f.client()}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("configure: %v", resp.Diagnostics)
	}
	return r
}

func schemaOf(t *testing.T, r resource.Resource) (tfsdk.State, tfsdk.Plan) {
	t.Helper()
	var sr resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &sr)
	typ := sr.Schema.Type().TerraformType(context.Background())
	return tfsdk.State{Schema: sr.Schema, Raw: tftypes.NewValue(typ, nil)},
		tfsdk.Plan{Schema: sr.Schema, Raw: tftypes.NewValue(typ, nil)}
}

// tfCreate runs Create with plan as the planned value and returns the state.
func tfCreate[M any](t *testing.T, f *fakeAPI, r resource.Resource, plan M) M {
	t.Helper()
	ctx := context.Background()
	r = configured(t, f, r)
	state, p := schemaOf(t, r)
	if d := p.Set(ctx, &plan); d.HasError() {
		t.Fatalf("plan set: %v", d)
	}
	resp := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: p}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create: %v", resp.Diagnostics)
	}
	var out M
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("state get: %v", d)
	}
	return out
}

// tfRead runs Read; removed reports whether the resource dropped itself.
func tfRead[M any](t *testing.T, f *fakeAPI, r resource.Resource, prior M) (out M, removed bool) {
	t.Helper()
	ctx := context.Background()
	r = configured(t, f, r)
	state, _ := schemaOf(t, r)
	if d := state.Set(ctx, &prior); d.HasError() {
		t.Fatalf("state set: %v", d)
	}
	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		return out, true
	}
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("state get: %v", d)
	}
	return out, false
}

// tfUpdate runs Update from prior state to plan.
func tfUpdate[M any](t *testing.T, f *fakeAPI, r resource.Resource, prior, plan M) M {
	t.Helper()
	ctx := context.Background()
	r = configured(t, f, r)
	state, p := schemaOf(t, r)
	if d := state.Set(ctx, &prior); d.HasError() {
		t.Fatalf("state set: %v", d)
	}
	if d := p.Set(ctx, &plan); d.HasError() {
		t.Fatalf("plan set: %v", d)
	}
	resp := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: p, State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("update: %v", resp.Diagnostics)
	}
	var out M
	if d := resp.State.Get(ctx, &out); d.HasError() {
		t.Fatalf("state get: %v", d)
	}
	return out
}

// tfDelete runs Delete and fails on any diagnostic.
func tfDelete[M any](t *testing.T, f *fakeAPI, r resource.Resource, prior M) {
	t.Helper()
	ctx := context.Background()
	r = configured(t, f, r)
	state, _ := schemaOf(t, r)
	if d := state.Set(ctx, &prior); d.HasError() {
		t.Fatalf("state set: %v", d)
	}
	resp := resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("delete: %v", resp.Diagnostics)
	}
}

// tfDataRead runs a data source Read with the given config model.
func tfDataRead[M any](t *testing.T, f *fakeAPI, d datasource.DataSource, cfg M) M {
	t.Helper()
	ctx := context.Background()
	var cr datasource.ConfigureResponse
	d.(datasource.DataSourceWithConfigure).Configure(ctx, datasource.ConfigureRequest{ProviderData: f.client()}, &cr)
	var sr datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &sr)
	typ := sr.Schema.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: sr.Schema, Raw: tftypes.NewValue(typ, nil)}
	state := tfsdk.State{Schema: sr.Schema, Raw: tftypes.NewValue(typ, nil)}
	if dg := state.Set(ctx, &cfg); dg.HasError() {
		t.Fatalf("config set: %v", dg)
	}
	config.Raw = state.Raw
	resp := datasource.ReadResponse{State: state}
	d.Read(ctx, datasource.ReadRequest{Config: config}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("data read: %v", resp.Diagnostics)
	}
	var out M
	if dg := resp.State.Get(ctx, &out); dg.HasError() {
		t.Fatalf("state get: %v", dg)
	}
	return out
}

// tenantPath mirrors client.Tenantf for route keys in tests.
func tenantPath(service, suffix string) string {
	return service + "/tenants/" + testTenant + suffix
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("%q does not contain %q", haystack, needle)
	}
}
