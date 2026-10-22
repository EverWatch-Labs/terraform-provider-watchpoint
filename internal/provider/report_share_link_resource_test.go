package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The token and URL are kept from create; a link revoked in the portal drops out of state.
func TestReportShareLink(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/analytics", "/reports/r1/share-links")
	link := map[string]any{"id": "rsl_1", "label": "Exec", "tokenPrefix": "abcdefgh", "expiresAt": nil, "revokedAt": nil}
	f.on("POST "+list, func(recordedRequest) (int, any) {
		return http.StatusCreated, map[string]any{"link": link, "token": "tok", "url": "https://x/tok"}
	})
	f.on("GET "+list, func(recordedRequest) (int, any) { return 200, map[string]any{"items": []any{link}} })

	got := tfCreate(t, f, NewReportShareLinkResource(), reportShareLinkModel{
		ReportID: types.StringValue("r1"), Label: types.StringValue("Exec"),
	})
	if f.last("POST " + list).Body["expiresInDays"] != nil || got.Token.ValueString() != "tok" || got.URL.ValueString() != "https://x/tok" {
		t.Fatalf("state = %+v", got)
	}
	if after, removed := tfRead(t, f, NewReportShareLinkResource(), got); removed || after.Token.ValueString() != "tok" {
		t.Fatal("token must survive a read")
	}
	link["revokedAt"] = "2026-10-21T00:00:00Z"
	if _, removed := tfRead(t, f, NewReportShareLinkResource(), got); !removed {
		t.Fatal("a revoked link must leave state")
	}
}
