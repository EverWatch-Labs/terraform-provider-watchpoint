package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// PUT replaces the whole pipeline: an omitted list goes as [], the operator's
// JSON spelling survives the API's re-serialisation, and destroy clears both.
func TestLogPipelineSingleton(t *testing.T) {
	f := newFakeAPI(t)
	path := "/observability/otlp/log-pipeline/" + testTenant
	stored := map[string]any{
		"processors": []any{map[string]any{"type": "drop", "name": "debug", "enabled": true, "filter": "severity:debug", "keepRate": 0}},
		"piiRules":   []any{},
	}
	f.on("PUT "+path, ok(stored))
	f.on("GET "+path, ok(stored))

	procs := `[{"type":"drop","name":"debug","enabled":true,"filter":"severity:debug"}]`
	got := tfCreate(t, f, NewLogPipelineResource(), logPipelineModel{ProcessorsJSON: types.StringValue(procs), PiiRulesJSON: types.StringNull()})
	if rules, isList := f.last("PUT " + path).Body["piiRules"].([]any); !isList || len(rules) != 0 {
		t.Fatal("an omitted pii_rules_json must be sent as [] so PUT clears stale rules")
	}
	if got.ProcessorsJSON.ValueString() != procs || !got.PiiRulesJSON.IsNull() || got.ID.ValueString() != testTenant {
		t.Fatalf("state = %+v", got)
	}

	tfDelete(t, f, NewLogPipelineResource(), got)
	del := f.last("PUT " + path).Body
	if len(del["processors"].([]any)) != 0 || len(del["piiRules"].([]any)) != 0 {
		t.Fatalf("destroy must PUT empty lists, sent %v", del)
	}
}

// The archive credential is never returned, so a refresh keeps the configured
// value and the next PUT still carries it.
func TestLogArchiveDestinationKeepsCredential(t *testing.T) {
	f := newFakeAPI(t)
	path := "/observability/otlp/log-archive/" + testTenant
	dest := map[string]any{"type": "s3", "baseUrl": "https://logs.s3.us-east-1.amazonaws.com", "region": "us-east-1", "enabled": true}
	f.on("PUT "+path, ok(dest))
	f.on("GET "+path, ok(map[string]any{"destination": dest}))
	f.on("DELETE "+path, ok(nil))

	plan := logArchiveDestinationModel{
		Type: types.StringValue("s3"), Bucket: types.StringValue("logs"), Region: types.StringValue("us-east-1"),
		AccessKeyID: types.StringValue("AKIAEXAMPLE"), SecretAccessKey: types.StringValue("s3cr3t"), Enabled: types.BoolValue(true),
	}
	got := tfCreate(t, f, NewLogArchiveDestinationResource(), plan)
	if f.last("PUT " + path).Body["secretAccessKey"] != "s3cr3t" {
		t.Fatal("secret not sent")
	}
	read, removed := tfRead(t, f, NewLogArchiveDestinationResource(), got)
	if removed || read.SecretAccessKey.ValueString() != "s3cr3t" || read.BaseURL.ValueString() == "" {
		t.Fatalf("read = %+v (removed %v)", read, removed)
	}

	// No destination configured any more: the resource drops out of state.
	f.on("GET "+path, ok(map[string]any{"destination": nil}))
	if _, removed := tfRead(t, f, NewLogArchiveDestinationResource(), got); !removed {
		t.Fatal("a cleared destination must be removed from state")
	}
	tfDelete(t, f, NewLogArchiveDestinationResource(), got)
}

// Write-only cloud credentials stay as configured across a refresh (the AWS key
// id comes back masked and is not decoded), and a vanished integration drops out.
func TestCloudIntegrationCredentialsAndRemoval(t *testing.T) {
	f := newFakeAPI(t)
	base := "/observability/otlp/cloud-integrations/" + testTenant
	row := map[string]any{"id": "ci1", "provider": "aws", "name": "AWS", "enabled": true,
		"resourceTypes": []string{"rds", "ec2"}, "regions": []string{"us-east-1"}, "accessKeyId": "AKIA****MPLE", "hasSecret": true}
	f.on("POST "+base, ok(row))
	f.on("GET "+base, ok(map[string]any{"integrations": []any{row}}))
	f.on("PUT "+base+"/ci1", ok(row))

	ctx := context.Background()
	rts, _ := types.SetValueFrom(ctx, types.StringType, []string{"ec2", "rds"})
	got := tfCreate(t, f, NewCloudIntegrationResource(), cloudIntegrationModel{
		Provider: types.StringValue("aws"), ResourceTypes: rts, Regions: types.SetNull(types.StringType),
		AccessKeyID: types.StringValue("AKIAEXAMPLE"), SecretAccessKey: types.StringValue("secret"), Enabled: types.BoolValue(true),
	})
	body := f.last("POST " + base).Body
	if body["provider"] != "aws" || body["secretAccessKey"] != "secret" || len(body["resourceTypes"].([]any)) != 2 {
		t.Fatalf("body = %v", body)
	}
	read, _ := tfRead(t, f, NewCloudIntegrationResource(), got)
	if read.AccessKeyID.ValueString() != "AKIAEXAMPLE" || read.SecretAccessKey.ValueString() != "secret" || !read.HasSecret.ValueBool() {
		t.Fatalf("credentials must stay as configured, got %+v", read)
	}

	f.on("GET "+base, ok(map[string]any{"integrations": []any{}}))
	if _, removed := tfRead(t, f, NewCloudIntegrationResource(), read); !removed {
		t.Fatal("a deleted integration must be removed from state")
	}
}
