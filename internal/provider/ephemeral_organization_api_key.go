package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
)

var _ ephemeral.EphemeralResource = &OrganizationAPIKeyEphemeralResource{}
var _ ephemeral.EphemeralResourceWithConfigure = &OrganizationAPIKeyEphemeralResource{}

func NewOrganizationAPIKeyEphemeralResource() ephemeral.EphemeralResource {
	return &OrganizationAPIKeyEphemeralResource{}
}

// OrganizationAPIKeyEphemeralResource reads the API key of an organization without storing it.
type OrganizationAPIKeyEphemeralResource struct {
	client *vaultwarden.Client
}

// OrganizationAPIKeyEphemeralModel describes the ephemeral resource data model.
type OrganizationAPIKeyEphemeralModel struct {
	OrganizationID types.String `tfsdk:"organization_id"`
	ClientID       types.String `tfsdk:"client_id"`
	ClientSecret   types.String `tfsdk:"client_secret"`
	RevisionDate   types.String `tfsdk:"revision_date"`
}

func (r *OrganizationAPIKeyEphemeralResource) Metadata(ctx context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_api_key"
}

func (r *OrganizationAPIKeyEphemeralResource) Schema(ctx context.Context, req ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the API key of an organization, for the Bitwarden public API and Directory Connector, " +
			"without storing it in the plan or state. Requires Terraform 1.10 or later.\n\n" +
			"The organization gets an API key the first time it is read. Reading it requires the provider user's " +
			"master password and, on Vaultwarden 1.37 and later, the `Owner` role in the organization " +
			"(1.36 also allows `Admin`). To rotate the key, use `vaultwarden_organization_api_key_rotation`.",
		Attributes: map[string]schema.Attribute{
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "ID of the organization.",
				Required:            true,
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "Client ID of the API key, `organization.<organization_id>`.",
				Computed:            true,
			},
			"client_secret": schema.StringAttribute{
				MarkdownDescription: "Client secret of the API key.",
				Computed:            true,
				Sensitive:           true,
			},
			"revision_date": schema.StringAttribute{
				MarkdownDescription: "When the API key was created or last rotated.",
				Computed:            true,
			},
		},
	}
}

func (r *OrganizationAPIKeyEphemeralResource) Configure(ctx context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	r.client = configureEphemeralClient(req, resp)
}

func (r *OrganizationAPIKeyEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var data OrganizationAPIKeyEphemeralModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey, err := r.client.GetOrganizationAPIKey(ctx, data.OrganizationID.ValueString(), false)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading organization API key",
			"Could not read the API key of organization "+data.OrganizationID.ValueString()+": "+err.Error(),
		)
		return
	}

	data.ClientID = types.StringValue(apiKey.ClientID)
	data.ClientSecret = types.StringValue(apiKey.ClientSecret)
	data.RevisionDate = types.StringValue(apiKey.RevisionDate)

	resp.Diagnostics.Append(resp.Result.Set(ctx, &data)...)
}

// configureEphemeralClient returns the client passed by the provider to ephemeral resources.
func configureEphemeralClient(req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) *vaultwarden.Client {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return nil
	}

	client, ok := req.ProviderData.(*vaultwarden.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Ephemeral Resource Configure Type",
			fmt.Sprintf("Expected *vaultwarden.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return nil
	}

	return client
}
