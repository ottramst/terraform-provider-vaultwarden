package provider

import (
	"context"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ list.ListResource = &UserListResource{}
var _ list.ListResourceWithConfigure = &UserListResource{}

func NewUserListResource() list.ListResource {
	return &UserListResource{}
}

// UserListResource lists the users of the server with the admin API.
type UserListResource struct {
	client *vaultwarden.Client
}

// UserListModel describes the configuration of the vaultwarden_user list resource.
type UserListModel struct {
	EmailContains types.String `tfsdk:"email_contains"`
}

func (r *UserListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all users of the server, sorted by email, including invited and disabled ones.\n\n" +
			"Requires `admin_token` to be set in the provider configuration.",
		Attributes: map[string]schema.Attribute{
			"email_contains": listContainsAttribute("users", "email"),
		},
	}
}

func (r *UserListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureListResourceClient(req, resp); client != nil {
		r.client = client
	}
}

func (r *UserListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config UserListModel
	if diags := req.Config.Get(ctx, &config); diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	all, err := r.client.ListAdminUsers(ctx)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Error Reading Vaultwarden Users", "Could not list the users of the server: "+err.Error())
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	var users []models.AdminUser
	for _, user := range all {
		if containsFold(user.Email, config.EmailContains) {
			users = append(users, user)
		}
	}
	sort.SliceStable(users, func(i, j int) bool {
		if users[i].Email != users[j].Email {
			return users[i].Email < users[j].Email
		}
		return users[i].ID < users[j].ID
	})

	stream.Results = listResults(users, func(user models.AdminUser) list.ListResult {
		state := UserModel{
			ID:      types.StringValue(user.ID),
			Email:   types.StringValue(user.Email),
			Enabled: types.BoolValue(user.Enabled),
		}

		return newListResult(ctx, req, user.Email, UserIdentityModel{ID: state.ID}, state)
	})
}
