package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestAPIKeyApplyCreated verifies the create/roll response maps onto state,
// including the plaintext secret and the CSV scope string becoming a set.
func TestAPIKeyApplyCreated(t *testing.T) {
	expires := "2026-06-01T00:00:00Z"
	resp := &apiKeyCreatedResponse{
		ID:        "apikey_1",
		Name:      "ci-pipeline",
		KeyPrefix: "wp_live_abc123def456...",
		Scopes:    "monitors.read,deployments.write",
		CreatedAt: "2026-01-01T00:00:00Z",
		ExpiresAt: &expires,
		APIKey:    "wp_live_abc123def4567890abcdef1234567890abcdef1234567890abcdef12345678",
	}

	var diags diag.Diagnostics
	ds := &diagSink{d: &diags}
	var m apiKeyModel
	m.applyCreated(context.Background(), resp, ds)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if m.ID.ValueString() != "apikey_1" {
		t.Fatalf("ID = %q, want apikey_1", m.ID.ValueString())
	}
	if m.APIKey.ValueString() != resp.APIKey {
		t.Fatalf("APIKey was not carried onto state")
	}
	if m.KeyPrefix.ValueString() != resp.KeyPrefix {
		t.Fatalf("KeyPrefix = %q, want %q", m.KeyPrefix.ValueString(), resp.KeyPrefix)
	}
	if m.ExpiresAt.ValueString() != expires {
		t.Fatalf("ExpiresAt = %q, want %q", m.ExpiresAt.ValueString(), expires)
	}

	var scopes []string
	diags.Append(m.Scopes.ElementsAs(context.Background(), &scopes, false)...)
	if len(scopes) != 2 {
		t.Fatalf("Scopes length = %d, want 2 (%v)", len(scopes), scopes)
	}
}

// TestAPIKeyApplyReadPreservesSecret is the important one. The list endpoint
// cannot return the plaintext key, so a Read must never clobber the secret
// already held in state. Losing it here would silently break every downstream
// reference on the next refresh.
func TestAPIKeyApplyReadPreservesSecret(t *testing.T) {
	var diags diag.Diagnostics
	ds := &diagSink{d: &diags}

	m := apiKeyModel{
		APIKey:        types.StringValue("wp_live_secret_from_create"),
		RotateTrigger: types.StringValue("2026-q1"),
	}
	lastUsed := "2026-02-01T12:00:00Z"
	m.applyRead(context.Background(), &apiKeyResponse{
		ID:         "apikey_1",
		Name:       "ci-pipeline",
		KeyPrefix:  "wp_live_abc123def456...",
		Scopes:     "*",
		IsActive:   false,
		CreatedAt:  "2026-01-01T00:00:00Z",
		LastUsedAt: &lastUsed,
	}, ds)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if m.APIKey.ValueString() != "wp_live_secret_from_create" {
		t.Fatalf("Read clobbered the secret: got %q", m.APIKey.ValueString())
	}
	if m.RotateTrigger.ValueString() != "2026-q1" {
		t.Fatalf("Read clobbered rotate_trigger: got %q", m.RotateTrigger.ValueString())
	}
	if m.Enabled.ValueBool() {
		t.Fatalf("Enabled = true, want false (isActive was false)")
	}
	if m.LastUsedAt.ValueString() != lastUsed {
		t.Fatalf("LastUsedAt = %q, want %q", m.LastUsedAt.ValueString(), lastUsed)
	}
}

// TestAPIKeyApplyReadNullExpiry covers a non-expiring key: expires_at and
// last_used_at must land as null rather than an empty string, or the plan shows
// a spurious diff on every refresh.
func TestAPIKeyApplyReadNullExpiry(t *testing.T) {
	var diags diag.Diagnostics
	ds := &diagSink{d: &diags}

	var m apiKeyModel
	m.applyRead(context.Background(), &apiKeyResponse{
		ID:        "apikey_2",
		Name:      "never-expires",
		KeyPrefix: "wp_live_zzz...",
		Scopes:    "*",
		IsActive:  true,
		CreatedAt: "2026-01-01T00:00:00Z",
	}, ds)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !m.ExpiresAt.IsNull() {
		t.Fatalf("ExpiresAt = %q, want null", m.ExpiresAt.ValueString())
	}
	if !m.LastUsedAt.IsNull() {
		t.Fatalf("LastUsedAt = %q, want null", m.LastUsedAt.ValueString())
	}
	if !m.Enabled.ValueBool() {
		t.Fatalf("Enabled = false, want true")
	}
}

// TestAPIKeyScopesRoundTrip pins the set/CSV conversion in both directions,
// since the API takes and returns a comma string while the schema exposes an
// unordered set.
func TestAPIKeyScopesRoundTrip(t *testing.T) {
	var diags diag.Diagnostics
	ctx := context.Background()

	set := setFromStringSlice(ctx, []string{"monitors.read", "alerts.read"}, &diags)
	csv := csvPtrFromSet(ctx, set, &diags)
	if csv == nil {
		t.Fatal("csvPtrFromSet returned nil for a populated set")
	}
	back := setFromCSVPtr(ctx, csv, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	var out []string
	diags.Append(back.ElementsAs(ctx, &out, false)...)
	if len(out) != 2 {
		t.Fatalf("round-trip length = %d, want 2 (%v)", len(out), out)
	}

	// An omitted scope set must produce no JSON field, so the API applies its
	// own "*" default rather than receiving an empty string.
	if csvPtrFromSet(ctx, types.SetNull(types.StringType), &diags) != nil {
		t.Fatal("a null scope set should omit the field, not send an empty value")
	}
}
