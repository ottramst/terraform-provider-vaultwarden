package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &OrganizationGroup{}
var _ resource.ResourceWithConfigure = &OrganizationGroup{}
var _ resource.ResourceWithImportState = &OrganizationGroup{}
var _ resource.ResourceWithIdentity = &OrganizationGroup{}

func OrganizationGroupResource() resource.Resource {
	return &OrganizationGroup{}
}

// OrganizationGroup defines the resource implementation.
type OrganizationGroup struct {
	client *vaultwarden.Client
}

// OrganizationGroupModel describes the resource data model.
type OrganizationGroupModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Name           types.String `tfsdk:"name"`
	AccessAll      types.Bool   `tfsdk:"access_all"`
	ExternalID     types.String `tfsdk:"external_id"`
}

func (r *OrganizationGroup) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_group"
}

func (r *OrganizationGroup) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This resource creates a group in a Vaultwarden organization.\n\n" +
			"Groups require the `ORG_GROUPS_ENABLED=true` setting on the Vaultwarden server. " +
			"Updates keep the group's members and collection access, including ones managed outside Terraform.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the group",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "ID of the organization that the group belongs to",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the group",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"access_all": schema.BoolAttribute{
				MarkdownDescription: "Whether the members of the group have access to all collections of the organization. " +
					"Defaults to `false`. Vaultwarden 1.37 and later do not return this value, so changes made outside Terraform are not detected on those versions",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"external_id": schema.StringAttribute{
				MarkdownDescription: "An identifier to link the group to an external system, such as a directory service. " +
					"Vaultwarden cannot change the external ID of an existing group, so changing it replaces the group",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`\S`), "must not be empty"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *OrganizationGroup) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = organizationObjectIdentitySchema("group")
}

func (r *OrganizationGroup) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrganizationGroup) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrganizationGroupModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	orgID := data.OrganizationID.ValueString()
	group, err := r.client.CreateOrganizationGroup(ctx, orgID, r.toGroup(&data))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating Vaultwarden organization group",
			"Could not create organization group, unexpected error: "+err.Error(),
		)
		return
	}
	data.ID = types.StringValue(group.ID)
	resp.Diagnostics.Append(setOrganizationObjectIdentity(ctx, resp.Identity, data.OrganizationID, data.ID)...)

	tflog.Trace(ctx, fmt.Sprintf("created a new organization group with ID: %s", data.ID))

	details, err := r.client.GetOrganizationGroup(ctx, orgID, group.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden organization group",
			"Could not read organization group "+group.ID+", unexpected error: "+err.Error(),
		)
		return
	}
	updateOrganizationGroupModel(&data, &details.Group)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationGroup) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrganizationGroupModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	group, err := r.client.GetOrganizationGroup(ctx, data.OrganizationID.ValueString(), data.ID.ValueString())
	if err != nil {
		// Deleted outside Terraform, or the organization was deleted
		if vaultwarden.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden organization group",
			"Could not read organization group "+data.ID.ValueString()+", unexpected error: "+err.Error(),
		)
		return
	}
	updateOrganizationGroupModel(&data, &group.Group)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setOrganizationObjectIdentity(ctx, resp.Identity, data.OrganizationID, data.ID)...)
}

func (r *OrganizationGroup) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data OrganizationGroupModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	orgID, groupID := data.OrganizationID.ValueString(), data.ID.ValueString()
	if _, err := r.client.UpdateOrganizationGroup(ctx, orgID, groupID, r.toGroup(&data)); err != nil {
		resp.Diagnostics.AddError(
			"Error updating Vaultwarden organization group",
			"Could not update organization group "+groupID+", unexpected error: "+err.Error(),
		)
		return
	}

	group, err := r.client.GetOrganizationGroup(ctx, orgID, groupID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden organization group",
			"Could not read organization group "+groupID+", unexpected error: "+err.Error(),
		)
		return
	}
	updateOrganizationGroupModel(&data, &group.Group)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	resp.Diagnostics.Append(setOrganizationObjectIdentity(ctx, resp.Identity, data.OrganizationID, data.ID)...)
}

func (r *OrganizationGroup) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OrganizationGroupModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteOrganizationGroup(ctx, data.OrganizationID.ValueString(), data.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Vaultwarden organization group",
			"Could not delete organization group "+data.ID.ValueString()+", unexpected error: "+err.Error(),
		)
		return
	}
}

func (r *OrganizationGroup) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importOrganizationObject(ctx, req, resp, "group_id")
}

// toGroup returns the group to send for data.
func (r *OrganizationGroup) toGroup(data *OrganizationGroupModel) models.Group {
	accessAll := data.AccessAll.ValueBool()
	return models.Group{
		Name:       data.Name.ValueString(),
		AccessAll:  &accessAll,
		ExternalID: data.ExternalID.ValueStringPointer(),
	}
}

// updateOrganizationGroupModel copies the group read from the server into data.
func updateOrganizationGroupModel(data *OrganizationGroupModel, group *models.Group) {
	data.ID = types.StringValue(group.ID)
	data.OrganizationID = types.StringValue(group.OrganizationID)
	data.Name = types.StringValue(group.Name)
	data.ExternalID = types.StringPointerValue(group.ExternalID)

	// Vaultwarden 1.37 and later do not return accessAll: keep the known value, and assume
	// the default after an import
	if group.AccessAll != nil {
		data.AccessAll = types.BoolValue(*group.AccessAll)
	} else if data.AccessAll.IsNull() || data.AccessAll.IsUnknown() {
		data.AccessAll = types.BoolValue(false)
	}
}
