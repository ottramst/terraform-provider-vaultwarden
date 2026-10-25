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
var _ resource.ResourceWithValidateConfig = &OrganizationUser{}
var _ resource.ResourceWithModifyPlan = &OrganizationUser{}

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
	AutoConfirm    types.Bool   `tfsdk:"auto_confirm"`
	Revoked        types.Bool   `tfsdk:"revoked"`
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
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "ID of the organization to invite the user to",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "The email of the user to invite. Changing it removes the user from the organization and invites the new email.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
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
				MarkdownDescription: "Whether the user has access to all collections in the organization. " +
					"`Owner` and `Admin` users always have access to all collections. " +
					"For `Manager` users it can be set to `true` (the *Manage all collections* permission). " +
					"`User` users cannot have it; grant them access to individual collections instead. " +
					"Defaults to `true` for `Owner` and `Admin`, and `false` otherwise.",
				Computed: true,
				Optional: true,
			},
			"auto_confirm": schema.BoolAttribute{
				MarkdownDescription: "Whether to confirm the user once they have accepted the invitation. " +
					"Until a user is confirmed, they cannot decrypt the organization's items. " +
					"Confirming encrypts the organization key with the user's public key, which Vaultwarden only allows after the user accepted the invitation. " +
					"When the user has accepted, the plan shows `status` as changing and the apply confirms them; while they are still `Invited`, nothing changes. " +
					"Setting it to `false` does not unconfirm a confirmed user. Defaults to `false`.",
				Computed: true,
				Optional: true,
				Default:  booldefault.StaticBool(false),
			},
			"revoked": schema.BoolAttribute{
				MarkdownDescription: "Whether the user's access to the organization is revoked. " +
					"A revoked user stays in the organization but cannot access its items; restoring them (`false`) brings back the status they had before. " +
					"Defaults to `false`.",
				Computed: true,
				Optional: true,
				Default:  booldefault.StaticBool(false),
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "The status of the user: `Invited`, `Accepted`, `Confirmed` or `Revoked`",
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("Revoked", "Invited", "Accepted", "Confirmed"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
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

func (r *OrganizationUser) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data OrganizationUserModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() || data.AccessAll.IsNull() || data.AccessAll.IsUnknown() || data.Type.IsUnknown() {
		return
	}

	userType := data.Type.ValueString()
	if data.Type.IsNull() {
		userType = "User"
	}

	switch {
	case userType == "User" && data.AccessAll.ValueBool():
		resp.Diagnostics.AddAttributeError(
			path.Root("access_all"),
			"Invalid access_all for User",
			"Vaultwarden does not allow users of type User to have access to all collections. "+
				"Use type Manager with access_all, or grant access to individual collections.",
		)
	case (userType == "Owner" || userType == "Admin") && !data.AccessAll.ValueBool():
		resp.Diagnostics.AddAttributeError(
			path.Root("access_all"),
			"Invalid access_all for "+userType,
			"Vaultwarden always gives users of type "+userType+" access to all collections. "+
				"Remove access_all or set it to true.",
		)
	}
}

func (r *OrganizationUser) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Nothing to do on destroy
	if req.Plan.Raw.IsNull() {
		return
	}

	var config, plan OrganizationUserModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Without an explicit value, access_all follows from the type
	if config.AccessAll.IsNull() && !plan.Type.IsUnknown() {
		userType := plan.Type.ValueString()
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("access_all"), userType == "Owner" || userType == "Admin")...)
	}

	// The status is unknown on create anyway
	if req.State.Raw.IsNull() {
		return
	}

	var state OrganizationUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The status changes when the user is revoked or restored, or when an accepted user is
	// confirmed. Planning it as unknown also makes a user that accepted the invitation since
	// the last apply show up as a change, so that the next apply confirms them.
	if !plan.Revoked.Equal(state.Revoked) || (shouldConfirm(plan) && state.Status.ValueString() == "Accepted") {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("status"), types.StringUnknown())...)
	}
}

