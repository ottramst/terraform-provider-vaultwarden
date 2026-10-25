package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
)

var _ ephemeral.EphemeralResource = &UserAPIKeyEphemeralResource{}
var _ ephemeral.EphemeralResourceWithConfigure = &UserAPIKeyEphemeralResource{}

func NewUserAPIKeyEphemeralResource() ephemeral.EphemeralResource {
	return &UserAPIKeyEphemeralResource{}
}

// UserAPIKeyEphemeralResource reads the personal API key of the provider user without storing it.
type UserAPIKeyEphemeralResource struct {
	client *vaultwarden.Client
}

// UserAPIKeyEphemeralModel describes the ephemeral resource data model.
type UserAPIKeyEphemeralModel struct {
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
}

func (r *UserAPIKeyEphemeralResource) Metadata(ctx context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_api_key"
}

func (r *UserAPIKeyEphemeralResource) Schema(ctx context.Context, req ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the personal API key of the provider user, for the Bitwarden CLI or the provider's " +
			"`client_id` and `client_secret`, without storing it in the plan or state. Requires Terraform 1.10 or later.\n\n" +
			"The user gets an API key the first time it is read. Reading it requires the provider user's master password. " +
			"Only the provider user's own key can be read.",
		Attributes: map[string]schema.Attribute{
			"client_id": schema.StringAttribute{
				MarkdownDescription: "Client ID of the API key, `user.<user_id>`.",
				Computed:            true,
			},
			"client_secret": schema.StringAttribute{
				MarkdownDescription: "Client secret of the API key.",
				Computed:            true,
				Sensitive:           true,
			},
		},
	}
}

func (r *UserAPIKeyEphemeralResource) Configure(ctx context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	r.client = configureEphemeralClient(req, resp)
}

func (r *UserAPIKeyEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	apiKey, err := r.client.GetUserAPIKey(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading user API key",
			"Could not read the API key of the provider user: "+err.Error(),
		)
		return
	}

	data := UserAPIKeyEphemeralModel{
		ClientID:     types.StringValue(apiKey.ClientID),
		ClientSecret: types.StringValue(apiKey.ClientSecret),
	}

	resp.Diagnostics.Append(resp.Result.Set(ctx, &data)...)
}
