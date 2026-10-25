package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &OrganizationGroupDataSource{}
var _ datasource.DataSourceWithConfigure = &OrganizationGroupDataSource{}
var _ datasource.DataSourceWithConfigValidators = &OrganizationGroupDataSource{}

func NewOrganizationGroupDataSource() datasource.DataSource {
	return &OrganizationGroupDataSource{}
}

// OrganizationGroupDataSource defines the data source implementation.
type OrganizationGroupDataSource struct {
	client *vaultwarden.Client
}

// OrganizationGroupDataSourceModel describes the data source data model.
type OrganizationGroupDataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Name           types.String `tfsdk:"name"`
	ExternalID     types.String `tfsdk:"external_id"`
}

func (d *OrganizationGroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_group"
}

func (d *OrganizationGroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This data source gets a group of a Vaultwarden organization by its ID or name.\n\n" +
			"Groups require the `ORG_GROUPS_ENABLED=true` setting on the Vaultwarden server.",
		Attributes: map[string]schema.Attribute{
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "ID of the organization that the group belongs to",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "ID of the group. Exactly one of `id` and `name` must be set",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the group. Exactly one of `id` and `name` must be set. The name must match exactly one group",
				Optional:            true,
				Computed:            true,
			},
			"external_id": schema.StringAttribute{
				MarkdownDescription: "The external ID of the group",
				Computed:            true,
			},
		},
	}
}

func (d *OrganizationGroupDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *OrganizationGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*vaultwarden.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *vaultwarden.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *OrganizationGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data OrganizationGroupDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	orgID := data.OrganizationID.ValueString()
	var group *models.Group
	if !data.ID.IsNull() {
		details, err := d.client.GetOrganizationGroup(ctx, orgID, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Error reading Vaultwarden organization group",
				fmt.Sprintf("Could not read group %s of organization %s: %s", data.ID.ValueString(), orgID, err),
			)
			return
		}
		group = &details.Group
	} else {
		groups, err := d.client.GetOrganizationGroups(ctx, orgID)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error reading Vaultwarden organization groups",
				fmt.Sprintf("Could not list the groups of organization %s: %s", orgID, err),
			)
			return
		}

		name := data.Name.ValueString()
		var matches []models.Group
		for _, g := range groups {
			if g.Name == name {
				matches = append(matches, g)
			}
		}
		switch len(matches) {
		case 0:
			resp.Diagnostics.AddError(
				"Organization group not found",
				fmt.Sprintf("Organization %s has no group named %q. Groups also require the ORG_GROUPS_ENABLED=true setting on the Vaultwarden server.", orgID, name),
			)
			return
		case 1:
			group = &matches[0]
		default:
			resp.Diagnostics.AddError(
				"Multiple organization groups found",
				fmt.Sprintf("Organization %s has %d groups named %q. Use id to select one.", orgID, len(matches), name),
			)
			return
		}
	}

	data.ID = types.StringValue(group.ID)
	data.Name = types.StringValue(group.Name)
	data.ExternalID = types.StringPointerValue(group.ExternalID)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