// shouldConfirm reports whether the plan may confirm the user once they have accepted. A
// revoked user cannot be confirmed; they are confirmed once restored.
func shouldConfirm(plan OrganizationUserModel) bool {
	return (plan.AutoConfirm.IsUnknown() || plan.AutoConfirm.ValueBool()) && !plan.Revoked.ValueBool()
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
	requestType, permissions := vaultwarden.OrganizationUserRole(userType, data.AccessAll.ValueBool())
	inviteReq := vaultwarden.InviteOrganizationUserRequest{
		Type:        requestType,
		AccessAll:   data.AccessAll.ValueBool(),
		Permissions: permissions,
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

	// Map response body to schema and populate Computed attribute values. The state is saved
	// before confirming or revoking, so that a failure there leaves the user tracked (tainted).
	data.ID = types.StringValue(userResp.ID)
	plan := data
	setOrganizationUserState(&data, userResp)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)

	tflog.Trace(ctx, fmt.Sprintf("created a new user_invite with ID: %s", data.ID))

	// An existing account is accepted right away when the server does not send mail
	userResp, err = r.reconcileStatus(ctx, plan, userResp)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating organization user status",
			"Could not update the status of organization user with ID "+data.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	setOrganizationUserState(&data, userResp)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// setOrganizationUserState copies the server's view of the user, except the email, into the model.
func setOrganizationUserState(data *OrganizationUserModel, user *models.OrganizationUserDetails) {
	data.Status = types.StringValue(user.Status.String())
	data.Revoked = types.BoolValue(user.Status == models.UserOrgStatusRevoked)
	data.AccessAll = types.BoolValue(user.HasAccessAll())
	data.Type = types.StringValue(user.Type.String())
}

// reconcileStatus restores or revokes the user to match the plan, and confirms them when
// auto_confirm is set and they have accepted the invitation. It returns the user as the server
// has them afterwards.
func (r *OrganizationUser) reconcileStatus(ctx context.Context, plan OrganizationUserModel, user *models.OrganizationUserDetails) (*models.OrganizationUserDetails, error) {
	// The status may only change when the plan expects it to (see ModifyPlan). A user that
	// accepted after a saved plan was made is confirmed by the next plan and apply.
	if !plan.Status.IsUnknown() {
		return user, nil
	}

	userID, orgID := plan.ID.ValueString(), plan.OrganizationID.ValueString()
	revoke := plan.Revoked.ValueBool()

	refresh := func(err error) (*models.OrganizationUserDetails, error) {
		if err != nil {
			return nil, err
		}
		return r.client.GetOrganizationUser(ctx, userID, orgID)
	}

	var err error

	// A restored user gets back the status they had before they were revoked
	if !revoke && user.Status == models.UserOrgStatusRevoked {
		if user, err = refresh(r.client.RestoreOrganizationUser(ctx, userID, orgID)); err != nil {
			return nil, err
		}
	}

	if shouldConfirm(plan) && user.Status == models.UserOrgStatusAccepted {
		if user, err = refresh(r.client.ConfirmOrganizationUser(ctx, userID, orgID)); err != nil {
			return nil, err
		}
	}

	if revoke && user.Status != models.UserOrgStatusRevoked {
		if user, err = refresh(r.client.RevokeOrganizationUser(ctx, userID, orgID)); err != nil {
			return nil, err
		}
	}

	return user, nil
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
		// Deleted outside Terraform
		if vaultwarden.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error fetching organization user",
			"Could not fetch organization user, unexpected error: "+err.Error(),
		)
		return
	}

	// Overwrite the model with the refreshed data
	data.Email = types.StringValue(userResp.Email)
	setOrganizationUserState(&data, userResp)

	// State written by provider versions before auto_confirm existed has no value for it
	if data.AutoConfirm.IsNull() {
		data.AutoConfirm = types.BoolValue(false)
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationUser) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state OrganizationUserModel

	// Read Terraform plan and prior state data into the models
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Update the role, unless only the status changes
	if !data.Type.Equal(state.Type) || !data.AccessAll.Equal(state.AccessAll) {
		// Parse the type string into a UserOrgType (value will always be present due to schema default)
		var userType models.UserOrgType
		if err := userType.FromString(data.Type.ValueString()); err != nil {
			resp.Diagnostics.AddError(
				"Error parsing user type",
				"Could not parse user type: "+err.Error(),
			)
			return
		}

		requestType, permissions := vaultwarden.OrganizationUserRole(userType, data.AccessAll.ValueBool())
		updateReq := vaultwarden.UpdateOrganizationUserRequest{
			Type:        requestType,
			AccessAll:   data.AccessAll.ValueBool(),
			Permissions: permissions,
		}

		if err := r.client.UpdateOrganizationUser(ctx, data.ID.ValueString(), data.OrganizationID.ValueString(), updateReq); err != nil {
			resp.Diagnostics.AddError(
				"Error updating organization user",
				"Could not update organization user with ID "+data.ID.ValueString()+": "+err.Error(),
			)
			return
		}
	}

	// Read back what the server applied
	userResp, err := r.client.GetOrganizationUser(ctx, data.ID.ValueString(), data.OrganizationID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error fetching organization user",
			"Could not fetch organization user, unexpected error: "+err.Error(),
		)
		return
	}

	userResp, err = r.reconcileStatus(ctx, data, userResp)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating organization user status",
			"Could not update the status of organization user with ID "+data.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	setOrganizationUserState(&data, userResp)

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
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("access_all"), userResp.HasAccessAll())...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("status"), userResp.Status.String())...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("revoked"), userResp.Status == models.UserOrgStatusRevoked)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("auto_confirm"), false)...)
}
