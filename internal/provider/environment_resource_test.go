package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A configured sort order goes through PATCH (POST has none), is_default = true
// moves the default, and destroy passes reassign_to as reassignTo.
func TestEnvironmentLifecycle(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/monitoring", "/environments")
	item := list + "/env_s"
	row := map[string]any{"id": "env_s", "key": "staging", "name": "Staging", "color": "amber", "sortOrder": 10,
		"isProduction": false, "isDefault": false, "createdAt": "2026-10-21T00:00:00Z"}
	f.on("POST "+list, func(recordedRequest) (int, any) { return http.StatusCreated, row })
	f.on("PATCH "+item, func(r recordedRequest) (int, any) { row["sortOrder"] = r.Body["sortOrder"]; return 200, row })
	f.on("PUT "+item+"/default", func(recordedRequest) (int, any) { row["isDefault"] = true; return 200, row })
	f.on("GET "+list, func(recordedRequest) (int, any) { return 200, map[string]any{"items": []any{row}} })
	f.on("DELETE "+item, func(recordedRequest) (int, any) { return http.StatusNoContent, nil })

	got := tfCreate(t, f, NewEnvironmentResource(), environmentModel{
		Key: types.StringValue("staging"), Name: types.StringValue("Staging"), Color: types.StringValue("amber"),
		IsProduction: types.BoolValue(false), SortOrder: types.Int64Value(5), IsDefault: types.BoolValue(true),
		ReassignTo: types.StringValue("env_p"),
	})
	if b := f.last("POST " + list).Body; b["key"] != "staging" || b["sortOrder"] != nil {
		t.Fatalf("post = %v", b)
	}
	if f.last("PATCH " + item).Body["sortOrder"] != float64(5) || f.count("PUT "+item+"/default") != 1 {
		t.Fatal("sort order or default not applied")
	}
	if !got.IsDefault.ValueBool() || got.SortOrder.ValueInt64() != 5 || got.ID.ValueString() != "env_s" {
		t.Fatalf("state = %+v", got)
	}

	ds := tfDataRead(t, f, NewEnvironmentDataSource(), environmentDataSourceModel{Key: types.StringValue("default")})
	if ds.ID.ValueString() != "env_s" {
		t.Fatalf("default lookup = %+v", ds)
	}

	tfDelete(t, f, NewEnvironmentResource(), got)
	if q := f.last("DELETE " + item).Query; q != "reassignTo=env_p" {
		t.Fatalf("delete query = %q", q)
	}
}
