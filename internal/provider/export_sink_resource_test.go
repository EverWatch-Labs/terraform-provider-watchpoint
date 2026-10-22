package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// An S3 sink sends only S3 settings and the credential; the write-only secret
// stays in state across a read.
func TestExportSinkS3(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/monitoring", "/export-sinks")
	row := map[string]any{"id": "sink_1", "name": "Lake", "kind": "s3", "enabled": true, "format": "ndjson",
		"prefix": "watchpoint", "datasets": []any{"monitors"}, "environmentIds": nil,
		"config":     map[string]any{"endpoint": nil, "bucket": "wp-export", "region": "us-east-1", "forcePathStyle": false},
		"secretHint": "cret", "scheduleHourUtc": 2}
	f.on("POST "+list, func(recordedRequest) (int, any) { return http.StatusCreated, row })
	f.on("GET "+list, ok(map[string]any{"items": []any{row}}))

	got := tfCreate(t, f, NewExportSinkResource(), exportSinkModel{
		Name: types.StringValue("Lake"), Kind: types.StringValue("s3"), Enabled: types.BoolValue(true),
		Format: types.StringValue("ndjson"), Prefix: types.StringValue("watchpoint"), Datasets: strSet("monitors"),
		EnvironmentIDs: types.SetNull(types.StringType), ScheduleHourUTC: types.Int64Value(2),
		Bucket: types.StringValue("wp-export"), Region: types.StringValue("us-east-1"), ForcePathStyle: types.BoolValue(false),
		AccountURL: types.StringValue("https://ignored.blob.core.windows.net"),
		Secret:     types.StringValue("AKIA:secret"),
	})
	b := f.last("POST " + list).Body
	cfg := b["config"].(map[string]any)
	if b["secret"] != "AKIA:secret" || cfg["bucket"] != "wp-export" || cfg["accountUrl"] != nil {
		t.Fatalf("post = %v", b)
	}
	after, removed := tfRead(t, f, NewExportSinkResource(), got)
	if removed || after.Secret.ValueString() != "AKIA:secret" || !after.EnvironmentIDs.IsNull() || after.SecretHint.ValueString() != "cret" {
		t.Fatalf("read = %+v", after)
	}
}
