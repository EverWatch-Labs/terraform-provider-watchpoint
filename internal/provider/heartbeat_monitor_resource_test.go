package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The ping token and URL are the whole point of the resource: a pipeline reads
// them to wire up the job. They must survive the response mapping, and the
// optionals the API returns as null must stay null rather than becoming "".
func TestHeartbeatMonitorApplyResponse(t *testing.T) {
	var diags diag.Diagnostics
	tags := "cron,billing"
	lastCheck := "2026-09-04T00:00:00Z"

	var m heartbeatMonitorModel
	m.applyResponse(context.Background(), &heartbeatMonitorResponse{
		ID:                    "mon_hb_1",
		Name:                  "Nightly invoicing",
		Status:                "Active",
		PingToken:             "tok_43_chars",
		PingURL:               "https://api.example.com/api/v1/monitoring/heartbeat/tok_43_chars",
		ExpectedPeriodSeconds: 86400,
		GracePeriodSeconds:    600,
		Tags:                  &tags,
		LastCheckAt:           &lastCheck,
		CreatedAt:             "2026-09-01T00:00:00Z",
	}, &diagSink{d: &diags})

	if m.PingToken.ValueString() != "tok_43_chars" {
		t.Fatalf("PingToken = %q, want tok_43_chars", m.PingToken.ValueString())
	}
	if m.PingURL.IsNull() {
		t.Fatal("PingURL is null, want the ingest URL")
	}
	if m.ExpectedPeriodSeconds.ValueInt64() != 86400 || m.GracePeriodSeconds.ValueInt64() != 600 {
		t.Fatalf("timings = %d/%d, want 86400/600", m.ExpectedPeriodSeconds.ValueInt64(), m.GracePeriodSeconds.ValueInt64())
	}
	if len(m.Tags.Elements()) != 2 {
		t.Fatalf("len(Tags) = %d, want 2", len(m.Tags.Elements()))
	}
	// A monitor that has never pinged reports null, not a zero timestamp.
	if !m.LastCheckStatus.IsNull() || !m.AlertProfileID.IsNull() || !m.SlaProfileID.IsNull() {
		t.Fatal("absent optionals must map to null")
	}
}

// A removed optional has to reach the API as "" (its clear signal), and the
// token must only rotate when rotate_trigger actually changed.
func TestHeartbeatMonitorUpdateBodyClearsAndRotates(t *testing.T) {
	var diags diag.Diagnostics
	m := heartbeatMonitorModel{
		Name:                  types.StringValue("Nightly invoicing"),
		ExpectedPeriodSeconds: types.Int64Value(3600),
		GracePeriodSeconds:    types.Int64Value(300),
		LocationProfileID:     types.StringNull(),
		ThresholdProfileID:    types.StringNull(),
		AlertProfileID:        types.StringNull(),
		GroupID:               types.StringNull(),
		Tags:                  types.SetNull(types.StringType),
	}

	body := m.toUpdateBody(context.Background(), false, &diagSink{d: &diags})
	for name, got := range map[string]*string{
		"locationProfileId":  body.LocationProfileID,
		"thresholdProfileId": body.ThresholdProfileID,
		"alertProfileId":     body.AlertProfileID,
		"groupId":            body.GroupID,
		"tags":               body.Tags,
	} {
		if got == nil || *got != "" {
			t.Fatalf("%s = %v, want a pointer to the empty string so the API clears it", name, got)
		}
	}
	if body.RotateToken != nil {
		t.Fatalf("RotateToken = %v, want nil when rotate_trigger did not change", *body.RotateToken)
	}

	if rotated := m.toUpdateBody(context.Background(), true, &diagSink{d: &diags}); rotated.RotateToken == nil || !*rotated.RotateToken {
		t.Fatal("RotateToken must be true when rotate_trigger changed")
	}
	if body.CronExpression == nil || *body.CronExpression != "" || body.TimeZone == nil || *body.TimeZone != "" {
		t.Fatal("a removed cron schedule must reach the API as \"\" so it falls back to the fixed period")
	}
}

// A cron schedule is sent on create, sla_profile_id is attached through the
// SLA endpoint (which now covers heartbeats), and the next expected run is exported.
func TestHeartbeatMonitorCronAndSla(t *testing.T) {
	f := newFakeAPI(t)
	base := tenantPath("/monitoring", "/monitors/heartbeat")
	f.on("POST "+base, ok(map[string]any{"id": "hb1", "pingToken": "tok", "pingUrl": "https://x/tok"}))
	f.on("PUT "+tenantPath("/monitoring", "/monitors/hb1/sla-profile"), ok(nil))
	f.on("GET "+base+"/hb1", ok(map[string]any{"id": "hb1", "name": "nightly", "status": "Active",
		"pingToken": "tok", "pingUrl": "https://x/tok", "expectedPeriodSeconds": 3600, "gracePeriodSeconds": 300,
		"createdAt": "2026-10-01T00:00:00Z", "slaProfileId": "sla1", "cronExpression": "0 2 * * 1-5",
		"timeZone": "Europe/Berlin", "nextExpectedAt": "2026-10-05T00:00:00Z"}))

	got := tfCreate(t, f, NewHeartbeatMonitorResource(), heartbeatMonitorModel{
		Name: types.StringValue("nightly"), ExpectedPeriodSeconds: types.Int64Value(3600),
		GracePeriodSeconds: types.Int64Value(300), Tags: types.SetNull(types.StringType),
		SlaProfileID: types.StringValue("sla1"), CronExpression: types.StringValue("0 2 * * 1-5"),
		TimeZone: types.StringValue("Europe/Berlin"),
	})
	body := f.last("POST " + base).Body
	if body["cronExpression"] != "0 2 * * 1-5" || body["timeZone"] != "Europe/Berlin" {
		t.Fatalf("create body = %v", body)
	}
	if f.last("PUT " + tenantPath("/monitoring", "/monitors/hb1/sla-profile")).Body["slaProfileId"] != "sla1" {
		t.Fatal("sla_profile_id must be attached on create")
	}
	if got.NextExpectedAt.ValueString() != "2026-10-05T00:00:00Z" {
		t.Fatalf("next_expected_at = %q", got.NextExpectedAt.ValueString())
	}
}
