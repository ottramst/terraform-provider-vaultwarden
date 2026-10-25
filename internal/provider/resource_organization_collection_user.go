package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &OrganizationCollectionUser{}
var _ resource.ResourceWithConfigure = &OrganizationCollectionUser{}
var _ resource.ResourceWithImportState = &OrganizationCollectionUser{}

func OrganizationCollectionUserResource() resource.Resource {
	return &OrganizationCollectionUser{}
}

// OrganizationCollectionUser defines the resource implementation.
type OrganizationCollectionUser struct {
	client *vaultwarden.Client
}

// OrganizationCollectionUserModel describes the resource data model.
type OrganizationCollectionUserModel struct {
	ID                 types.String `tfsdk:"id"`
	OrganizationID     types.String `tfsdk:"organization_id"`
	CollectionID       types.String `tfsdk:"collection_id"`
	OrganizationUserID types.String `tfsdk:"organization_user_id"`
	ReadOnly           types.Bool   `tfsdk:"read_only"`
	HidePasswords      types.Bool   `tfsdk:"hide_passwords"`
	Manage             types.Bool   `tfsdk:"manage"`
}

func (r *OrganizationCollectionUser) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_collection_user"
}

func (r *OrganizationCollectionUser) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This resource gives an organization user access to a collection.\n\n" +
			"Access of other users and groups to the collection is left unchanged. " +
			"Users with access to all collections (`Owner`, `Admin`, or a `Manager` with `access_all`) cannot be assigned to individual collections.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the collection access, in the format `<organization_id>/<collection_id>/<organization_user_id>`",
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
			"collection_id": schema.StringAttribute{
				MarkdownDescription: "ID of the collection",
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
			"read_only": schema.BoolAttribute{
				MarkdownDescription: "Whether the user can only view the items in the collection. Defaults to `false`",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"hide_passwords": schema.BoolAttribute{
				MarkdownDescription: "Whether passwords are hidden from the user. Defaults to `false`",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"manage": schema.BoolAttribute{
				MarkdownDescription: "Whether the user can manage the collection, including its access. " +
					"A `Manager` that can edit the collection and see its passwords can always manage it. Defaults to `false`",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
		},
	}
}

func (r *OrganizationCollectionUser) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrganizationCollectionUser) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrganizationCollectionUserModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	r.set(ctx, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = types.StringValue(strings.Join([]string{
		data.OrganizationID.ValueString(), data.CollectionID.ValueString(), data.OrganizationUserID.ValueString(),
	}, "/"))

	tflog.Trace(ctx, fmt.Sprintf("created a new organization collection user with ID: %s", data.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationCollectionUser) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrganizationCollectionUserModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Get refreshed data from the client
	access, err := r.client.GetOrganizationCollectionUser(ctx, data.OrganizationID.ValueString(), data.CollectionID.ValueString(), data.OrganizationUserID.ValueString())
	if err != nil {
		// Removed outside Terraform, or the collection or organization was deleted
		if vaultwarden.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden organization collection user",
			"Could not read the access of the organization user to the collection, unexpected error: "+err.Error(),
		)
		return
	}

	r.updateModel(ctx, &data, access, &resp.Diagnostics)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationCollectionUser) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data OrganizationCollectionUserModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	r.set(ctx, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationCollectionUser) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OrganizationCollectionUserModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.RemoveOrganizationCollectionUser(ctx, data.OrganizationID.ValueString(), data.CollectionID.ValueString(), data.OrganizationUserID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Vaultwarden organization collection user",
			"Could not remove the access of the organization user to the collection: "+err.Error(),
		)
		return
	}
}

func (r *OrganizationCollectionUser) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, "/")
	if len(idParts) != 3 || idParts[0] == "" || idParts[1] == "" || idParts[2] == "" {
		resp.Diagnostics.AddError(
			"Invalid ID format",
			"Expected import identifier with format: organization_id/collection_id/organization_user_id",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("organization_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("collection_id"), idParts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("organization_user_id"), idParts[2])...)
}

// set grants or changes the access in data and reads it back.
func (r *OrganizationCollectionUser) set(ctx context.Context, data *OrganizationCollectionUserModel, diags *diag.Diagnostics) {
	orgID, colID, userID := data.OrganizationID.ValueString(), data.CollectionID.ValueString(), data.OrganizationUserID.ValueString()

	err := r.client.SetOrganizationCollectionUser(ctx, orgID, colID, models.CollectionAccess{
		ID:            userID,
		ReadOnly:      data.ReadOnly.ValueBool(),
		HidePasswords: data.HidePasswords.ValueBool(),
		Manage:        data.Manage.ValueBool(),
	})
	if err != nil {
		diags.AddError(
			"Error setting Vaultwarden organization collection user",
			"Could not give the organization user access to the collection: "+err.Error(),
		)
		return
	}

	// Vaultwarden silently skips users with access to all collections
	if _, err := r.client.GetOrganizationCollectionUser(ctx, orgID, colID, userID); err != nil {
		if vaultwarden.IsNotFound(err) {
			diags.AddError(
				"Organization user has access to all collections",
				"Vaultwarden did not assign organization user "+userID+" to collection "+colID+". "+
					"Owners, Admins and Managers with access_all already have access to all collections and cannot be assigned to individual ones.",
			)
			return
		}
		diags.AddError(
			"Error reading Vaultwarden organization collection user",
			"Could not read the access of the organization user to the collection, unexpected error: "+err.Error(),
		)
	}
}

// updateModel copies the access read from the server into data.
func (r *OrganizationCollectionUser) updateModel(ctx context.Context, data *OrganizationCollectionUserModel, access *models.CollectionAccess, diags *diag.Diagnostics) {
	data.ReadOnly = types.BoolValue(access.ReadOnly)
	data.HidePasswords = types.BoolValue(access.HidePasswords)

	// Vaultwarden reports manage for a Manager that can edit the collection and see its
	// passwords, whether or not it was granted. Keep the configured value in that case.
	if access.Manage && !data.Manage.ValueBool() && !access.ReadOnly && !access.HidePasswords {
		user, err := r.client.GetOrganizationUser(ctx, data.OrganizationUserID.ValueString(), data.OrganizationID.ValueString())
		if err != nil {
			diags.AddError(
				"Error fetching organization user",
				"Could not fetch organization user, unexpected error: "+err.Error(),
			)
			return
		}
		if user.Type == models.UserOrgTypeManager || user.Type == models.UserOrgTypeCustom {
			return
		}
	}
	data.Manage = types.BoolValue(access.Manage)
}
