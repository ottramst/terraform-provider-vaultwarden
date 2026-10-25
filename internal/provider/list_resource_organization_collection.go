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
var _ list.ListResource = &OrganizationCollectionListResource{}
var _ list.ListResourceWithConfigure = &OrganizationCollectionListResource{}

func NewOrganizationCollectionListResource() list.ListResource {
	return &OrganizationCollectionListResource{}
}

// OrganizationCollectionListResource lists the collections of an organization.
type OrganizationCollectionListResource struct {
	client *vaultwarden.Client
}

// OrganizationCollectionListModel describes the configuration of the vaultwarden_organization_collection list resource.
type OrganizationCollectionListModel struct {
	OrganizationID types.String `tfsdk:"organization_id"`
	NameContains   types.String `tfsdk:"name_contains"`
}

func (r *OrganizationCollectionListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_collection"
}

func (r *OrganizationCollectionListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the collections of an organization, sorted by name. " +
			"The provider user must be a confirmed owner or admin of the organization, or a manager with access to all collections, " +
			"because collection names are encrypted with the organization key.\n\n" +
			"Requires `email` and `master_password` to be set in the provider configuration.",
		Attributes: map[string]schema.Attribute{
			"organization_id": listOrganizationIDAttribute("collections"),
			"name_contains":   listContainsAttribute("collections", "name"),
		},
	}
}

func (r *OrganizationCollectionListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if client := configureListResourceClient(req, resp); client != nil {
		r.client = client
	}
}

func (r *OrganizationCollectionListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var config OrganizationCollectionListModel
	if diags := req.Config.Get(ctx, &config); diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	orgID := config.OrganizationID.ValueString()
	all, err := r.client.ListOrganizationCollections(ctx, orgID)
	if err != nil {
		var diags diag.Diagnostics
		addOrganizationListError(&diags, orgID, "Collections", err)
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	var collections []models.Collection
	for _, collection := range all {
		if containsFold(collection.Name, config.NameContains) {
			collections = append(collections, collection)
		}
	}
	sort.SliceStable(collections, func(i, j int) bool {
		if collections[i].Name != collections[j].Name {
			return collections[i].Name < collections[j].Name
		}
		return collections[i].ID < collections[j].ID
	})

	stream.Results = listResults(collections, func(collection models.Collection) list.ListResult {
		state := OrganizationCollectionModel{
			ID:             types.StringValue(collection.ID),
			OrganizationID: config.OrganizationID,
			Name:           types.StringValue(collection.Name),
			ExternalID:     stringOrNull(collection.ExternalID),
		}

		identity := OrganizationObjectIdentityModel{OrganizationID: state.OrganizationID, ID: state.ID}
		return newListResult(ctx, req, collection.Name, identity, state)
	})
}
