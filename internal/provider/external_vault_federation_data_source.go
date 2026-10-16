package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The identity Watchpoint presents when it authenticates to a vault you run:
// issuer, discovery URL, JWKS URL, the subject for this tenant and the audience for
// the vendor you name.
//
// This exists so the trust policy on the vendor side can be built from the SAME values
// Watchpoint will actually present, in the same apply. Copying a subject by hand into
// an IAM trust policy or a Vault role is the single most common reason a federated
// setup comes back "not authorized", and a typo there is not visible from either side.
//
//	GET /monitoring/tenants/{tenant}/vault/provider-kinds -> []{ kind, federation{...} }
var (
	_ datasource.DataSource              = (*externalVaultFederationDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*externalVaultFederationDataSource)(nil)
)

// NewExternalVaultFederationDataSource is the data source constructor wired into the provider.
func NewExternalVaultFederationDataSource() datasource.DataSource {
	return &externalVaultFederationDataSource{}
}

type externalVaultFederationDataSource struct {
	client *client.Client
}

type externalVaultFederationModel struct {
	Kind types.String `tfsdk:"kind"`

	Issuer            types.String `tfsdk:"issuer"`
	DiscoveryURL      types.String `tfsdk:"discovery_url"`
	JwksURL           types.String `tfsdk:"jwks_url"`
	Subject           types.String `tfsdk:"subject"`
	Audience          types.String `tfsdk:"audience"`
	AudienceFromField types.String `tfsdk:"audience_from_field"`
	SigningAlgorithm  types.String `tfsdk:"signing_algorithm"`
	Reachable         types.Bool   `tfsdk:"reachable"`
	Supported         types.Bool   `tfsdk:"supported"`
}

type providerKindFederation struct {
	Applies           bool   `json:"applies"`
	Issuer            string `json:"issuer"`
	DiscoveryURL      string `json:"discoveryUrl"`
	JwksURL           string `json:"jwksUrl"`
	Subject           string `json:"subject"`
	Audience          string `json:"audience"`
	AudienceFromField string `json:"audienceFromField"`
	SigningAlgorithm  string `json:"signingAlgorithm"`
	Reachable         bool   `json:"reachable"`
	ReachableHint     string `json:"reachableHint"`
}

type providerKindResponse struct {
	Kind       string                 `json:"kind"`
	Label      string                 `json:"label"`
	Federation providerKindFederation `json:"federation"`
}

func (d *externalVaultFederationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_external_vault_federation"
}

