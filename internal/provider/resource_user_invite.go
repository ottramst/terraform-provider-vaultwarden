package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"net/http"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &UserInvite{}
var _ resource.ResourceWithImportState = &UserInvite{}

func UserInviteResource() resource.Resource {
	return &UserInvite{}
}

// UserInvite defines the resource implementation.
type UserInvite struct {
	client *vaultwarden.Client
}

// UserInviteModel describes the resource data model.
type UserInviteModel struct {
	Email types.String `tfsdk:"email"`
	ID    types.String `tfsdk:"id"`
}

func (r *UserInvite) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_invite"
}

func (r *UserInvite) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This resource invites a user to the Vaultwarden server.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the invited user",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "The email of the user to invite",
				Required:            true,
			},
		},
	}
}

func (r *UserInvite) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *UserInvite) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data UserInviteModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Call the client method to invite the user
	user, err := r.client.InviteUser(data.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error inviting user",
			"Could not invite user, unexpected error: "+err.Error(),
		)
		return
	}

	// Map response body to schema and populate Computed attribute values
	data.ID = types.StringValue(user.ID)

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, fmt.Sprintf("created a new user_invite with ID: %s", data.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserInvite) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data UserInviteModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Get refreshed data from the client
	user, httpResp, err := r.client.GetUser(data.ID.ValueString())
	if err != nil {
		// If the user is not found in Vaultwarden, tell Terraform the resource needs to be recreated
		// instead of returning an error
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			tflog.Debug(ctx, "User not found, recreating resource", map[string]interface{}{
				"email": data.Email.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}

		// Otherwise, return an error
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden user",
			"Could not read user with ID "+data.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// Overwrite the model with the refreshed data
	data.ID = types.StringValue(user.ID)
	data.Email = types.StringValue(user.Email)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserInvite) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data UserInviteModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserInvite) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data UserInviteModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Delete the user
	if err := r.client.DeleteUser(data.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Vaultwarden user",
			"Could not delete user with ID "+data.ID.ValueString()+": "+err.Error(),
		)
		return
	}
}

func (r *UserInvite) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
