package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// pathRoot is a small wrapper so the provider package can build attribute paths
// without every file importing the path package.
func pathRoot(name string) path.Path { return path.Root(name) }

// pathRootExpr is pathRoot for validators that take a path expression.
func pathRootExpr(name string) path.Expression { return path.MatchRoot(name) }

// diagSink wraps a *diag.Diagnostics so model helper methods can accumulate
// diagnostics with a single, concise parameter type.
type diagSink struct{ d *diag.Diagnostics }

// importByID implements the common "terraform import <addr> <id>" flow where the
// import identifier is the resource's `id` attribute.
func importByID(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// importChild implements "terraform import <addr> <parent_id>/<id>" for a
// resource addressed under a parent (a component group under a status page).
func importChild(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, parentAttr string) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID",
			fmt.Sprintf("Expected import ID in the form \"<%s>/<id>\", got: %s", parentAttr, req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(parentAttr), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// configureClient pulls the shared *client.Client out of the provider data on a
// resource's Configure call. It is a no-op (no error) when provider data is nil,
// which happens during early framework lifecycle phases.
func configureClient(req resource.ConfigureRequest, diags *diag.Diagnostics) *client.Client {
	if req.ProviderData == nil {
		return nil
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		diags.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T. This is a bug in the provider.", req.ProviderData),
		)
		return nil
	}
	return c
}

// errText is err's message, or fallback when a lookup returned no error but also
// no row (a list-and-select read that did not find the id).
func errText(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}

// ---- SLA profile attachment ----
//
// An SLA profile is NOT part of a monitor's create/update body; the platform
// exposes a dedicated endpoint (PUT .../monitors/{id}/sla-profile, null = detach)
// gated on the sla_monitoring feature flag. slaIsSet reports whether the plan
// asks for an attachment, and applySlaProfile performs the attach/detach. Call
// applySlaProfile only when the value is set (create) or changed (update) so
// tenants that never use SLA don't hit the feature-flag gate.

func slaIsSet(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown() && v.ValueString() != ""
}

func applySlaProfile(ctx context.Context, c *client.Client, monitorID string, sla types.String) error {
	var v *string
	if slaIsSet(sla) {
		s := sla.ValueString()
		v = &s
	}
	body := struct {
		SlaProfileID *string `json:"slaProfileId"`
	}{SlaProfileID: v}
	return c.Put(ctx, c.Tenantf("/monitoring", "/monitors/%s/sla-profile", monitorID), body, nil)
}

// ---- Conversion helpers: framework types -> Go pointers (for JSON bodies) ----

// strPtr returns a *string for a known, non-null value, else nil. Useful for
// PATCH bodies where omitting a field means "leave unchanged".
func strPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// boolPtr returns a *bool for a known, non-null value, else nil.
func boolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

// int64Ptr returns a *int64 for a known, non-null value, else nil.
func int64Ptr(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := v.ValueInt64()
	return &i
}

// float64Ptr returns a *float64 for a known, non-null value, else nil.
func float64Ptr(v types.Float64) *float64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	f := v.ValueFloat64()
	return &f
}

// strPtrClearing is strPtr for a plain Optional (NOT Computed) attribute on a
// PUT body: a null value becomes a pointer to "" rather than nil, so removing
// the attribute from config actually clears it server-side. strPtr's nil +
// `omitempty` means "leave unchanged", which silently ignores the removal and
// then fails the apply because the response still carries the old value.
// Not for Optional+Computed attributes - there a null config means "keep what
// the server has", which is exactly what omitting the field does.
func strPtrClearing(v types.String) *string {
	s := v.ValueString() // "" for null and unknown
	return &s
}

// ---- Conversion helpers: Go values -> framework types (for state) ----

