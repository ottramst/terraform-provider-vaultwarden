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
var _ list.ListResource = &OrganizationUserListResource{}
var _ list.ListResourceWithConfigure = &OrganizationUserListResource{}

func NewOrganizationUserListResource() list.ListResource {
	return &OrganizationUserListResource{}
}

// OrganizationUserListResource lists the members of an organization.
type OrganizationUserListResource struct {
	client *vaultwarden.Client
}

// OrganizationUserListModel describes the configuration of the vaultwarden_organization_user list resource.
type OrganizationUserListModel struct {
	OrganizationID types.String `tfsdk:"organization_id"`
	EmailContains  types.String `tfsdk:"email_contains"`
}

func (r *OrganizationUserListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_user"
}

func (r *OrganizationUserListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the members of an organization, sorted by email, including invited and revoked ones.\n\n" +
			"Requires `email` and `master_password` to be set in the provider configuration.",
		Attributes: map[string]schema.Attribute{
			"organization_id": listOrganizationIDAttribute("members"),
			"email_contains":  listContainsAttribute("members", "email"),
		},
	}
}

func (r *OrganizationUserListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureListResourceClient(req, resp); client != nil {
		r.client = client
	}
}

func (r *OrganizationUserListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config OrganizationUserListModel
	if diags := req.Config.Get(ctx, &config); diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	orgID := config.OrganizationID.ValueString()
	resp, err := r.client.GetOrganizationUsers(ctx, orgID)
	if err != nil {
		var diags diag.Diagnostics
		addOrganizationListError(&diags, orgID, "Users", err)
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	var users []models.OrganizationUserDetails
	for _, user := range resp.Data {
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

	stream.Results = listResults(users, func(user models.OrganizationUserDetails) list.ListResult {
		state := OrganizationUserModel{
			ID:             types.StringValue(user.ID),
			OrganizationID: config.OrganizationID,
			Email:          types.StringValue(user.Email),
			AutoConfirm:    types.BoolValue(false),
		}
		setOrganizationUserState(&state, &user)

		identity := OrganizationObjectIdentityModel{OrganizationID: state.OrganizationID, ID: state.ID}
		return newListResult(ctx, req, user.Email, identity, state)
	})
}
