package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &UserDataSource{}
var _ datasource.DataSourceWithConfigure = &UserDataSource{}
var _ datasource.DataSourceWithConfigValidators = &UserDataSource{}
var _ datasource.DataSource = &UsersDataSource{}
var _ datasource.DataSourceWithConfigure = &UsersDataSource{}

func NewUserDataSource() datasource.DataSource {
	return &UserDataSource{}
}

func NewUsersDataSource() datasource.DataSource {
	return &UsersDataSource{}
}

// UserDataSource looks up a user of the server with the admin API.
type UserDataSource struct {
	client *vaultwarden.Client
}

// UsersDataSource lists the users of the server with the admin API.
type UsersDataSource struct {
	client *vaultwarden.Client
}

// UserDataSourceModel describes the vaultwarden_user data source.
type UserDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Email            types.String `tfsdk:"email"`
	Name             types.String `tfsdk:"name"`
	Enabled          types.Bool   `tfsdk:"enabled"`
	Invited          types.Bool   `tfsdk:"invited"`
	EmailVerified    types.Bool   `tfsdk:"email_verified"`
	TwoFactorEnabled types.Bool   `tfsdk:"two_factor_enabled"`
	CreatedAt        types.String `tfsdk:"created_at"`
	Organizations    types.List   `tfsdk:"organizations"`
}

// UsersDataSourceModel describes the vaultwarden_users data source.
type UsersDataSourceModel struct {
	Users []UsersItemModel `tfsdk:"users"`
}

// UsersItemModel describes a user in the vaultwarden_users data source.
type UsersItemModel struct {
	UserDataSourceModel
	LastActive types.String `tfsdk:"last_active"`
}

// userOrganizationAttrTypes are the attribute types of an organization membership of a user.
var userOrganizationAttrTypes = map[string]attr.Type{
	"id":                   types.StringType,
	"name":                 types.StringType,
	"organization_user_id": types.StringType,
	"type":                 types.StringType,
}

func newUserDataSourceModel(user models.AdminUser) (UserDataSourceModel, error) {
	memberships := append([]models.OrganizationMembership(nil), user.Organizations...)
	sort.SliceStable(memberships, func(i, j int) bool {
		if memberships[i].Name != memberships[j].Name {
			return memberships[i].Name < memberships[j].Name
		}
		return memberships[i].ID < memberships[j].ID
	})

	organizations := make([]attr.Value, 0, len(memberships))
	for _, membership := range memberships {
		organization, diags := types.ObjectValue(userOrganizationAttrTypes, map[string]attr.Value{
			"id":                   types.StringValue(membership.ID),
			"name":                 types.StringValue(membership.Name),
			"organization_user_id": types.StringValue(membership.OrganizationUserID),
			"type":                 types.StringValue(membership.Type.String()),
		})
		if diags.HasError() {
			return UserDataSourceModel{}, fmt.Errorf("failed to build organization %s of user %s", membership.ID, user.ID)
		}
		organizations = append(organizations, organization)
	}
	organizationList, diags := types.ListValue(types.ObjectType{AttrTypes: userOrganizationAttrTypes}, organizations)
	if diags.HasError() {
		return UserDataSourceModel{}, fmt.Errorf("failed to build the organizations of user %s", user.ID)
	}

	model := UserDataSourceModel{
		ID:               types.StringValue(user.ID),
		Email:            types.StringValue(user.Email),
		Name:             types.StringNull(),
		Enabled:          types.BoolValue(user.Enabled),
		Invited:          types.BoolValue(user.Status == models.AdminUserStatusInvited),
		EmailVerified:    types.BoolValue(user.EmailVerified),
		TwoFactorEnabled: types.BoolValue(user.TwoFactorEnabled),
		CreatedAt:        types.StringValue(user.CreationDate),
		Organizations:    organizationList,
	}
	if user.Name != "" {
		model.Name = types.StringValue(user.Name)
	}

	return model, nil
}

