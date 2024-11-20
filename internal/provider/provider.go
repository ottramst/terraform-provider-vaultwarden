package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"os"
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
	Endpoint types.String `tfsdk:"endpoint"`

	// Admin Authentication
	AdminToken types.String `tfsdk:"admin_token"`

	// User Authentication
	Email          types.String `tfsdk:"email"`
	MasterPassword types.String `tfsdk:"master_password"`

	// OAuth2 Authentication
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
}

func (p *VaultwardenProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "vaultwarden"
	resp.Version = p.version
}

func (p *VaultwardenProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Vaultwarden provider allows you to interact with a Vaultwarden server.\n\n" +
			"More information about authentication methods can be found in the [provider repository](https://github.com/ottramst/terraform-provider-vaultwarden#authentication)",
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
			"email": schema.StringAttribute{
				MarkdownDescription: "Email for API operations",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.AtLeastOneOf(path.Expressions{
						path.MatchRoot("client_id"),
						path.MatchRoot("client_secret"),
					}...),
				},
			},
			"master_password": schema.StringAttribute{
				MarkdownDescription: "Master password for API operations",
				Sensitive:           true,
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.AtLeastOneOf(path.Expressions{
						path.MatchRoot("client_id"),
						path.MatchRoot("client_secret"),
					}...),
				},
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "OAuth2 client ID for API key authentication",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.Expressions{
						path.MatchRoot("client_secret"),
						path.MatchRoot("email"),
						path.MatchRoot("master_password"),
					}...),
				},
			},
			"client_secret": schema.StringAttribute{
				MarkdownDescription: "OAuth2 client secret for API key authentication",
				Sensitive:           true,
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.Expressions{
						path.MatchRoot("client_id"),
						path.MatchRoot("email"),
						path.MatchRoot("master_password"),
					}...),
				},
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

	if data.Email.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("email"),
			"Unknown Vaultwarden email",
			"The provider cannot create the Vaultwarden API client as there is an unknown configuration value for the Vaultwarden email. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the VAULTWARDEN_EMAIL environment variable.",
		)
	}

	if data.MasterPassword.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("master_password"),
			"Unknown Vaultwarden master password",
			"The provider cannot create the Vaultwarden API client as there is an unknown configuration value for the Vaultwarden master password. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the VAULTWARDEN_MASTER_PASSWORD environment variable.",
		)
	}

	if data.ClientID.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("client_id"),
			"Unknown OAuth2 client ID",
			"The provider cannot create the Vaultwarden API client as there is an unknown configuration value for the OAuth2 client ID. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the VAULTWARDEN_CLIENT_ID environment variable.",
		)
	}

	if data.ClientSecret.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("client_secret"),
			"Unknown OAuth2 client secret",
			"The provider cannot create the Vaultwarden API client as there is an unknown configuration value for the OAuth2 client secret. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the VAULTWARDEN_CLIENT_SECRET environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.
	endpoint := os.Getenv("VAULTWARDEN_ENDPOINT")
	adminToken := os.Getenv("VAULTWARDEN_ADMIN_TOKEN")
	email := os.Getenv("VAULTWARDEN_EMAIL")
	masterPassword := os.Getenv("VAULTWARDEN_MASTER_PASSWORD")
	clientID := os.Getenv("VAULTWARDEN_CLIENT_ID")
	clientSecret := os.Getenv("VAULTWARDEN_CLIENT_SECRET")

	if !data.Endpoint.IsNull() {
		endpoint = data.Endpoint.ValueString()
	}
	if !data.AdminToken.IsNull() {
		adminToken = data.AdminToken.ValueString()
	}
	if !data.Email.IsNull() {
		email = data.Email.ValueString()
	}
	if !data.MasterPassword.IsNull() {
		masterPassword = data.MasterPassword.ValueString()
	}
	if !data.ClientID.IsNull() {
		clientID = data.ClientID.ValueString()
	}
	if !data.ClientSecret.IsNull() {
		clientSecret = data.ClientSecret.ValueString()
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

	// Create options slice for client configuration
	var opts []vaultwarden.ClientOption

	// Check authentication methods
	hasUserAuth := email != "" && masterPassword != ""
	hasAPIAuth := clientID != "" && clientSecret != ""

	if !hasUserAuth && !hasAPIAuth {
		resp.Diagnostics.AddError(
			"Missing authentication credentials",
			"The provider requires either user credentials (email + master password) or API credentials (client_id + client_secret) for authentication. "+
				"Please provide one set of credentials either in the configuration or via environment variables.",
		)
	}

	// If API auth is provided, ensure user auth is also present
	if hasAPIAuth && !hasUserAuth {
		resp.Diagnostics.AddError(
			"Invalid authentication configuration",
			"When using API credentials (client_id + client_secret), user credentials (email + master password) are also required. "+
				"Please provide both sets of credentials.",
		)
	}

	// Create options for the client
	if hasAPIAuth {
		// When using API auth, we need both sets of credentials
		opts = append(opts,
			vaultwarden.WithUserCredentials(email, masterPassword),
			vaultwarden.WithOAuth2Credentials(clientID, clientSecret),
		)
	} else {
		// When using only user auth
		opts = append(opts, vaultwarden.WithUserCredentials(email, masterPassword))
	}

	// Add admin token if provided (optional)
	if adminToken != "" {
		opts = append(opts, vaultwarden.WithAdminToken(adminToken))
	}

	if resp.Diagnostics.HasError() {
		return
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
		return
	}

	// Make the Vaultwarden client available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *VaultwardenProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		UserInviteResource,
		OrganizationResource,
		OrganizationCollectionResource,
	}
}

func (p *VaultwardenProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewOrganizationDataSource,
	}
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
