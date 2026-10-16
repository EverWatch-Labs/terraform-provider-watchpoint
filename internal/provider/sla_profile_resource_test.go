package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The API drops source_config_json for a Monitor-sourced profile, so accepting
// it would fail the apply as an inconsistent result. It is rejected at plan
// time instead, including when source_kind is left to its Monitor default.
func TestSLAProfileSourceConfigNeedsTelemetryKind(t *testing.T) {
	ctx := context.Background()
	validate := func(kind types.String) bool {
		r := NewSLAProfileResource()
		state, _ := schemaOf(t, r)
		m := slaProfileModel{Name: types.StringValue("x"), TargetUptimePercent: types.Float64Value(99.9),
			WindowDays: types.Int64Value(30), SourceKind: kind,
			SourceConfigJSON: types.StringValue(`{"serviceName":"checkout"}`)}
		if d := state.Set(ctx, &m); d.HasError() {
			t.Fatalf("set: %v", d)
		}
		var resp resource.ValidateConfigResponse
		r.(resource.ResourceWithValidateConfig).ValidateConfig(ctx,
			resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: state.Schema, Raw: state.Raw}}, &resp)
		return resp.Diagnostics.HasError()
	}
	if !validate(types.StringValue("Monitor")) {
		t.Error("source_config_json with source_kind Monitor must be rejected")
	}
	if !validate(types.StringNull()) {
		t.Error("source_config_json with the default (Monitor) source must be rejected")
	}
	if validate(types.StringValue("Traces")) {
		t.Error("source_config_json with source_kind Traces must be accepted")
	}
	if validate(types.StringUnknown()) {
		t.Error("an unknown source_kind must not be rejected before it is known")
	}
}