// userAttributes returns the computed attributes of a user.
func userAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "The ID of the user",
			Computed:            true,
		},
		"email": schema.StringAttribute{
			MarkdownDescription: "The email of the user",
			Computed:            true,
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "The name of the user, `null` when empty. Vaultwarden sets the name of an invited user to their email",
			Computed:            true,
		},
		"enabled": schema.BoolAttribute{
			MarkdownDescription: "Whether the user is enabled. A disabled user cannot log in",
			Computed:            true,
		},
		"invited": schema.BoolAttribute{
			MarkdownDescription: "Whether the user was invited and has not created an account (set a master password) yet",
			Computed:            true,
		},
		"email_verified": schema.BoolAttribute{
			MarkdownDescription: "Whether the user has verified their email. Always `true` when the server has no SMTP configured",
			Computed:            true,
		},
		"two_factor_enabled": schema.BoolAttribute{
			MarkdownDescription: "Whether the user has two-step login enabled",
			Computed:            true,
		},
		"created_at": schema.StringAttribute{
			MarkdownDescription: "When the user was created, as an RFC 3339 timestamp in UTC",
			Computed:            true,
		},
		"organizations": schema.ListNestedAttribute{
			MarkdownDescription: "The organizations the user is a confirmed member of, sorted by name. " +
				"Organizations the user is only invited to, or has accepted but is not yet confirmed in, are not listed",
			Computed: true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						MarkdownDescription: "The ID of the organization",
						Computed:            true,
					},
					"name": schema.StringAttribute{
						MarkdownDescription: "The name of the organization",
						Computed:            true,
					},
					"organization_user_id": schema.StringAttribute{
						MarkdownDescription: "The ID of the user's membership in the organization",
						Computed:            true,
					},
					"type": schema.StringAttribute{
						MarkdownDescription: "The type of the user in the organization: `Owner`, `Admin`, `User` or `Manager`",
						Computed:            true,
					},
				},
			},
		},
	}
}

func configureUserDataSourceClient(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *vaultwarden.Client {
	if req.ProviderData == nil {
		return nil
	}

	client, ok := req.ProviderData.(*vaultwarden.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *vaultwarden.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return nil
	}

	return client
}

func (d *UserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *UserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := userAttributes()
	attributes["id"] = schema.StringAttribute{
		MarkdownDescription: "The ID of the user to look up. Exactly one of `id` and `email` must be set",
		Optional:            true,
		Computed:            true,
	}
	attributes["email"] = schema.StringAttribute{
		MarkdownDescription: "The email of the user to look up, compared case-insensitively. Exactly one of `id` and `email` must be set",
		Optional:            true,
		Computed:            true,
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up a user of the Vaultwarden server by ID or email, including invited users. " +
			"The time the user was last active is only available from the `vaultwarden_users` data source.\n\n" +
			"Requires `admin_token` to be set in the provider configuration.",
		Attributes: attributes,
	}
}

func (d *UserDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("email")),
	}
}

func (d *UserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureUserDataSourceClient(req, resp); client != nil {
		d.client = client
	}
}

func (d *UserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data UserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var user *models.AdminUser
	var err error
	key, value := "ID", data.ID.ValueString()
	if data.ID.IsNull() {
		key, value = "email", data.Email.ValueString()
		user, err = d.client.GetAdminUserByEmail(ctx, value)
	} else {
		user, err = d.client.GetAdminUser(ctx, value)
	}
	if err != nil {
		if vaultwarden.IsNotFound(err) {
			resp.Diagnostics.AddError("User Not Found", fmt.Sprintf("The Vaultwarden server has no user with %s %q.", key, value))
			return
		}
		resp.Diagnostics.AddError("Error Reading User", fmt.Sprintf("Could not read the user with %s %q: %s", key, value, err))
		return
	}

	model, err := newUserDataSourceModel(*user)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading User", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (d *UsersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_users"
}

func (d *UsersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := userAttributes()
	attributes["last_active"] = schema.StringAttribute{
		MarkdownDescription: "When the user was last active, as Vaultwarden formats it for display: `YYYY-MM-DD hh:mm:ss` and a time zone " +
			"abbreviation, in the server's time zone (the `TZ` environment variable). `null` when the user has never logged in",
		Computed: true,
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to list all users of the Vaultwarden server, including invited users.\n\n" +
			"Requires `admin_token` to be set in the provider configuration.",
		Attributes: map[string]schema.Attribute{
			"users": schema.ListNestedAttribute{
				MarkdownDescription: "The users of the server, sorted by email",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: attributes,
				},
			},
		},
	}
}

func (d *UsersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureUserDataSourceClient(req, resp); client != nil {
		d.client = client
	}
}

func (d *UsersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	users, err := d.client.ListAdminUsers(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Users", fmt.Sprintf("Could not list the users: %s", err))
		return
	}

	sort.SliceStable(users, func(i, j int) bool {
		if users[i].Email != users[j].Email {
			return users[i].Email < users[j].Email
		}
		return users[i].ID < users[j].ID
	})

	data := UsersDataSourceModel{Users: make([]UsersItemModel, 0, len(users))}
	for _, user := range users {
		model, err := newUserDataSourceModel(user)
		if err != nil {
			resp.Diagnostics.AddError("Error Reading Users", err.Error())
			return
		}
		item := UsersItemModel{UserDataSourceModel: model, LastActive: types.StringNull()}
		if user.LastActive != nil {
			item.LastActive = types.StringValue(*user.LastActive)
		}
		data.Users = append(data.Users, item)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
