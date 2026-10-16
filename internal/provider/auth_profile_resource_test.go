package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestAuthProfileToUpsertBodyClientCredentials verifies the flat OAuth2
// client-credentials fields serialize onto the wire DTO and that fields not set
// stay nil (dropped from JSON via omitempty).
func TestAuthProfileToUpsertBodyClientCredentials(t *testing.T) {
	m := authProfileModel{
		Name:             types.StringValue("PaymentsApi"),
		Kind:             types.StringValue("OAuth2ClientCredentials"),
		TokenEndpoint:    types.StringValue("https://auth.example.com/oauth/token"),
		ClientID:         types.StringValue("svc-123"),
		ClientSecret:     types.StringValue("s3cr3t"),
		Scopes:           types.StringValue("read write"),
		ClientAuthMethod: types.StringValue("body"),
		// Audience left null (not supplied).
	}

	body := m.toUpsertBody()
	if body.Name == nil || *body.Name != "PaymentsApi" {
		t.Fatalf("body.Name = %v, want PaymentsApi", body.Name)
	}
	if body.Kind == nil || *body.Kind != "OAuth2ClientCredentials" {
		t.Fatalf("body.Kind = %v, want OAuth2ClientCredentials", body.Kind)
	}
	if body.TokenEndpoint == nil || *body.TokenEndpoint != "https://auth.example.com/oauth/token" {
		t.Fatalf("body.TokenEndpoint = %v, want token endpoint", body.TokenEndpoint)
	}
	if body.ClientSecret == nil || *body.ClientSecret != "s3cr3t" {
		t.Fatalf("clientSecret = %v, want s3cr3t", body.ClientSecret)
	}
	if body.Audience != nil {
		t.Fatalf("audience = %v, want nil (omitted)", body.Audience)
	}
	// Fields for other kinds must be omitted.
	if body.Password != nil || body.RefreshToken != nil || body.StaticKey != nil ||
		body.ClientCertPEM != nil || body.ClientKeyPEM != nil || body.LoginFlowJSON != nil {
		t.Fatalf("unexpected non-credential field set on body: %+v", body)
	}
}

// TestAuthProfileToUpsertBodyOmitsUnsuppliedSecret verifies that a null secret
// yields a nil pointer (field dropped from JSON), so the backend keeps the stored
// secret intact, mirroring the masked-update "omit to retain" semantics.
func TestAuthProfileToUpsertBodyOmitsUnsuppliedSecret(t *testing.T) {
	m := authProfileModel{
		Name:          types.StringValue("PaymentsApi"),
		Kind:          types.StringValue("OAuth2ClientCredentials"),
		TokenEndpoint: types.StringValue("https://auth.example.com/oauth/token"),
		ClientID:      types.StringValue("svc-123"),
		// ClientSecret left null (not re-supplied).
		Scopes: types.StringValue("read"),
	}

	body := m.toUpsertBody()
	if body.ClientSecret != nil {
		t.Fatalf("clientSecret = %v, want nil when not re-supplied", body.ClientSecret)
	}
	if body.ClientID == nil || *body.ClientID != "svc-123" {
		t.Fatalf("clientId = %v, want svc-123", body.ClientID)
	}
}

// TestAuthProfileApplyResponsePreservesSecrets verifies the masked response maps
// non-secret fields onto state while leaving the write-only secret untouched, and
// that the has* booleans are surfaced.
func TestAuthProfileApplyResponsePreservesSecrets(t *testing.T) {
	tokenEndpoint := "https://auth.example.com/oauth/token"
	clientID := "svc-123"
	scopes := "read write"
	desc := "Payments upstream"
	updated := "2026-02-02T00:00:00Z"

	m := authProfileModel{
		TokenEndpoint: types.StringValue(tokenEndpoint),
		ClientID:      types.StringValue(clientID),
		ClientSecret:  types.StringValue("s3cr3t"),
		Scopes:        types.StringValue("read"),
	}

	resp := &authProfileResponse{
		ID:              "auth_1",
		Name:            "PaymentsApi",
		Kind:            "OAuth2ClientCredentials",
		Description:     &desc,
		TokenEndpoint:   &tokenEndpoint,
		ClientID:        &clientID,
		Scopes:          &scopes,
		HasClientSecret: true,
		CreatedAt:       "2026-02-01T00:00:00Z",
		UpdatedAt:       &updated,
	}

	m.applyResponse(resp)

	if m.ID.ValueString() != "auth_1" {
		t.Fatalf("ID = %q, want auth_1", m.ID.ValueString())
	}
	if m.Description.ValueString() != "Payments upstream" {
		t.Fatalf("Description = %q, want Payments upstream", m.Description.ValueString())
	}
	// Non-secret echo refreshes in place.
	if m.Scopes.ValueString() != "read write" {
		t.Fatalf("Scopes = %q, want 'read write'", m.Scopes.ValueString())
	}
	// Write-only secret must survive the read unchanged.
	if m.ClientSecret.ValueString() != "s3cr3t" {
		t.Fatalf("ClientSecret = %q, want s3cr3t (write-only, must be preserved)", m.ClientSecret.ValueString())
	}
	// Masked indicator surfaced.
	if !m.HasClientSecret.ValueBool() {
		t.Fatalf("HasClientSecret = false, want true")
	}
}

// TestAuthProfileApplyStrIfPresent verifies a null API value does not clobber an
// existing configured value into null (which would churn the plan).
func TestAuthProfileApplyStrIfPresent(t *testing.T) {
	v := types.StringValue("keep-me")
	applyStrIfPresent(&v, nil)
	if v.ValueString() != "keep-me" {
		t.Fatalf("value clobbered by nil src: %q", v.ValueString())
	}
	repl := "new"
	applyStrIfPresent(&v, &repl)
	if v.ValueString() != "new" {
		t.Fatalf("value not refreshed: %q", v.ValueString())
	}
}
