package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &OrganizationUserDataSource{}
var _ datasource.DataSourceWithConfigure = &OrganizationUserDataSource{}
var _ datasource.DataSourceWithConfigValidators = &OrganizationUserDataSource{}
var _ datasource.DataSource = &OrganizationUsersDataSource{}
var _ datasource.DataSourceWithConfigure = &OrganizationUsersDataSource{}

func NewOrganizationUserDataSource() datasource.DataSource {
	return &OrganizationUserDataSource{}
}

func NewOrganizationUsersDataSource() datasource.DataSource {
	return &OrganizationUsersDataSource{}
}

// OrganizationUserDataSource looks up a single member of an organization.
type OrganizationUserDataSource struct {
	client *vaultwarden.Client
}

// OrganizationUsersDataSource lists the members of an organization.
type OrganizationUsersDataSource struct {
	client *vaultwarden.Client
}

// OrganizationUserDataSourceModel describes the vaultwarden_organization_user data source.
type OrganizationUserDataSourceModel struct {
	OrganizationID types.String `tfsdk:"organization_id"`
	OrganizationUserItemModel
}

// OrganizationUsersDataSourceModel describes the vaultwarden_organization_users data source.
type OrganizationUsersDataSourceModel struct {
	OrganizationID types.String                `tfsdk:"organization_id"`
	Users          []OrganizationUserItemModel `tfsdk:"users"`
}

// OrganizationUserItemModel describes a member of an organization.
type OrganizationUserItemModel struct {
	ID               types.String `tfsdk:"id"`
	Email            types.String `tfsdk:"email"`
	UserID           types.String `tfsdk:"user_id"`
	Name             types.String `tfsdk:"name"`
	ExternalID       types.String `tfsdk:"external_id"`
	Type             types.String `tfsdk:"type"`
	Status           types.String `tfsdk:"status"`
	AccessAll        types.Bool   `tfsdk:"access_all"`
	TwoFactorEnabled types.Bool   `tfsdk:"two_factor_enabled"`
}

func newOrganizationUserItemModel(user models.OrganizationUserDetails) OrganizationUserItemModel {
	model := OrganizationUserItemModel{
		ID:               types.StringValue(user.ID),
		Email:            types.StringValue(user.Email),
		UserID:           types.StringValue(user.UserID),
		Name:             types.StringNull(),
		ExternalID:       types.StringNull(),
		Type:             types.StringValue(user.Type.String()),
		Status:           types.StringValue(user.Status.String()),
		AccessAll:        types.BoolValue(user.HasAccessAll()),
		TwoFactorEnabled: types.BoolValue(user.TwoFactorEnabled),
	}
	if user.Name != nil {
		model.Name = stringOrNull(*user.Name)
	}
	if user.ExternalID != nil {
		model.ExternalID = stringOrNull(*user.ExternalID)
	}
	return model
}

// organizationUserAttributes returns the computed attributes of an organization member.
func organizationUserAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "The ID of the organization user (the membership, not the user account)",
			Computed:            true,
		},
		"email": schema.StringAttribute{
			MarkdownDescription: "The email of the user",
			Computed:            true,
		},
		"user_id": schema.StringAttribute{
			MarkdownDescription: "The ID of the user account. Use it with the `vaultwarden_user` data source",
			Computed:            true,
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "The name of the user. Vaultwarden only returns it once the user has accepted the invitation, so it is `null` for invited users, and for users without a name",
			Computed:            true,
		},
		"external_id": schema.StringAttribute{
			MarkdownDescription: "The external ID of the organization user, `null` when not set",
			Computed:            true,
		},
		"type": schema.StringAttribute{
			MarkdownDescription: "The type of the user in the organization: `Owner`, `Admin`, `User` or `Manager`",
			Computed:            true,
		},
		"status": schema.StringAttribute{
			MarkdownDescription: "The status of the user in the organization: `Invited`, `Accepted`, `Confirmed` or `Revoked`",
			Computed:            true,
		},
		"access_all": schema.BoolAttribute{
			MarkdownDescription: "Whether the user has access to all collections of the organization. Vaultwarden derives it from the type: " +
				"`Owner` and `Admin` always have it, a `Manager` has it with the *Manage all collections* permission, and a `User` never has it",
			Computed: true,
		},
		"two_factor_enabled": schema.BoolAttribute{
			MarkdownDescription: "Whether the user has two-step login enabled",
			Computed:            true,
		},
	}
}