func (d *externalVaultFederationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *externalVaultFederationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The federated identity Watchpoint presents to a vault vendor, so the trust policy on " +
			"the vendor side can be built from the same values Watchpoint will actually send.\n\n" +
			"```terraform\n" +
			"data \"watchpoint_external_vault_federation\" \"aws\" {\n" +
			"  kind = \"aws-secrets-manager\"\n" +
			"}\n\n" +
			"resource \"aws_iam_openid_connect_provider\" \"watchpoint\" {\n" +
			"  url             = data.watchpoint_external_vault_federation.aws.issuer\n" +
			"  client_id_list  = [data.watchpoint_external_vault_federation.aws.audience]\n" +
			"}\n" +
			"```\n\n" +
			"Pin `subject` in the trust policy. Without it, any Watchpoint tenant could authenticate against it.",
		Attributes: map[string]schema.Attribute{
			"kind": schema.StringAttribute{
				MarkdownDescription: "Vendor to report the audience for, for example `aws-secrets-manager`. " +
					"`issuer`, `discovery_url`, `jwks_url` and `subject` are the same for every vendor; only the " +
					"audience differs.",
				Required: true,
			},
			"issuer": schema.StringAttribute{
				MarkdownDescription: "The `iss` of every assertion Watchpoint mints, and the OIDC issuer to " +
					"register with the vendor.",
				Computed: true,
			},
			"discovery_url": schema.StringAttribute{
				MarkdownDescription: "The OIDC discovery document. Your cloud fetches this itself, so it has to be " +
					"reachable from the vendor's network, not just from you.",
				Computed: true,
			},
			"jwks_url": schema.StringAttribute{
				MarkdownDescription: "The signing keys. Vendors that do not do full OIDC discovery (HashiCorp " +
					"Vault's JWKS mode, Conjur, Akeyless) want this directly.",
				Computed: true,
			},
			"subject": schema.StringAttribute{
				MarkdownDescription: "The `sub` Watchpoint asserts for THIS tenant. Pin the trust policy to it " +
					"exactly; a wildcard would let any Watchpoint tenant authenticate against your vault.",
				Computed: true,
			},
			"audience": schema.StringAttribute{
				MarkdownDescription: "The audience for this vendor. Empty when the vendor's audience is a value " +
					"you create rather than one Watchpoint hands over (Google's workload identity pool path, for " +
					"example); `audience_from_field` then names the `config` key to put it in.",
				Computed: true,
			},
			"audience_from_field": schema.StringAttribute{
				MarkdownDescription: "When `audience` is empty, the `watchpoint_external_vault.config` key that " +
					"the audience you create belongs in. Empty otherwise.",
				Computed: true,
			},
			"signing_algorithm": schema.StringAttribute{
				MarkdownDescription: "Signature algorithm of the assertion, for vendors that ask.",
				Computed:            true,
			},
			"reachable": schema.BoolAttribute{
				MarkdownDescription: "False when the issuer is not a public HTTPS address, which means the " +
					"vendor cannot fetch the discovery document and federation cannot work yet. Worth asserting " +
					"on with a `precondition` rather than discovering it from the vendor's error message.",
				Computed: true,
			},
			"supported": schema.BoolAttribute{
				MarkdownDescription: "False for vendors with no federation option at all (Delinea Secret Server, " +
					"BeyondTrust Password Safe, the CyberArk Central Credential Provider). Those need " +
					"`token_source = \"client_credentials\"`, `\"mtls\"` or `\"local\"` instead.",
				Computed: true,
			},
		},
	}
}

func (d *externalVaultFederationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config externalVaultFederationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var kinds []providerKindResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/monitoring", "/vault/provider-kinds"), nil, &kinds); err != nil {
		resp.Diagnostics.AddError("Error reading external vault provider kinds", err.Error())
		return
	}

	wanted := config.Kind.ValueString()
	for i := range kinds {
		k := &kinds[i]
		if !strings.EqualFold(k.Kind, wanted) {
			continue
		}

		f := k.Federation
		config.Issuer = types.StringValue(f.Issuer)
		config.DiscoveryURL = types.StringValue(f.DiscoveryURL)
		config.JwksURL = types.StringValue(f.JwksURL)
		config.Subject = types.StringValue(f.Subject)
		config.Audience = types.StringValue(f.Audience)
		config.AudienceFromField = types.StringValue(f.AudienceFromField)
		config.SigningAlgorithm = types.StringValue(f.SigningAlgorithm)
		config.Reachable = types.BoolValue(f.Reachable)
		config.Supported = types.BoolValue(f.Applies)

		// A warning rather than an error: the practitioner may be laying the ground work
		// before the issuer is public, and failing the plan outright would block that.
		if f.Applies && !f.Reachable && f.ReachableHint != "" {
			resp.Diagnostics.AddWarning("Federation is not usable yet", f.ReachableHint)
		}

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	// Name what does exist. A bare "not found" for a vendor slug sends somebody hunting
	// through documentation for a spelling they cannot check.
	available := make([]string, 0, len(kinds))
	for i := range kinds {
		available = append(available, kinds[i].Kind)
	}
	resp.Diagnostics.AddError(
		"Unknown external vault kind",
		fmt.Sprintf("No vault provider named %q. Available: %s.", wanted, strings.Join(available, ", ")),
	)
}
