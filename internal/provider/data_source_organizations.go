package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &OrganizationsDataSource{}
var _ datasource.DataSourceWithConfigure = &OrganizationsDataSource{}

func NewOrganizationsDataSource() datasource.DataSource {
	return &OrganizationsDataSource{}
}

// OrganizationsDataSource lists the organizations the provider user is a member of.
type OrganizationsDataSource struct {
	client *vaultwarden.Client
}

// OrganizationsDataSourceModel describes the vaultwarden_organizations data source.
type OrganizationsDataSourceModel struct {
	Organizations []OrganizationsItemModel `tfsdk:"organizations"`
}

// OrganizationsItemModel describes an organization the provider user is a member of.
type OrganizationsItemModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	OrganizationUserID types.String `tfsdk:"organization_user_id"`
	Type               types.String `tfsdk:"type"`
}

func (d *OrganizationsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organizations"
}

func (d *OrganizationsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to list the organizations the provider user is a confirmed member of. " +
			"Organizations the user is only invited to, or has accepted but is not yet confirmed in, are not listed.\n\n" +
			"Requires `email` and `master_password` to be set in the provider configuration.",
		Attributes: map[string]schema.Attribute{
			"organizations": schema.ListNestedAttribute{
				MarkdownDescription: "The organizations, sorted by name",
				Computed:            true,
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
							MarkdownDescription: "The ID of the provider user's membership in the organization",
							Computed:            true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: "The type of the provider user in the organization: `Owner`, `Admin`, `User` or `Manager`",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *OrganizationsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureLookupClient(req, resp); client != nil {
		d.client = client
	}
}

func (d *OrganizationsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data OrganizationsDataSourceModel

	memberships, err := d.client.GetOrganizationMemberships(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Organizations", fmt.Sprintf("Could not list the organizations of the provider user: %s", err))
		return
	}

	sort.SliceStable(memberships, func(i, j int) bool {
		if memberships[i].Name != memberships[j].Name {
			return memberships[i].Name < memberships[j].Name
		}
		return memberships[i].ID < memberships[j].ID
	})

	data.Organizations = make([]OrganizationsItemModel, 0, len(memberships))
	for _, membership := range memberships {
		data.Organizations = append(data.Organizations, OrganizationsItemModel{
			ID:                 types.StringValue(membership.ID),
			Name:               types.StringValue(membership.Name),
			OrganizationUserID: types.StringValue(membership.OrganizationUserID),
			Type:               types.StringValue(membership.Type.String()),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