// strNullIfEmpty maps a plain string onto types.String, treating "" as null. The
// APIs normalise an unset optional to an empty string; writing that back as a known
// empty value would diff forever against a config that simply omits the attribute.
func strNullIfEmpty(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// strFromPtr maps a *string onto types.String. nil AND "" both map to null:
// the APIs treat an unset optional and an empty string as the same thing and
// return either one, so writing "" back as a known value made every Optional
// attribute the config omits fail its apply with "was null, but now
// cty.StringVal(\"\")". Same rationale as strNullIfEmpty above.
func strFromPtr(s *string) types.String {
	if s == nil || *s == "" {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

// boolFromPtr maps a *bool onto types.Bool (nil -> null).
func boolFromPtr(b *bool) types.Bool {
	if b == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*b)
}

// int64FromPtr maps a *int64 onto types.Int64 (nil -> null).
func int64FromPtr(i *int64) types.Int64 {
	if i == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*i)
}

// float64FromPtr maps a *float64 onto types.Float64 (nil -> null).
func float64FromPtr(f *float64) types.Float64 {
	if f == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*f)
}

// setOrNull encodes a membership array onto a Set, mapping an EMPTY array to a
// null Set rather than an empty one. The APIs return `[]` for a membership the
// configuration simply omits, and writing that back as a known empty set fails
// the apply with "was null, but now cty.SetVal([])". Same rationale as
// strFromPtr mapping "" to null.
func setOrNull(ctx context.Context, values []string, diags *diagSink) types.Set {
	if len(values) == 0 {
		return types.SetNull(types.StringType)
	}
	return setFromStringSlice(ctx, values, diags.d)
}

// emptyIfNil turns a nil slice into an empty one, for a PUT body whose JSON
// array the API dereferences without a null check: sending `null` is a 500,
// sending `[]` is the clear the caller meant.
func emptyIfNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// jsonKeepingConfig maps a JSON value the API returns onto a *_json attribute.
// The API re-serialises what it stored (key order, whitespace, defaults it fills
// in), so writing its text back would diff against the operator's own spelling
// on every plan. When the API value carries everything the prior (config) value
// says, the prior string is kept: extra keys the API filled in with defaults are
// not drift. Otherwise the API's text is taken, which surfaces genuine drift. An
// absent value, `null`, `[]` and `{}` all map to null so an omitted attribute
// stays null.
func jsonKeepingConfig(prior types.String, raw []byte) types.String {
	var got any
	if len(raw) == 0 || json.Unmarshal(raw, &got) != nil || isEmptyJSON(got) {
		return types.StringNull()
	}
	if !prior.IsNull() && !prior.IsUnknown() {
		var want any
		if json.Unmarshal([]byte(prior.ValueString()), &want) == nil && jsonCovers(got, want) {
			return prior
		}
	}
	return types.StringValue(string(raw))
}

