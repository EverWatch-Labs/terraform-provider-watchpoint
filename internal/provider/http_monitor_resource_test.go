package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// environment_id travels with every monitor write and an emptied ignore regex is cleared with "".
func TestHTTPMonitorEnvironmentAndContent(t *testing.T) {
	f := newFakeAPI(t)
	base := tenantPath("/monitoring", "/monitors/http")
	row := map[string]any{"id": "h1", "name": "web", "status": "Active", "url": "https://x", "method": "GET",
		"followRedirects": true, "maxRedirects": 5, "expectedResponseTypeSeverity": "Warning", "validateSsl": true,
		"checkIntervalSeconds": 300, "timeoutSeconds": 30, "createdAt": "2026-10-01T00:00:00Z",
		"environmentId": "env_s", "contentChangeMode": "alert", "contentIgnoreRegex": `\d+`}
	f.on("POST "+base, ok(map[string]any{"id": "h1", "name": "web"}))
	f.on("GET "+base+"/h1", ok(row))
	f.on("PATCH "+base+"/h1", ok(map[string]any{"message": "ok"}))

	plan := httpMonitorModel{
		Name: types.StringValue("web"), URL: types.StringValue("https://x"), Method: types.StringValue("GET"),
		FollowRedirects: types.BoolValue(true), MaxRedirects: types.Int64Value(5),
		ExpectedResponseTypeSeverity: types.StringValue("Warning"), ValidateSSL: types.BoolValue(true),
		CheckIntervalSeconds: types.Int64Value(300), TimeoutSeconds: types.Int64Value(30), IsMultiStep: types.BoolValue(false),
		Tags: types.SetNull(types.StringType), EnvironmentID: types.StringValue("env_s"),
		ContentChangeMode: types.StringValue("alert"), ContentIgnoreRegex: types.StringValue(`\d+`),
	}
	got := tfCreate(t, f, NewHTTPMonitorResource(), plan)
	if b := f.last("POST " + base).Body; b["environmentId"] != "env_s" || b["contentChangeMode"] != "alert" {
		t.Fatalf("post = %v", b)
	}
	if got.EnvironmentID.ValueString() != "env_s" || got.ContentIgnoreRegex.ValueString() != `\d+` {
		t.Fatalf("state = %+v", got)
	}

	next := got
	next.ContentIgnoreRegex = types.StringNull()
	row["contentIgnoreRegex"] = nil
	tfUpdate(t, f, NewHTTPMonitorResource(), got, next)
	if v, present := f.last("PATCH " + base + "/h1").Body["contentIgnoreRegex"]; !present || v != "" {
		t.Fatalf("contentIgnoreRegex = %v, want \"\"", v)
	}
}
