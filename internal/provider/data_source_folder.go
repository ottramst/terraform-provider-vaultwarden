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
var _ datasource.DataSource = &FolderDataSource{}
var _ datasource.DataSourceWithConfigure = &FolderDataSource{}
var _ datasource.DataSourceWithConfigValidators = &FolderDataSource{}

func NewFolderDataSource() datasource.DataSource {
	return &FolderDataSource{}
}

// FolderDataSource looks up a folder of the provider user.
type FolderDataSource struct {
	client *vaultwarden.Client
}

// FolderDataSourceModel describes the vaultwarden_folder data source.
type FolderDataSourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (d *FolderDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_folder"
}

func (d *FolderDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to look up a folder in the personal vault of the provider user by ID or name.\n\n" +
			"Requires `email` and `master_password` to be set in the provider configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The ID of the folder to look up. Exactly one of `id` and `name` must be set",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the folder to look up, compared exactly. Folder names are not unique: " +
					"when several folders have the name, the lookup fails and lists their IDs. Exactly one of `id` and `name` must be set",
				Optional: true,
				Computed: true,
			},
		},
	}
}

func (d *FolderDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *FolderDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureLookupClient(req, resp); client != nil {
		d.client = client
	}
}

func (d *FolderDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data FolderDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Look the folder up by ID directly, so that other folders need not be decryptable
	if !data.ID.IsNull() {
		folder, err := d.client.GetFolder(ctx, data.ID.ValueString())
		if err != nil {
			if vaultwarden.IsNotFound(err) {
				resp.Diagnostics.AddError(
					"Folder Not Found",
					fmt.Sprintf("The provider user has no folder with ID %q.", data.ID.ValueString()),
				)
				return
			}
			resp.Diagnostics.AddError("Error Reading Folder", "Could not read folder "+data.ID.ValueString()+": "+err.Error())
			return
		}
		data.Name = types.StringValue(folder.Name)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	folders, err := d.client.ListFolders(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Folders", "Could not list the folders of the provider user: "+err.Error())
		return
	}

	name := data.Name.ValueString()
	var matches []models.Folder
	for _, folder := range folders {
		if folder.Name == name {
			matches = append(matches, folder)
		}
	}

	switch len(matches) {
	case 0:
		resp.Diagnostics.AddError(
			"Folder Not Found",
			fmt.Sprintf("The provider user has no folder with name %q.", name),
		)
		return
	case 1:
	default:
		ids := make([]string, 0, len(matches))
		for _, folder := range matches {
			ids = append(ids, folder.ID)
		}
		sort.Strings(ids)
		resp.Diagnostics.AddError(
			"Multiple Folders Found",
			fmt.Sprintf("The provider user has %d folders with name %q (IDs: %s). Look the folder up by `id` instead.",
				len(matches), name, strings.Join(ids, ", ")),
		)
		return
	}

	data.ID = types.StringValue(matches[0].ID)
	data.Name = types.StringValue(matches[0].Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