// jsonCovers reports whether got carries every value in want: objects may have
// extra keys, arrays must match element for element.
func jsonCovers(got, want any) bool {
	switch w := want.(type) {
	case map[string]any:
		g, isMap := got.(map[string]any)
		if !isMap {
			return false
		}
		for k, wv := range w {
			if !jsonCovers(g[k], wv) {
				return false
			}
		}
		return true
	case []any:
		g, isList := got.([]any)
		if !isList || len(g) != len(w) {
			return false
		}
		for i := range w {
			if !jsonCovers(g[i], w[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(got, want)
	}
}

func isEmptyJSON(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

// rawJSONOrNil returns a JSON attribute as raw bytes for a request body, or nil
// (omitted) when the attribute is null or unknown.
func rawJSONOrNil(v types.String) json.RawMessage {
	if v.IsNull() || v.IsUnknown() || strings.TrimSpace(v.ValueString()) == "" {
		return nil
	}
	return json.RawMessage(v.ValueString())
}

// ---- String slice (tags etc.) helpers ----
//
// Set-based only. The types.List equivalents were removed: every unordered
// membership attribute in this provider is a types.Set, because a List makes
// Terraform compare element order and an API that returns the members in
// another order then fails the apply with "Provider produced inconsistent
// result after apply". Leaving unused List helpers here just invited the next
// resource to reintroduce that.

// stringSliceFromSet decodes a types.Set of strings into a Go slice. Returns nil
// for null/unknown sets so the field is omitted from JSON bodies.
func stringSliceFromSet(ctx context.Context, s types.Set, diags *diag.Diagnostics) []string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(s.ElementsAs(ctx, &out, false)...)
	return out
}

// setFromStringSlice encodes a Go slice into a types.Set of strings. A nil slice
// becomes a null set to keep plans stable when the API omits the field.
func setFromStringSlice(ctx context.Context, s []string, diags *diag.Diagnostics) types.Set {
	if s == nil {
		return types.SetNull(types.StringType)
	}
	set, d := types.SetValueFrom(ctx, types.StringType, s)
	diags.Append(d...)
	return set
}

// csvPtrFromSet joins a set(string) into a comma-separated *string for the
// monitoring tag fields (stored as CSV). A null/empty set yields nil.
func csvPtrFromSet(ctx context.Context, s types.Set, diags *diag.Diagnostics) *string {
	v := stringSliceFromSet(ctx, s, diags)
	if len(v) == 0 {
		return nil
	}
	joined := strings.Join(v, ",")
	return &joined
}

// setFromCSVPtr splits a comma-separated *string (as the API returns tag fields)
// into a set(string). nil/empty yields a null set.
func setFromCSVPtr(ctx context.Context, csv *string, diags *diag.Diagnostics) types.Set {
	if csv == nil || strings.TrimSpace(*csv) == "" {
		return types.SetNull(types.StringType)
	}
	parts := strings.Split(*csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return setFromStringSlice(ctx, out, diags)
}

// csvPtrClearing is csvPtrFromSet for a PATCH/PUT body: an emptied or absent set
// becomes a pointer to "" (the API's clear signal) rather than nil, so removing
// tags from config actually clears them server-side. nil plus `omitempty` means
// "leave unchanged", which silently keeps the old tags and then fails the state
// comparison after apply. Same reasoning as strPtrClearing.
func csvPtrClearing(ctx context.Context, s types.Set, diags *diag.Diagnostics) *string {
	if csv := csvPtrFromSet(ctx, s, diags); csv != nil {
		return csv
	}
	empty := ""
	return &empty
}

// csvPtrFromInt64Set joins a set(number) into a comma-separated *string, for the
// alerting API's numeric CSV fields (days-of-week). A null/empty set yields nil.
func csvPtrFromInt64Set(ctx context.Context, s types.Set, diags *diag.Diagnostics) *string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var nums []int64
	diags.Append(s.ElementsAs(ctx, &nums, false)...)
	if len(nums) == 0 {
		return nil
	}
	parts := make([]string, 0, len(nums))
	for _, n := range nums {
		parts = append(parts, strconv.FormatInt(n, 10))
	}
	joined := strings.Join(parts, ",")
	return &joined
}

// int64SetFromCSVPtr splits a comma-separated *string back into a set(number).
// nil/empty yields a null set; an entry that is not a number is skipped rather
// than failing the read, matching the API's own tolerance for a stray value.
func int64SetFromCSVPtr(ctx context.Context, csv *string, diags *diag.Diagnostics) types.Set {
	if csv == nil || strings.TrimSpace(*csv) == "" {
		return types.SetNull(types.Int64Type)
	}
	out := make([]int64, 0)
	for _, p := range strings.Split(*csv, ",") {
		n, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	set, d := types.SetValueFrom(ctx, types.Int64Type, out)
	diags.Append(d...)
	return set
}

// alignTo reorders an API-returned list to follow the prior (plan or state)
// order, for list attributes whose API returns elements in an order of its own
// (sorted by a field, or unordered from the database). Each matcher is a pass,
// most specific first: an element is paired with the first unclaimed match of
// the earliest pass that finds one. Unmatched API elements keep their relative
// order at the end, so an import (no prior) reads back in API order.
func alignTo[P, R any](prior []P, got []R, matchers ...func(P, R) bool) []R {
	slot := make([]int, len(prior))
	for i := range slot {
		slot[i] = -1
	}
	used := make([]bool, len(got))
	for _, match := range matchers {
		for i, p := range prior {
			if slot[i] >= 0 {
				continue
			}
			for j, g := range got {
				if !used[j] && match(p, g) {
					slot[i], used[j] = j, true
					break
				}
			}
		}
	}
	out := make([]R, 0, len(got))
	for _, j := range slot {
		if j >= 0 {
			out = append(out, got[j])
		}
	}
	for j, g := range got {
		if !used[j] {
			out = append(out, g)
		}
	}
	return out
}
