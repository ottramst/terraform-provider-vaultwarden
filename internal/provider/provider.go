package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"os"
	"strconv"
	"time"
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

	MaxRetryWait types.Int64 `tfsdk:"max_retry_wait"`
}

func (p *VaultwardenProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "vaultwarden"
	resp.Version = p.version
}

func (p *VaultwardenProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Terraform provider for [Vaultwarden](https://github.com/dani-garcia/vaultwarden), " +
			"an unofficial Bitwarden-compatible server. " +
			"It supports and is tested against the latest two Vaultwarden minor releases, currently 1.36.x and 1.37.x; " +
			"older Vaultwarden versions get best-effort support only, and the provider warns when it detects one.\n\n" +
			"The provider authenticates in up to two ways, which can be combined:\n\n" +
			"- **As a user**, with `email` and `master_password`. Required for resources that act on behalf of a user, " +
			"such as organizations and collections, because their data is encrypted client-side with the user's keys. " +
			"If the user has two-step login enabled, also set the user's personal API key (`client_id` and `client_secret`).\n" +
			"- **As the server admin**, with `admin_token`. Required for resources that use the `/admin` API, " +
			"such as `vaultwarden_user` and `vaultwarden_account_register`. The admin page must be enabled on the server.\n\n" +
			"Every attribute can also be set with an environment variable.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "The URL of the Vaultwarden server. Can also be set with the `VAULTWARDEN_ENDPOINT` environment variable.",
				Required:            true,
			},
			"admin_token": schema.StringAttribute{
				MarkdownDescription: "The admin token, for resources that use the `/admin` API. This requires the admin page to be enabled. Can also be set with the `VAULTWARDEN_ADMIN_TOKEN` environment variable.",
				Sensitive:           true,
				Optional:            true,
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "The email of the user to authenticate as. Can also be set with the `VAULTWARDEN_EMAIL` environment variable.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.AtLeastOneOf(path.Expressions{
						path.MatchRoot("admin_token"),
						path.MatchRoot("client_id"),
						path.MatchRoot("client_secret"),
					}...),
				},
			},
			"master_password": schema.StringAttribute{
				MarkdownDescription: "The master password of the user, used to log in and to decrypt the user's keys. Can also be set with the `VAULTWARDEN_MASTER_PASSWORD` environment variable.",
				Sensitive:           true,
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.AtLeastOneOf(path.Expressions{
						path.MatchRoot("admin_token"),
						path.MatchRoot("client_id"),
						path.MatchRoot("client_secret"),
					}...),
				},
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "The `client_id` of the user's personal API key, to log in with the API key instead of the master password, for example when two-step login is enabled. Requires `email` and `master_password`. Can also be set with the `VAULTWARDEN_CLIENT_ID` environment variable.",
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
				MarkdownDescription: "The `client_secret` of the user's personal API key. Can also be set with the `VAULTWARDEN_CLIENT_SECRET` environment variable.",
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
			"max_retry_wait": schema.Int64Attribute{
				MarkdownDescription: "The total number of seconds a request waits for retries when Vaultwarden rate limits it (HTTP 429), " +
					"for example when many provider instances log in from the same IP address. Retries back off exponentially. " +
					"Set to `0` to disable retries. Defaults to `60`. Can also be set with the `VAULTWARDEN_MAX_RETRY_WAIT` environment variable.",
				Optional: true,
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
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
	maxRetryWait := int64(vaultwarden.DefaultMaxRetryWait / time.Second)
	if value := os.Getenv("VAULTWARDEN_MAX_RETRY_WAIT"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			resp.Diagnostics.AddAttributeError(
				path.Root("max_retry_wait"),
				"Invalid VAULTWARDEN_MAX_RETRY_WAIT",
				"The VAULTWARDEN_MAX_RETRY_WAIT environment variable must be a whole number of seconds, 0 or more. Got: "+value,
			)
		}
		maxRetryWait = parsed
	}

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
	if !data.MaxRetryWait.IsNull() && !data.MaxRetryWait.IsUnknown() {
		maxRetryWait = data.MaxRetryWait.ValueInt64()
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
	opts := []vaultwarden.ClientOption{
		vaultwarden.WithMaxRetryWait(time.Duration(maxRetryWait) * time.Second),
	}

	// Check authentication methods
	hasAdminAuth := adminToken != ""
	hasUserAuth := email != "" && masterPassword != ""
	hasAPIAuth := clientID != "" && clientSecret != ""

	if !hasUserAuth && !hasAPIAuth && !hasAdminAuth {
		resp.Diagnostics.AddError(
			"Missing authentication credentials",
			"The provider requires at least one authentication method to be provided. "+
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

	// Warn when the server is older than the supported versions. A failed probe is not
	// fatal: the provider has no version-dependent behavior.
	if serverVersion, err := client.GetServerVersion(ctx); err != nil {
		tflog.Warn(ctx, "could not detect the Vaultwarden server version", map[string]interface{}{"error": err.Error()})
	} else if supported, err := vaultwarden.IsSupportedVersion(serverVersion); err != nil {
		tflog.Warn(ctx, "could not parse the Vaultwarden server version", map[string]interface{}{"version": serverVersion})
	} else if !supported {
		resp.Diagnostics.AddWarning(
			"Unsupported Vaultwarden version",
			fmt.Sprintf("The Vaultwarden server at %s runs version %s. The provider supports Vaultwarden %s and newer; "+
				"older versions get best-effort support only. Upgrading Vaultwarden is recommended.",
				endpoint, serverVersion, vaultwarden.MinimumSupportedVersion),
		)
	} else {
		tflog.Info(ctx, "detected Vaultwarden server version", map[string]interface{}{"version": serverVersion})
	}

	// Make the Vaultwarden client available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *VaultwardenProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		AccountRegisterResource,
		OrganizationCollectionResource,
		OrganizationCollectionUserResource,
		OrganizationGroupResource,
		OrganizationPolicyResource,
		OrganizationResource,
		OrganizationUserResource,
		UserResource,
	}
}

func (p *VaultwardenProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewOrganizationDataSource,
		NewOrganizationGroupDataSource,
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
