package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The API returns an unset optional as either null or "". Both must land in
// state as null, otherwise an apply against a config that omits the attribute
// fails with `was null, but now cty.StringVal("")`.
func TestRumSiteApplyResponseEmptyOptionalStaysNull(t *testing.T) {
	empty := ""
	value := "https://staging.example.com"

	for name, in := range map[string]*string{"nil": nil, "empty": &empty} {
		var m rumSiteModel
		m.applyResponse(&rumSiteResponse{AdditionalOrigins: in})
		if !m.AdditionalOrigins.IsNull() {
			t.Fatalf("%s response: expected null, got %q", name, m.AdditionalOrigins.ValueString())
		}
	}

	var m rumSiteModel
	m.applyResponse(&rumSiteResponse{AdditionalOrigins: &value})
	if m.AdditionalOrigins.ValueString() != value {
		t.Fatalf("expected %q, got %q", value, m.AdditionalOrigins.ValueString())
	}
}

// A null Optional attribute must be sent as "" on the PUT (not omitted), so
// removing it from config clears it instead of silently leaving the old value.
func TestRumSiteUpdateBodyClearsAdditionalOrigins(t *testing.T) {
	var m rumSiteModel
	m.applyResponse(&rumSiteResponse{})
	body := m.toUpdateBody()
	if body.AdditionalOrigins == nil || *body.AdditionalOrigins != "" {
		t.Fatalf("expected pointer to empty string, got %v", body.AdditionalOrigins)
	}
}

// The screenshot basic-auth pair follows the same rule: the RUM API reads null
// as "leave the stored value alone" and "" as "clear it", so a null attribute
// has to go out as "". Omitting it would leave a username and vault key on the
// site after they were deleted from configuration.
func TestRumSiteUpdateBodyClearsScreenshotAuth(t *testing.T) {
	var m rumSiteModel
	m.applyResponse(&rumSiteResponse{})
	body := m.toUpdateBody()
	if body.ScreenshotAuthUsername == nil || *body.ScreenshotAuthUsername != "" {
		t.Fatalf("expected pointer to empty string for the username, got %v", body.ScreenshotAuthUsername)
	}
	if body.ScreenshotAuthPassVault == nil || *body.ScreenshotAuthPassVault != "" {
		t.Fatalf("expected pointer to empty string for the vault key, got %v", body.ScreenshotAuthPassVault)
	}
}

// Setting only one half of the pair is a misconfiguration the API cannot detect
// (it stores both independently), so the create path must at least still push
// them: a site created with screenshot auth and nothing else set would otherwise
// skip the follow-up PUT entirely and silently drop both.
func TestRumSiteScreenshotAuthTriggersThePostCreatePut(t *testing.T) {
	// A zero-value model is the plan for a site that configures nothing but the
	// create-time attributes, so it must not trigger the PUT.
	var plain rumSiteModel
	if hasRumUpdateOnly(&plain) {
		t.Fatal("a plan with no update-only attributes should not trigger the follow-up PUT")
	}

	withAuth := rumSiteModel{ScreenshotAuthUsername: types.StringValue("rum-capture")}
	if !hasRumUpdateOnly(&withAuth) {
		t.Fatal("screenshot_auth_username must trigger the follow-up PUT after create")
	}

	withVaultKey := rumSiteModel{ScreenshotAuthPassVault: types.StringValue("RUM_SCREENSHOT_PASSWORD")}
	if !hasRumUpdateOnly(&withVaultKey) {
		t.Fatal("screenshot_auth_password_vault_key must trigger the follow-up PUT after create")
	}
}
