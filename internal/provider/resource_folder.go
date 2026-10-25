package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &Folder{}
var _ resource.ResourceWithConfigure = &Folder{}
var _ resource.ResourceWithImportState = &Folder{}

func FolderResource() resource.Resource {
	return &Folder{}
}

// Folder defines the resource implementation.
type Folder struct {
	client *vaultwarden.Client
}

// FolderModel describes the resource data model.
type FolderModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (r *Folder) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_folder"
}

func (r *Folder) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This resource creates a folder in the personal vault of the provider user. " +
			"The folder name is encrypted with the user's key. Deleting a folder keeps the items in it and moves them out of the folder.\n\n" +
			"Requires `email` and `master_password` to be set in the provider configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the folder",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the folder. Bitwarden clients show a name with slashes, such as `Parent/Child`, as a nested folder",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
		},
	}
}

func (r *Folder) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*vaultwarden.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *vaultwarden.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *Folder) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data FolderModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	folder, err := r.client.CreateFolder(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating Vaultwarden folder",
			"Could not create folder, unexpected error: "+err.Error(),
		)
		return
	}
	updateFolderModel(&data, folder)

	tflog.Trace(ctx, fmt.Sprintf("created a new folder with ID: %s", data.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *Folder) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data FolderModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	folder, err := r.client.GetFolder(ctx, data.ID.ValueString())
	if err != nil {
		// Deleted outside Terraform
		if vaultwarden.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden folder",
			"Could not read folder "+data.ID.ValueString()+", unexpected error: "+err.Error(),
		)
		return
	}
	updateFolderModel(&data, folder)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *Folder) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data FolderModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	folder, err := r.client.UpdateFolder(ctx, data.ID.ValueString(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating Vaultwarden folder",
			"Could not update folder "+data.ID.ValueString()+", unexpected error: "+err.Error(),
		)
		return
	}
	updateFolderModel(&data, folder)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *Folder) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data FolderModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteFolder(ctx, data.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Vaultwarden folder",
			"Could not delete folder "+data.ID.ValueString()+", unexpected error: "+err.Error(),
		)
		return
	}
}

func (r *Folder) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// updateFolderModel copies the folder read from the server into data.
func updateFolderModel(data *FolderModel, folder *models.Folder) {
	data.ID = types.StringValue(folder.ID)
	data.Name = types.StringValue(folder.Name)
}
