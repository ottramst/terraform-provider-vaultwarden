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
var _ resource.Resource = &OrganizationCollectionGroup{}
var _ resource.ResourceWithConfigure = &OrganizationCollectionGroup{}
var _ resource.ResourceWithImportState = &OrganizationCollectionGroup{}

func OrganizationCollectionGroupResource() resource.Resource {
	return &OrganizationCollectionGroup{}
}

// OrganizationCollectionGroup defines the resource implementation.
type OrganizationCollectionGroup struct {
	client *vaultwarden.Client
}

// OrganizationCollectionGroupModel describes the resource data model.
type OrganizationCollectionGroupModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	CollectionID   types.String `tfsdk:"collection_id"`
	GroupID        types.String `tfsdk:"group_id"`
	ReadOnly       types.Bool   `tfsdk:"read_only"`
	HidePasswords  types.Bool   `tfsdk:"hide_passwords"`
	Manage         types.Bool   `tfsdk:"manage"`
}

func (r *OrganizationCollectionGroup) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_collection_group"
}

func (r *OrganizationCollectionGroup) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This resource gives a group access to a collection.\n\n" +
			"Access of other users and groups to the collection is left unchanged. " +
			"Groups require the `ORG_GROUPS_ENABLED=true` setting on the Vaultwarden server.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the collection access, in the format `<organization_id>/<collection_id>/<group_id>`",
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
			"group_id": schema.StringAttribute{
				MarkdownDescription: "ID of the group, for example `vaultwarden_organization_group.example.id`",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"read_only": schema.BoolAttribute{
				MarkdownDescription: "Whether the members of the group can only view the items in the collection. Defaults to `false`",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"hide_passwords": schema.BoolAttribute{
				MarkdownDescription: "Whether passwords are hidden from the members of the group. Defaults to `false`",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"manage": schema.BoolAttribute{
				MarkdownDescription: "Whether the members of the group can manage the collection, including its access. " +
					"Vaultwarden reports a group that can edit the items and see their passwords as able to manage the collection, " +
					"so `manage = false` is not reported as drift in that case. Defaults to `false`",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
		},
	}
}

func (r *OrganizationCollectionGroup) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrganizationCollectionGroup) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrganizationCollectionGroupModel

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
		data.OrganizationID.ValueString(), data.CollectionID.ValueString(), data.GroupID.ValueString(),
	}, "/"))

	tflog.Trace(ctx, fmt.Sprintf("created a new organization collection group with ID: %s", data.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationCollectionGroup) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrganizationCollectionGroupModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Get refreshed data from the client
	access, err := r.client.GetOrganizationCollectionGroup(ctx, data.OrganizationID.ValueString(), data.CollectionID.ValueString(), data.GroupID.ValueString())
	if err != nil {
		// Removed outside Terraform, or the collection or organization was deleted
		if vaultwarden.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden organization collection group",
			"Could not read the access of the group to the collection, unexpected error: "+err.Error(),
		)
		return
	}

	r.updateModel(&data, access)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationCollectionGroup) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data OrganizationCollectionGroupModel

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

func (r *OrganizationCollectionGroup) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OrganizationCollectionGroupModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.RemoveOrganizationCollectionGroup(ctx, data.OrganizationID.ValueString(), data.CollectionID.ValueString(), data.GroupID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Vaultwarden organization collection group",
			"Could not remove the access of the group to the collection: "+err.Error(),
		)
		return
	}
}

func (r *OrganizationCollectionGroup) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, "/")
	if len(idParts) != 3 || idParts[0] == "" || idParts[1] == "" || idParts[2] == "" {
		resp.Diagnostics.AddError(
			"Invalid ID format",
			"Expected import identifier with format: organization_id/collection_id/group_id",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("organization_id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("collection_id"), idParts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), idParts[2])...)
}

// set grants or changes the access in data.
func (r *OrganizationCollectionGroup) set(ctx context.Context, data *OrganizationCollectionGroupModel, diags *diag.Diagnostics) {
	err := r.client.SetOrganizationCollectionGroup(ctx, data.OrganizationID.ValueString(), data.CollectionID.ValueString(), models.CollectionAccess{
		ID:            data.GroupID.ValueString(),
		ReadOnly:      data.ReadOnly.ValueBool(),
		HidePasswords: data.HidePasswords.ValueBool(),
		Manage:        data.Manage.ValueBool(),
	})
	if err != nil {
		diags.AddError(
			"Error setting Vaultwarden organization collection group",
			"Could not give the group access to the collection: "+err.Error(),
		)
	}
}

// updateModel copies the access read from the server into data.
func (r *OrganizationCollectionGroup) updateModel(data *OrganizationCollectionGroupModel, access *models.CollectionAccess) {
	data.ReadOnly = types.BoolValue(access.ReadOnly)
	data.HidePasswords = types.BoolValue(access.HidePasswords)

	// Vaultwarden reports manage for a group that can edit the items and see their passwords,
	// whether or not it was granted. Keep the configured value in that case.
	if access.Manage && !access.ReadOnly && !access.HidePasswords && !data.Manage.IsNull() {
		return
	}
	data.Manage = types.BoolValue(access.Manage)
}
