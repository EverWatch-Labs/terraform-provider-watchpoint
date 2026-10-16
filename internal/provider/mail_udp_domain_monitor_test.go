package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func commonMonitorRow(id string) map[string]any {
	return map[string]any{"id": id, "name": "mx", "status": "Active", "checkIntervalSeconds": 300,
		"timeoutSeconds": 30, "slaProfileId": "sla1", "createdAt": "2026-10-01T00:00:00Z"}
}

func withRow(base map[string]any, extra map[string]any) map[string]any {
	for k, v := range extra {
		base[k] = v
	}
	return base
}

func newCommon(name string) monitorCommonModel {
	return monitorCommonModel{
		Name: types.StringValue(name), CheckIntervalSeconds: types.Int64Value(300), TimeoutSeconds: types.Int64Value(30),
		SlaProfileID: types.StringValue("sla1"), Tags: types.SetNull(types.StringType),
	}
}

// The mail password is write-only: it is sent, kept in state although the API
// never returns it, and removing username / threshold on update must clear them.
func TestMailMonitorLifecycle(t *testing.T) {
	f := newFakeAPI(t)
	base := tenantPath("/monitoring", "/monitors/mail")
	row := withRow(commonMonitorRow("m1"), map[string]any{"host": "mx.example.com", "port": 587,
		"protocol": "Smtp", "tlsMode": "StartTls", "username": "probe", "hasPassword": true,
		"validateCertificate": true, "maxResponseTimeMs": 2000})
	f.on("POST "+base, ok(map[string]any{"id": "m1", "name": "mx"}))
	f.on("PUT "+tenantPath("/monitoring", "/monitors/m1/sla-profile"), ok(nil))
	f.on("GET "+base+"/m1", func(recordedRequest) (int, any) { return http.StatusOK, row })
	f.on("PATCH "+base+"/m1", ok(map[string]any{"message": "ok"}))

	plan := mailMonitorModel{
		monitorCommonModel: newCommon("mx"),
		Host:               types.StringValue("mx.example.com"), Port: types.Int64Value(587),
		Protocol: types.StringValue("Smtp"), TLSMode: types.StringValue("StartTls"),
		Username: types.StringValue("probe"), Password: types.StringValue("s3cret"),
		ValidateCertificate: types.BoolValue(true), MaxResponseTimeMs: types.Int64Value(2000),
	}
	got := tfCreate(t, f, NewMailMonitorResource(), plan)
	if f.last("POST " + base).Body["password"] != "s3cret" {
		t.Fatal("password must be sent on create")
	}
	if f.last("PUT " + tenantPath("/monitoring", "/monitors/m1/sla-profile")).Body["slaProfileId"] != "sla1" {
		t.Fatal("sla_profile_id must be attached through the attach endpoint")
	}
	if got.Password.ValueString() != "s3cret" || !got.HasPassword.ValueBool() {
		t.Fatal("the write-only password must stay in state")
	}

	next := got
	next.Username = types.StringNull()
	next.MaxResponseTimeMs = types.Int64Null()
	tfUpdate(t, f, NewMailMonitorResource(), got, next)
	patch := f.last("PATCH " + base + "/m1").Body
	if patch["username"] != "" || patch["maxResponseTimeMs"].(float64) != 0 {
		t.Fatalf("PATCH = %v, want username \"\" and maxResponseTimeMs 0 to clear", patch)
	}
	if patch["locationProfileId"] != "" {
		t.Fatalf("locationProfileId = %v, want \"\" so a removed profile is cleared", patch["locationProfileId"])
	}
	if f.count("PUT "+tenantPath("/monitoring", "/monitors/m1/sla-profile")) != 1 {
		t.Fatal("an unchanged SLA profile must not be re-attached")
	}
}

func TestUDPMonitorCreateSendsPayload(t *testing.T) {
	f := newFakeAPI(t)
	base := tenantPath("/monitoring", "/monitors/udp")
	f.on("POST "+base, ok(map[string]any{"id": "u1"}))
	f.on("PUT "+tenantPath("/monitoring", "/monitors/u1/sla-profile"), ok(nil))
	f.on("GET "+base+"/u1", ok(withRow(commonMonitorRow("u1"), map[string]any{"host": "ntp.example.com",
		"port": 123, "payload": "1b00", "payloadIsHex": true, "expectRegex": "^\\x1c"})))

	got := tfCreate(t, f, NewUDPMonitorResource(), udpMonitorModel{
		monitorCommonModel: newCommon("ntp"),
		Host:               types.StringValue("ntp.example.com"), Port: types.Int64Value(123),
		Payload: types.StringValue("1b00"), PayloadIsHex: types.BoolValue(true),
		ExpectRegex: types.StringValue("^\\x1c"),
	})
	body := f.last("POST " + base).Body
	if body["payloadIsHex"] != true || body["port"].(float64) != 123 {
		t.Fatalf("body = %v", body)
	}
	if got.ExpectContains.IsNull() == false {
		t.Fatal("an unset expectation must stay null")
	}
}

// The API stores the domain lower-cased; the configured spelling must survive.
func TestDomainMonitorKeepsConfiguredCase(t *testing.T) {
	f := newFakeAPI(t)
	base := tenantPath("/monitoring", "/monitors/domain")
	f.on("POST "+base, ok(map[string]any{"id": "d1"}))
	f.on("PUT "+tenantPath("/monitoring", "/monitors/d1/sla-profile"), ok(nil))
	f.on("GET "+base+"/d1", ok(withRow(commonMonitorRow("d1"), map[string]any{"domain": "example.com",
		"expiryWarningDays": 30, "expiryCriticalDays": 7, "alertOnRegistrarChange": true,
		"alertOnNameserverChange": false, "lastKnownRegistrar": "Example Registrar",
		"expiresAt": "2027-01-01T00:00:00Z"})))

	got := tfCreate(t, f, NewDomainMonitorResource(), domainMonitorModel{
		monitorCommonModel: newCommon("example"),
		Domain:             types.StringValue("Example.COM"),
		ExpiryWarningDays:  types.Int64Value(30), ExpiryCriticalDays: types.Int64Value(7),
		AlertOnRegistrarChange: types.BoolValue(true), AlertOnNameserverChange: types.BoolValue(false),
	})
	if got.Domain.ValueString() != "Example.COM" {
		t.Fatalf("domain = %q, want the configured spelling", got.Domain.ValueString())
	}
	if got.Registrar.ValueString() != "Example Registrar" || got.ExpiresAt.IsNull() {
		t.Fatal("registry facts must be exported")
	}
}
