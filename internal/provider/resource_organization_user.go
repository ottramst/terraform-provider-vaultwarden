package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"strings"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &OrganizationUser{}
var _ resource.ResourceWithConfigure = &OrganizationUser{}
var _ resource.ResourceWithImportState = &OrganizationUser{}

func OrganizationUserResource() resource.Resource {
	return &OrganizationUser{}
}

// OrganizationUser defines the resource implementation.
type OrganizationUser struct {
	client *vaultwarden.Client
}

// OrganizationUserModel describes the resource data model.
type OrganizationUserModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Email          types.String `tfsdk:"email"`
	Type           types.String `tfsdk:"type"`
	AccessAll      types.Bool   `tfsdk:"access_all"`
	Status         types.String `tfsdk:"status"`
}

func (r *OrganizationUser) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_user"
}

func (r *OrganizationUser) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This resource invites a user to an organization on the Vaultwarden server.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the invited user",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "ID of the organization to invite the user to",
				Required:            true,
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "The email of the user to invite",
				Required:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "The role type of the user (Owner, Admin, User, Manager). Defaults to `User`",
				Computed:            true,
				Optional:            true,
				Default:             stringdefault.StaticString("User"),
				Validators: []validator.String{
					stringvalidator.OneOf("Owner", "Admin", "User", "Manager"),
				},
			},
			"access_all": schema.BoolAttribute{
				MarkdownDescription: "Whether the user has access to all collections in the organization. Defaults to `false`",
				Computed:            true,
				Optional:            true,
				Default:             booldefault.StaticBool(false),
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "The status of the user",
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("Revoked", "Invited", "Accepted", "Confirmed"),
				},
			},
		},
	}
}

func (r *OrganizationUser) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrganizationUser) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrganizationUserModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Parse the type string into a UserOrgType (value will always be present due to schema default)
	var userType models.UserOrgType
	if err := userType.FromString(data.Type.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error parsing user type",
			"Could not parse user type: "+err.Error(),
		)
		return
	}

	// Call the client method to invite the user
	inviteReq := vaultwarden.InviteOrganizationUserRequest{
		Type:      userType,
		AccessAll: data.AccessAll.ValueBool(),
	}

	if err := r.client.InviteOrganizationUser(ctx, inviteReq, data.Email.ValueString(), data.OrganizationID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error inviting user",
			"Could not invite user, unexpected error: "+err.Error(),
		)
		return
	}

	// Fetch the invited user by email
	userResp, err := r.client.GetOrganizationUserByEmail(ctx, data.Email.ValueString(), data.OrganizationID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error fetching registered user",
			"Could not fetch registered user, unexpected error: "+err.Error(),
		)
		return
	}

	// Map response body to schema and populate Computed attribute values
	data.ID = types.StringValue(userResp.ID)
	data.Status = types.StringValue(userResp.Status.String())
	data.AccessAll = types.BoolValue(userResp.AccessAll)
	data.Type = types.StringValue(userResp.Type.String())

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, fmt.Sprintf("created a new user_invite with ID: %s", data.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationUser) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrganizationUserModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Get refreshed data from the client
	userResp, err := r.client.GetOrganizationUser(ctx, data.ID.ValueString(), data.OrganizationID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error fetching organization user",
			"Could not fetch organization user, unexpected error: "+err.Error(),
		)
		return
	}

	// Overwrite the model with the refreshed data
	data.Email = types.StringValue(userResp.Email)
	data.Status = types.StringValue(userResp.Status.String())
	data.AccessAll = types.BoolValue(userResp.AccessAll)
	data.Type = types.StringValue(userResp.Type.String())

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationUser) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data OrganizationUserModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Parse the type string into a UserOrgType (value will always be present due to schema default)
	var userType models.UserOrgType
	if err := userType.FromString(data.Type.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error parsing user type",
			"Could not parse user type: "+err.Error(),
		)
		return
	}

	// Update the user if needed
	user := models.OrganizationUserDetails{
		Email:     data.Email.ValueString(),
		Type:      userType,
		AccessAll: data.AccessAll.ValueBool(),
	}

	if _, err := r.client.UpdateOrganizationUser(ctx, data.ID.ValueString(), data.OrganizationID.ValueString(), user); err != nil {
		resp.Diagnostics.AddError(
			"Error updating organization user",
			"Could not update organization user with ID "+data.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationUser) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OrganizationUserModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Delete the user
	if err := r.client.DeleteOrganizationUser(ctx, data.ID.ValueString(), data.OrganizationID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting organization user",
			"Could not delete organization user with ID "+data.ID.ValueString()+": "+err.Error(),
		)
		return
	}
}

func (r *OrganizationUser) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, "/")
	if len(idParts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid ID format",
			"Expected import identifier with format: organization_id/user_id",
		)
		return
	}

	organizationID := idParts[0]
	userID := idParts[1]

	// Set the organization_id and id attributes
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("organization_id"), organizationID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), userID)...)

	// After setting the IDs, fetch the current state of the resource
	userResp, err := r.client.GetOrganizationUser(ctx, userID, organizationID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error fetching organization user",
			"Could not fetch organization user, unexpected error: "+err.Error(),
		)
		return
	}

	// Map response body to schema and populate the rest of the attributes
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("email"), userResp.Email)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), userResp.Type.String())...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("access_all"), userResp.AccessAll)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("status"), userResp.Status.String())...)
}