func (d *OrganizationUserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_user"
}

func (d *OrganizationUserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := organizationUserAttributes()
	attributes["organization_id"] = schema.StringAttribute{
		MarkdownDescription: "The ID of the organization",
		Required:            true,
	}
	attributes["id"] = schema.StringAttribute{
		MarkdownDescription: "The ID of the organization user to look up (the membership, not the user account). Exactly one of `id` and `email` must be set",
		Optional:            true,
		Computed:            true,
	}
	attributes["email"] = schema.StringAttribute{
		MarkdownDescription: "The email of the user to look up, compared case-insensitively. Exactly one of `id` and `email` must be set",
		Optional:            true,
		Computed:            true,
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up a member of an organization by ID or email. " +
			"The provider user must be an owner or admin of the organization, or a manager with access to all collections.\n\n" +
			"Requires `email` and `master_password` to be set in the provider configuration.",
		Attributes: attributes,
	}
}

func (d *OrganizationUserDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("email")),
	}
}

func (d *OrganizationUserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureLookupClient(req, resp); client != nil {
		d.client = client
	}
}

func (d *OrganizationUserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data OrganizationUserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID := data.OrganizationID.ValueString()
	users, err := d.client.GetOrganizationUsers(ctx, orgID)
	if err != nil {
		addOrganizationListError(&resp.Diagnostics, orgID, "Users", err)
		return
	}

	var key, value string
	var found *models.OrganizationUserDetails
	if !data.ID.IsNull() {
		key, value = "ID", data.ID.ValueString()
		for i := range users.Data {
			if users.Data[i].ID == value {
				found = &users.Data[i]
				break
			}
		}
	} else {
		key, value = "email", data.Email.ValueString()
		for i := range users.Data {
			if strings.EqualFold(users.Data[i].Email, value) {
				found = &users.Data[i]
				break
			}
		}
	}

	if found == nil {
		resp.Diagnostics.AddError(
			"Organization User Not Found",
			fmt.Sprintf("Organization %s has no user with %s %q.", orgID, key, value),
		)
		return
	}

	data.OrganizationUserItemModel = newOrganizationUserItemModel(*found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (d *OrganizationUsersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_users"
}

func (d *OrganizationUsersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to list the members of an organization, including invited and revoked users. " +
			"The provider user must be an owner or admin of the organization, or a manager with access to all collections.\n\n" +
			"Requires `email` and `master_password` to be set in the provider configuration.",
		Attributes: map[string]schema.Attribute{
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the organization",
				Required:            true,
			},
			"users": schema.ListNestedAttribute{
				MarkdownDescription: "The members of the organization, sorted by email",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: organizationUserAttributes(),
				},
			},
		},
	}
}

func (d *OrganizationUsersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureLookupClient(req, resp); client != nil {
		d.client = client
	}
}

func (d *OrganizationUsersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data OrganizationUsersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID := data.OrganizationID.ValueString()
	users, err := d.client.GetOrganizationUsers(ctx, orgID)
	if err != nil {
		addOrganizationListError(&resp.Diagnostics, orgID, "Users", err)
		return
	}

	sort.SliceStable(users.Data, func(i, j int) bool {
		if users.Data[i].Email != users.Data[j].Email {
			return users.Data[i].Email < users.Data[j].Email
		}
		return users.Data[i].ID < users.Data[j].ID
	})

	data.Users = make([]OrganizationUserItemModel, 0, len(users.Data))
	for _, user := range users.Data {
		data.Users = append(data.Users, newOrganizationUserItemModel(user))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
