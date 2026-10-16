package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// registered walks what the provider actually hands Terraform, rather than a
// list someone has to remember to extend. A resource missing from provider.go
// compiles fine and simply does not exist to Terraform, so driving these tests
// off the registry is also what makes that omission visible.
func registered(t *testing.T) provider.Provider {
	t.Helper()
	return New("test")()
}

func resourceSchemas(t *testing.T) map[string]rschema.Schema {
	t.Helper()
	ctx := context.Background()
	out := map[string]rschema.Schema{}
	for _, ctor := range registered(t).Resources(ctx) {
		r := ctor()
		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "watchpoint"}, &meta)
		var resp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%s: schema diagnostics: %v", meta.TypeName, resp.Diagnostics)
		}
		out[meta.TypeName] = resp.Schema
	}
	return out
}

// The framework only validates a schema when Terraform starts the provider, so
// a mistake like a Default on a non-Computed attribute (easy to make inside the
// nested objects of a SetNestedAttribute) surfaces as a failed apply rather than
// a failed build. Run that validation here instead, over everything registered.
func TestRegisteredSchemasAreValid(t *testing.T) {
	ctx := context.Background()

	for name, s := range resourceSchemas(t) {
		if d := s.ValidateImplementation(ctx); d.HasError() {
			t.Errorf("%s: invalid schema implementation: %v", name, d)
		}
	}

	for _, ctor := range registered(t).DataSources(ctx) {
		d := ctor()
		var meta datasource.MetadataResponse
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "watchpoint"}, &meta)
		var resp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("%s: schema diagnostics: %v", meta.TypeName, resp.Diagnostics)
			continue
		}
		if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
			t.Errorf("%s: invalid schema implementation: %v", meta.TypeName, diags)
		}
	}
}

// An unordered membership attribute (region_ids, tags, monitor_ids, scopes, any
// "which things belong to this") must be a Set. As a List, Terraform compares
// element order, and an API that returns the members in a different order than
// the config listed them fails the apply with "Provider produced inconsistent
// result after apply". That has bitten this provider before, so it is a test
// rather than a convention: a flat ListAttribute is never what was meant here.
//
// ListNestedAttribute is deliberately NOT covered. Escalation steps, on-call
// participants and threshold rules are genuinely ordered, and a Set would lose
// the ordering that gives them their meaning.
func TestNoFlatListAttributes(t *testing.T) {
	for name, s := range resourceSchemas(t) {
		walkAttributes(name, s.Attributes, func(path string, a rschema.Attribute) {
			if _, bad := a.(rschema.ListAttribute); bad {
				t.Errorf("%s: %s is a ListAttribute; unordered membership must be a "+
					"SetAttribute or the apply fails with \"Provider produced "+
					"inconsistent result after apply\"", name, path)
			}
		})
	}
}

// walkAttributes visits every attribute in a resource schema, nested ones
// included, since the traps above hide just as well inside a nested object.
func walkAttributes(prefix string, attrs map[string]rschema.Attribute, visit func(string, rschema.Attribute)) {
	for key, attr := range attrs {
		path := fmt.Sprintf("%s.%s", prefix, key)
		visit(path, attr)
		switch nested := attr.(type) {
		case rschema.SetNestedAttribute:
			walkAttributes(path, nested.NestedObject.Attributes, visit)
		case rschema.ListNestedAttribute:
			walkAttributes(path, nested.NestedObject.Attributes, visit)
		case rschema.SingleNestedAttribute:
			walkAttributes(path, nested.Attributes, visit)
		}
	}
}

// The composite alert rule API is only kept alive until a release has shipped
// that warns people off it, so the warning silently going missing would strand
// the endpoint forever. Pin it on both the resource and the data source.
func TestCompositeAlertRuleIsDeprecated(t *testing.T) {
	var res resource.SchemaResponse
	NewCompositeAlertRuleResource().Schema(context.Background(), resource.SchemaRequest{}, &res)
	if res.Schema.DeprecationMessage == "" {
		t.Error("resource schema lost its DeprecationMessage")
	}

	var ds datasource.SchemaResponse
	NewCompositeAlertRuleDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &ds)
	if ds.Schema.DeprecationMessage == "" {
		t.Error("data source schema lost its DeprecationMessage")
	}
}
