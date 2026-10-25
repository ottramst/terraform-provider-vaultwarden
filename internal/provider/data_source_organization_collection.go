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
var _ datasource.DataSource = &OrganizationCollectionDataSource{}
var _ datasource.DataSourceWithConfigure = &OrganizationCollectionDataSource{}
var _ datasource.DataSourceWithConfigValidators = &OrganizationCollectionDataSource{}
var _ datasource.DataSource = &OrganizationCollectionsDataSource{}
var _ datasource.DataSourceWithConfigure = &OrganizationCollectionsDataSource{}

func NewOrganizationCollectionDataSource() datasource.DataSource {
	return &OrganizationCollectionDataSource{}
}

func NewOrganizationCollectionsDataSource() datasource.DataSource {
	return &OrganizationCollectionsDataSource{}
}

// OrganizationCollectionDataSource looks up a single collection of an organization.
type OrganizationCollectionDataSource struct {
	client *vaultwarden.Client
}

// OrganizationCollectionsDataSource lists the collections of an organization.
type OrganizationCollectionsDataSource struct {
	client *vaultwarden.Client
}

// OrganizationCollectionDataSourceModel describes the vaultwarden_organization_collection data source.
type OrganizationCollectionDataSourceModel struct {
	OrganizationID types.String `tfsdk:"organization_id"`
	OrganizationCollectionItemModel
}

// OrganizationCollectionsDataSourceModel describes the vaultwarden_organization_collections data source.
type OrganizationCollectionsDataSourceModel struct {
	OrganizationID types.String                      `tfsdk:"organization_id"`
	Collections    []OrganizationCollectionItemModel `tfsdk:"collections"`
}

// OrganizationCollectionItemModel describes a collection of an organization.
type OrganizationCollectionItemModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	ExternalID types.String `tfsdk:"external_id"`
}

func newOrganizationCollectionItemModel(collection models.Collection) OrganizationCollectionItemModel {
	return OrganizationCollectionItemModel{
		ID:         types.StringValue(collection.ID),
		Name:       types.StringValue(collection.Name),
		ExternalID: stringOrNull(collection.ExternalID),
	}
}

// organizationCollectionAttributes returns the computed attributes of a collection.
func organizationCollectionAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "The ID of the collection",
			Computed:            true,
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "The name of the collection",
			Computed:            true,
		},
		"external_id": schema.StringAttribute{
			MarkdownDescription: "The external ID of the collection, `null` when not set",
			Computed:            true,
		},
	}
}

func (d *OrganizationCollectionDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_collection"
}

func (d *OrganizationCollectionDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := organizationCollectionAttributes()
	attributes["organization_id"] = schema.StringAttribute{
		MarkdownDescription: "The ID of the organization",
		Required:            true,
	}
	attributes["id"] = schema.StringAttribute{
		MarkdownDescription: "The ID of the collection to look up. Exactly one of `id` and `name` must be set",
		Optional:            true,
		Computed:            true,
	}
	attributes["name"] = schema.StringAttribute{
		MarkdownDescription: "The name of the collection to look up, compared exactly. Collection names are not unique: " +
			"when several collections have the name, the lookup fails and lists their IDs. Exactly one of `id` and `name` must be set",
		Optional: true,
		Computed: true,
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up a collection of an organization by ID or name. " +
			"The provider user must be a confirmed owner or admin of the organization, or a manager with access to all collections, " +
			"because collection names are encrypted with the organization key.\n\n" +
			"Requires `email` and `master_password` to be set in the provider configuration.",
		Attributes: attributes,
	}
}

func (d *OrganizationCollectionDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *OrganizationCollectionDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureLookupClient(req, resp); client != nil {
		d.client = client
	}
}

func (d *OrganizationCollectionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data OrganizationCollectionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID := data.OrganizationID.ValueString()
	collections, err := d.client.ListOrganizationCollections(ctx, orgID)
	if err != nil {
		addOrganizationListError(&resp.Diagnostics, orgID, "Collections", err)
		return
	}

	key, value := "ID", data.ID.ValueString()
	if data.ID.IsNull() {
		key, value = "name", data.Name.ValueString()
	}

	var matches []models.Collection
	for _, collection := range collections {
		if (key == "ID" && collection.ID == value) || (key == "name" && collection.Name == value) {
			matches = append(matches, collection)
		}
	}

	switch len(matches) {
	case 0:
		resp.Diagnostics.AddError(
			"Organization Collection Not Found",
			fmt.Sprintf("Organization %s has no collection with %s %q.", orgID, key, value),
		)
		return
	case 1:
	default:
		ids := make([]string, 0, len(matches))
		for _, collection := range matches {
			ids = append(ids, collection.ID)
		}
		sort.Strings(ids)
		resp.Diagnostics.AddError(
			"Multiple Organization Collections Found",
			fmt.Sprintf("Organization %s has %d collections with name %q (IDs: %s). Look the collection up by `id` instead.",
				orgID, len(matches), value, strings.Join(ids, ", ")),
		)
		return
	}

	data.OrganizationCollectionItemModel = newOrganizationCollectionItemModel(matches[0])
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (d *OrganizationCollectionsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_collections"
}

func (d *OrganizationCollectionsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to list the collections of an organization. " +
			"The provider user must be a confirmed owner or admin of the organization, or a manager with access to all collections, " +
			"because collection names are encrypted with the organization key.\n\n" +
			"Requires `email` and `master_password` to be set in the provider configuration.",
		Attributes: map[string]schema.Attribute{
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "The ID of the organization",
				Required:            true,
			},
			"collections": schema.ListNestedAttribute{
				MarkdownDescription: "The collections of the organization, sorted by name",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: organizationCollectionAttributes(),
				},
			},
		},
	}
}

func (d *OrganizationCollectionsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureLookupClient(req, resp); client != nil {
		d.client = client
	}
}

func (d *OrganizationCollectionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data OrganizationCollectionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID := data.OrganizationID.ValueString()
	collections, err := d.client.ListOrganizationCollections(ctx, orgID)
	if err != nil {
		addOrganizationListError(&resp.Diagnostics, orgID, "Collections", err)
		return
	}

	sort.SliceStable(collections, func(i, j int) bool {
		if collections[i].Name != collections[j].Name {
			return collections[i].Name < collections[j].Name
		}
		return collections[i].ID < collections[j].ID
	})

	data.Collections = make([]OrganizationCollectionItemModel, 0, len(collections))
	for _, collection := range collections {
		data.Collections = append(data.Collections, newOrganizationCollectionItemModel(collection))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
