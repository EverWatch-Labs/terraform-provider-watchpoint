package provider

import (
	"context"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func assertManaged(t *testing.T, r recordedRequest, address string) {
	t.Helper()
	if r.Header.Get("X-Watchpoint-Managed-By") != "terraform" ||
		r.Header.Get("X-Watchpoint-Terraform-Workspace") != "default" ||
		r.Header.Get("X-Watchpoint-Terraform-Address") != address {
		t.Fatalf("%s %s headers = %v, want the managed-by-terraform set for %s", r.Method, r.Path, r.Header, address)
	}
}

// Every monitor write, including the SLA attach, carries the managed headers; reads do not.
func TestMonitorWritesCarryManagedHeaders(t *testing.T) {
	f := newFakeAPI(t)
	base := tenantPath("/monitoring", "/monitors/mail")
	sla := tenantPath("/monitoring", "/monitors/m1/sla-profile")
	row := withRow(commonMonitorRow("m1"), map[string]any{"host": "mx.example.com", "port": 587,
		"protocol": "Smtp", "tlsMode": "StartTls", "validateCertificate": true})
	f.on("POST "+base, ok(map[string]any{"id": "m1", "name": "mx"}))
	f.on("PUT "+sla, ok(nil))
	f.on("GET "+base+"/m1", ok(row))
	f.on("PATCH "+base+"/m1", ok(map[string]any{"message": "ok"}))
	f.on("DELETE "+base+"/m1", ok(nil))

	plan := mailMonitorModel{
		monitorCommonModel: newCommon("mx"),
		Host:               types.StringValue("mx.example.com"), Port: types.Int64Value(587),
		Protocol: types.StringValue("Smtp"), TLSMode: types.StringValue("StartTls"),
		ValidateCertificate: types.BoolValue(true),
	}
	got := tfCreate(t, f, NewMailMonitorResource(), plan)
	assertManaged(t, f.last("POST "+base), "watchpoint_mail_monitor")
	assertManaged(t, f.last("PUT "+sla), "watchpoint_mail_monitor")
	if f.last("GET "+base+"/m1").Header.Get("X-Watchpoint-Managed-By") != "" {
		t.Fatal("reads must not carry the managed header")
	}

	next := got
	next.Port = types.Int64Value(465)
	got = tfUpdate(t, f, NewMailMonitorResource(), got, next)
	assertManaged(t, f.last("PATCH "+base+"/m1"), "watchpoint_mail_monitor")

	tfDelete(t, f, NewMailMonitorResource(), got)
	assertManaged(t, f.last("DELETE "+base+"/m1"), "watchpoint_mail_monitor")
}

// Import sends one empty managed PATCH so the server records the marker.
func TestMonitorImportStampsManagedMarker(t *testing.T) {
	f := newFakeAPI(t)
	path := tenantPath("/monitoring", "/monitors/http/h1")
	f.on("PATCH "+path, ok(map[string]any{"message": "ok"}))

	r := configured(t, f, NewHTTPMonitorResource())
	state, _ := schemaOf(t, r)
	resp := resource.ImportStateResponse{State: state}
	r.(resource.ResourceWithImportState).ImportState(context.Background(), resource.ImportStateRequest{ID: "h1"}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import: %v", resp.Diagnostics)
	}
	req := f.last("PATCH " + path)
	assertManaged(t, req, "watchpoint_http_monitor")
	if len(req.Body) != 0 {
		t.Fatalf("import PATCH body = %v, want {}", req.Body)
	}
}

// A failed stamp must not fail the import: the next apply records the marker.
func TestMonitorImportStampFailureWarns(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PATCH "+tenantPath("/monitoring", "/monitors/ssl/s1"), func(recordedRequest) (int, any) {
		return http.StatusConflict, map[string]any{"error": "conflict"}
	})
	r := configured(t, f, NewSSLMonitorResource())
	state, _ := schemaOf(t, r)
	resp := resource.ImportStateResponse{State: state}
	r.(resource.ResourceWithImportState).ImportState(context.Background(), resource.ImportStateRequest{ID: "s1"}, &resp)
	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 1 {
		t.Fatalf("diagnostics = %v, want exactly one warning", resp.Diagnostics)
	}
}

// Guard: no monitor resource may write through the unmarked client methods.
func TestMonitorResourcesUseManagedWrites(t *testing.T) {
	unmanaged := regexp.MustCompile(`client\.(Post|Patch|Put|Delete|Do)\(`)
	files := []string{"http", "ssl", "synthetic", "dns", "tcp", "ping", "heartbeat", "mail", "udp", "domain"}
	for _, k := range files {
		src, err := os.ReadFile(k + "_monitor_resource.go")
		if err != nil {
			t.Fatal(err)
		}
		if m := unmanaged.Find(src); m != nil {
			t.Errorf("%s_monitor_resource.go uses %s; monitor writes must use the *Managed client methods", k, m)
		}
		if !regexp.MustCompile(`importManagedMonitor\(`).Match(src) {
			t.Errorf("%s_monitor_resource.go must import through importManagedMonitor", k)
		}
	}
	common, _ := os.ReadFile("monitor_common.go")
	if m := unmanaged.Find(common); m != nil {
		t.Errorf("monitor_common.go uses %s", m)
	}
}

// workspace: attribute, else TF_WORKSPACE, else the client's "default".
func TestProviderWorkspaceResolution(t *testing.T) {
	var sr provider.SchemaResponse
	New("test")().Schema(context.Background(), provider.SchemaRequest{}, &sr)
	if _, ok := sr.Schema.Attributes["workspace"]; !ok {
		t.Fatal("provider schema must expose workspace")
	}
	t.Setenv("TF_WORKSPACE", "staging")
	if got := stringOrEnv(types.StringNull(), "TF_WORKSPACE"); got != "staging" {
		t.Fatalf("env fallback = %q", got)
	}
	if got := stringOrEnv(types.StringValue("prod"), "TF_WORKSPACE"); got != "prod" {
		t.Fatalf("attribute = %q, want it to win over the env", got)
	}
}
