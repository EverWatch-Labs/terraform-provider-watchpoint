package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Read keeps only owned keys, an entry dropped from config is blanked, and destroy blanks the rest.
func TestStatusPageTranslation(t *testing.T) {
	f := newFakeAPI(t)
	p := tenantPath("/status", "/pages/p1/translations")
	server := []any{
		map[string]any{"entityType": "page", "entityId": "p1", "field": "name", "value": "Statut"},
		map[string]any{"entityType": "component", "entityId": "c1", "field": "name", "value": "API"},
		map[string]any{"entityType": "incident", "entityId": "i9", "field": "title", "value": "Panne"},
	}
	f.on("PUT "+p, ok(map[string]any{"language": "fr", "entries": server}))
	f.on("GET "+p, ok(map[string]any{"language": "fr", "entries": server}))

	page := translationEntry{EntityType: "page", EntityID: "p1", Field: "name", Value: "Statut"}
	comp := translationEntry{EntityType: "component", EntityID: "c1", Field: "name", Value: "API"}
	got := tfCreate(t, f, NewStatusPageTranslationResource(), statusPageTranslationModel{
		StatusPageID: types.StringValue("p1"), Language: types.StringValue("fr"), Entries: []translationEntry{page, comp},
	})
	if b := f.last("PUT " + p).Body; b["language"] != "fr" || len(b["entries"].([]any)) != 2 || got.ID.ValueString() != "p1/fr" {
		t.Fatalf("put = %v, id = %s", b, got.ID)
	}
	if after, _ := tfRead(t, f, NewStatusPageTranslationResource(), got); len(after.Entries) != 2 {
		t.Fatalf("portal-only incident entry must be ignored: %+v", after.Entries)
	}

	next := got
	next.Entries = []translationEntry{page}
	tfUpdate(t, f, NewStatusPageTranslationResource(), got, next)
	sent := f.last("PUT " + p).Body["entries"].([]any)
	if len(sent) != 2 || sent[1].(map[string]any)["entityId"] != "c1" || sent[1].(map[string]any)["value"] != "" {
		t.Fatalf("removed entry must be blanked: %v", sent)
	}

	tfDelete(t, f, NewStatusPageTranslationResource(), next)
	sent = f.last("PUT " + p).Body["entries"].([]any)
	if len(sent) != 1 || sent[0].(map[string]any)["value"] != "" {
		t.Fatalf("delete = %v", sent)
	}
}
