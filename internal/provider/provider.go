package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure VaultwardenProvider satisfies various provider interfaces.
var _ provider.Provider = &VaultwardenProvider{}
var _ provider.ProviderWithFunctions = &VaultwardenProvider{}

// VaultwardenProvider defines the provider implementation.
type VaultwardenProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// VaultwardenProviderModel describes the provider data model.
type VaultwardenProviderModel struct {
	Endpoint   types.String `tfsdk:"endpoint"`
	AdminToken types.String `tfsdk:"admin_token"`
}

func (p *VaultwardenProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "vaultwarden"
	resp.Version = p.version
}

func (p *VaultwardenProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Vaultwarden provider allows you to interact with a Vaultwarden server.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "The endpoint of the Vaultwarden server",
				Optional:            true,
			},
			"admin_token": schema.StringAttribute{
				MarkdownDescription: "Token for admin page operations. This requires the `/admin` endpoint to be enabled.",
				Sensitive:           true,
				Optional:            true,
			},
		},
	}
}

func (p *VaultwardenProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// Retrieve the provider data from the configuration.
	var data VaultwardenProviderModel

	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.Endpoint.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("endpoint"),
			"Unknown Vaultwarden endpoint",
			"The provider cannot create the Vaultwarden API client as there is an unknown configuration value for the Vaultwarden endpoint. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the VAULTWARDEN_ENDPOINT environment variable.",
		)
	}

	if data.AdminToken.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("endpoint"),
			"Unknown Vaultwarden admin token",
			"The provider cannot create the Vaultwarden API client as there is an unknown configuration value for the Vaultwarden admin token. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the VAULTWARDEN_ADMIN_TOKEN environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.

	endpoint := os.Getenv("VAULTWARDEN_ENDPOINT")
	adminToken := os.Getenv("VAULTWARDEN_ADMIN_TOKEN")

	if !data.Endpoint.IsNull() {
		endpoint = data.Endpoint.ValueString()
	}
	if !data.AdminToken.IsNull() {
		adminToken = data.AdminToken.ValueString()
	}

	// If any of the expected configurations are missing, return
	// errors with provider-specific guidance.

	if endpoint == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("endpoint"),
			"Missing Vaultwarden endpoint",
			"The provider cannot create the Vaultwarden API client as there is a missing or empty value for the Vaultwarden endpoint. "+
				"Set the endpoint value in the configuration or use the VAULTWARDEN_ENDPOINT environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Create options slice for client configuration
	var opts []vaultwarden.Option

	// If admin token is provided, add it to options
	if adminToken != "" {
		opts = append(opts, vaultwarden.WithAdminToken(adminToken))
	}

	// Create a new Vaultwarden API client using the configuration values and options
	client, err := vaultwarden.New(endpoint, opts...)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Vaultwarden API client",
			"An unexpected error occurred when creating the Vaultwarden API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"Vaultwarden Client Error: "+err.Error(),
		)
	}

	// Make the Vaultwarden client available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *VaultwardenProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		UserInviteResource,
	}
}

func (p *VaultwardenProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func (p *VaultwardenProvider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &VaultwardenProvider{
			version: version,
		}
	}
}
