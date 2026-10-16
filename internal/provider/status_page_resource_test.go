package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The status API normalises an email-domain list on write (lower-case, leading
// "@" stripped, de-duplicated, split on comma/semicolon/whitespace). The
// provider has to normalise the same way or it compares a raw config against a
// normalised response and plans a change that never settles.
func TestNormalizeEmailDomainsMatchesTheApi(t *testing.T) {
	got := normalizeEmailDomains([]string{"@Acme.com", "  partner.io ", "acme.com", "a.com;b.com c.com", ""})
	want := []string{"acme.com", "partner.io", "a.com", "b.com", "c.com"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

// allowed_email_domains is a Set, so order is irrelevant, and a config that
// differs from the stored list only by the server's own normalisation is not
// drift.
func TestSameEmailDomainSetIgnoresOrderAndNormalisation(t *testing.T) {
	if !sameEmailDomainSet([]string{"@Acme.com", "partner.io"}, []string{"partner.io", "acme.com"}) {
		t.Fatal("normalised-equal lists were reported as different")
	}
	if sameEmailDomainSet([]string{"acme.com"}, []string{"acme.com", "partner.io"}) {
		t.Fatal("a genuinely longer stored list was reported as the same")
	}
	if sameEmailDomainSet([]string{"acme.com"}, nil) {
		t.Fatal("an empty stored list was reported as the same")
	}
}

// applyResponse must not rewrite allowed_email_domains into the stored spelling
// when it names the same domains: an Optional attribute whose applied value
// differs from configuration fails the apply with "was cty.SetVal(...), but now
// cty.SetVal(...)". A list someone actually changed elsewhere IS drift and has
// to land in state so the next plan puts it back.
func TestStatusPageApplyResponseKeepsConfiguredDomainSpelling(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	ds := &diagSink{d: &diags}

	configured := setFromStringSlice(ctx, []string{"@Acme.com", "partner.io"}, &diags)
	stored := "acme.com,partner.io"
	m := statusPageModel{AllowedEmailDomains: configured}
	m.applyResponse(ctx, &statusPageResponse{ID: "sp_1", AllowedEmailDomains: &stored}, ds)

	elems := m.AllowedEmailDomains.Elements()
	if len(elems) != 2 || elems[0].(types.String).ValueString() != "@Acme.com" {
		t.Fatalf("configured spelling was rewritten to %v", elems)
	}

	changed := "other.com"
	m2 := statusPageModel{AllowedEmailDomains: configured}
	m2.applyResponse(ctx, &statusPageResponse{ID: "sp_1", AllowedEmailDomains: &changed}, ds)
	if len(m2.AllowedEmailDomains.Elements()) != 1 {
		t.Fatalf("real drift did not land in state: %v", m2.AllowedEmailDomains)
	}

	// Cleared server-side: a null Set, never an empty one, or an apply against a
	// config that omits the attribute fails.
	cleared := statusPageModel{AllowedEmailDomains: configured}
	cleared.applyResponse(ctx, &statusPageResponse{ID: "sp_1"}, ds)
	if !cleared.AllowedEmailDomains.IsNull() {
		t.Fatalf("a cleared list should be null, got %v", cleared.AllowedEmailDomains)
	}

	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
}

// access_password is write-only: the API never returns it, so Read must leave
// whatever the configuration set alone rather than blanking it.
func TestStatusPageApplyResponseLeavesAccessPasswordAlone(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	m := statusPageModel{AccessPassword: types.StringValue("hunter2")}
	m.applyResponse(ctx, &statusPageResponse{ID: "sp_1", HasAccessPassword: true}, &diagSink{d: &diags})

	if m.AccessPassword.ValueString() != "hunter2" {
		t.Fatalf("access_password was overwritten with %q", m.AccessPassword.ValueString())
	}
	if !m.HasAccessPassword.ValueBool() {
		t.Fatal("has_access_password did not track the response")
	}
}
