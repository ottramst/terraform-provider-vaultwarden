package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &OrganizationGroupMember{}
var _ resource.ResourceWithConfigure = &OrganizationGroupMember{}
var _ resource.ResourceWithImportState = &OrganizationGroupMember{}

func OrganizationGroupMemberResource() resource.Resource {
	return &OrganizationGroupMember{}
}

// OrganizationGroupMember defines the resource implementation.
type OrganizationGroupMember struct {
	client *vaultwarden.Client
}

// OrganizationGroupMemberModel describes the resource data model.
type OrganizationGroupMemberModel struct {
	ID                 types.String `tfsdk:"id"`
	OrganizationID     types.String `tfsdk:"organization_id"`
	GroupID            types.String `tfsdk:"group_id"`
	OrganizationUserID types.String `tfsdk:"organization_user_id"`
}

func (r *OrganizationGroupMember) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_group_member"
}

func (r *OrganizationGroupMember) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This resource adds an organization user to a group.\n\n" +
			"Other members of the group and other groups of the user are left unchanged. " +
			"Groups require the `ORG_GROUPS_ENABLED=true` setting on the Vaultwarden server.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the group membership, in the format `<organization_id>/<group_id>/<organization_user_id>`",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "ID of the organization",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "ID of the group, for example `vaultwarden_organization_group.example.id`",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"organization_user_id": schema.StringAttribute{
				MarkdownDescription: "ID of the organization user, for example `vaultwarden_organization_user.example.id`",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *OrganizationGroupMember) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrganizationGroupMember) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrganizationGroupMemberModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	orgID, groupID, userID := data.OrganizationID.ValueString(), data.GroupID.ValueString(), data.OrganizationUserID.ValueString()
	if err := r.client.AddOrganizationGroupMember(ctx, orgID, groupID, userID); err != nil {
		resp.Diagnostics.AddError(
			"Error adding Vaultwarden organization group member",
			"Could not add the organization user to the group: "+err.Error(),
		)
		return
	}

	data.ID = types.StringValue(strings.Join([]string{orgID, groupID, userID}, "/"))

	tflog.Trace(ctx, fmt.Sprintf("created a new organization group member with ID: %s", data.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationGroupMember) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrganizationGroupMemberModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.GetOrganizationGroupMember(ctx, data.OrganizationID.ValueString(), data.GroupID.ValueString(), data.OrganizationUserID.ValueString())
	if err != nil {
		// Removed outside Terraform, or the group or organization was deleted
		if vaultwarden.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden organization group member",
			"Could not read the members of the group, unexpected error: "+err.Error(),
		)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update is never called with changes: every attribute requires replacement.
func (r *OrganizationGroupMember) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data OrganizationGroupMemberModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationGroupMember) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OrganizationGroupMemberModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.RemoveOrganizationGroupMember(ctx, data.OrganizationID.ValueString(), data.GroupID.ValueString(), data.OrganizationUserID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Vaultwarden organization group member",
			"Could not remove the organization user from the group: "+err.Error(),
		)
		return
	}
}

func (r *OrganizationGroupMember) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, "/")
	if len(idParts) != 3 || idParts[0] == "" || idParts[1] == "" || idParts[2] == "" {
		resp.Diagnostics.AddError(
			"Invalid ID format",
			"Expected import identifier with format: organization_id/group_id/organization_user_id",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("organization_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), idParts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("organization_user_id"), idParts[2])...)
}
