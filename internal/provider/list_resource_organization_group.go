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
var _ list.ListResource = &OrganizationGroupListResource{}
var _ list.ListResourceWithConfigure = &OrganizationGroupListResource{}

func NewOrganizationGroupListResource() list.ListResource {
	return &OrganizationGroupListResource{}
}

// OrganizationGroupListResource lists the groups of an organization.
type OrganizationGroupListResource struct {
	client *vaultwarden.Client
}

// OrganizationGroupListModel describes the configuration of the vaultwarden_organization_group list resource.
type OrganizationGroupListModel struct {
	OrganizationID types.String `tfsdk:"organization_id"`
	NameContains   types.String `tfsdk:"name_contains"`
}

func (r *OrganizationGroupListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_group"
}

func (r *OrganizationGroupListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the groups of an organization, sorted by name. " +
			"Groups require the `ORG_GROUPS_ENABLED=true` setting on the Vaultwarden server; without it, no groups are listed.\n\n" +
			"Requires `email` and `master_password` to be set in the provider configuration.",
		Attributes: map[string]schema.Attribute{
			"organization_id": listOrganizationIDAttribute("groups"),
			"name_contains":   listContainsAttribute("groups", "name"),
		},
	}
}

func (r *OrganizationGroupListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureListResourceClient(req, resp); client != nil {
		r.client = client
	}
}

func (r *OrganizationGroupListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config OrganizationGroupListModel
	if diags := req.Config.Get(ctx, &config); diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	orgID := config.OrganizationID.ValueString()
	all, err := r.client.GetOrganizationGroups(ctx, orgID)
	if err != nil {
		var diags diag.Diagnostics
		addOrganizationListError(&diags, orgID, "Groups", err)
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	var groups []models.Group
	for _, group := range all {
		if containsFold(group.Name, config.NameContains) {
			groups = append(groups, group)
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Name != groups[j].Name {
			return groups[i].Name < groups[j].Name
		}
		return groups[i].ID < groups[j].ID
	})

	stream.Results = listResults(groups, func(group models.Group) list.ListResult {
		var state OrganizationGroupModel
		updateOrganizationGroupModel(&state, &group)
		state.OrganizationID = config.OrganizationID

		identity := OrganizationObjectIdentityModel{OrganizationID: state.OrganizationID, ID: state.ID}
		return newListResult(ctx, req, group.Name, identity, state)
	})
}
