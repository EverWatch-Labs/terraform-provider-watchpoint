package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The billing API reads an OMITTED field on the PUT as "leave unchanged". A
// body that dropped either field would silently adopt whatever the tenant had
// configured in the portal instead of applying the configuration, and the apply
// would then fail because the response disagreed with the plan.
func TestUsageAnomalyUpsertBodySendsBothFields(t *testing.T) {
	m := usageAnomalyAlertModel{
		IsEnabled:   types.BoolValue(true),
		Sensitivity: types.Float64Value(2.5),
	}
	body := m.toUpsertBody()

	if body.IsEnabled == nil || !*body.IsEnabled {
		t.Fatalf("isEnabled not sent: %v", body.IsEnabled)
	}
	if body.Sensitivity == nil || *body.Sensitivity != 2.5 {
		t.Fatalf("sensitivity not sent: %v", body.Sensitivity)
	}
}

// Delete turns the alerts off and must not touch sensitivity on the way out:
// a body carrying a zero sensitivity would be clamped up to the API's floor of
// 1.5, quietly rewriting a figure the tenant tuned.
func TestUsageAnomalyDisableBodyLeavesSensitivityAlone(t *testing.T) {
	body := usageAnomalyDisableBody()

	if body.IsEnabled == nil || *body.IsEnabled {
		t.Fatalf("expected isEnabled=false, got %v", body.IsEnabled)
	}
	if body.Sensitivity != nil {
		t.Fatalf("expected sensitivity to be omitted, got %v", *body.Sensitivity)
	}
}

// A setting changed in the portal is real drift: applyResponse must take the
// server's values so the next plan proposes putting them back, and must key the
// id on the tenant rather than leaving it unknown after apply.
func TestUsageAnomalyApplyResponseTakesServerValues(t *testing.T) {
	m := usageAnomalyAlertModel{
		IsEnabled:   types.BoolValue(true),
		Sensitivity: types.Float64Value(3.0),
	}
	m.applyResponse("tnt_1", &usageAnomalyAlertResponse{IsEnabled: false, Sensitivity: 4.5})

	if m.IsEnabled.ValueBool() {
		t.Error("is_enabled kept the config value instead of the server's")
	}
	if m.Sensitivity.ValueFloat64() != 4.5 {
		t.Errorf("sensitivity kept the config value: %v", m.Sensitivity.ValueFloat64())
	}
	if m.ID.ValueString() != "tnt_1" {
		t.Errorf("id not set from the tenant: %q", m.ID.ValueString())
	}
}

// allocate_shared_cost is Required precisely because the API rejects an absent
// value, so the body must always carry a concrete bool, including false, which
// is a real convention and not an "unset".
func TestCostAllocationSettingsBodyCarriesFalse(t *testing.T) {
	m := costAllocationSettingsModel{AllocateSharedCost: types.BoolValue(false)}
	if m.toBody().AllocateSharedCost {
		t.Fatal("false was not sent as false")
	}

	m.AllocateSharedCost = types.BoolValue(true)
	if !m.toBody().AllocateSharedCost {
		t.Fatal("true was not sent as true")
	}
}

// Same drift rule as the anomaly resource: a convention flipped in the portal
// must land in state rather than being masked by the configured value.
func TestCostAllocationSettingsApplyResponseTakesServerValue(t *testing.T) {
	m := costAllocationSettingsModel{AllocateSharedCost: types.BoolValue(true)}
	m.applyResponse("tnt_1", &costAllocationSettingsResponse{AllocateSharedCost: false})

	if m.AllocateSharedCost.ValueBool() {
		t.Error("allocate_shared_cost kept the config value instead of the server's")
	}
	if m.ID.ValueString() != "tnt_1" {
		t.Errorf("id not set from the tenant: %q", m.ID.ValueString())
	}
}
