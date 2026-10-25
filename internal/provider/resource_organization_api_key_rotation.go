package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &OrganizationAPIKeyRotation{}
var _ resource.ResourceWithConfigure = &OrganizationAPIKeyRotation{}

func OrganizationAPIKeyRotationResource() resource.Resource {
	return &OrganizationAPIKeyRotation{}
}

// OrganizationAPIKeyRotation rotates the API key of an organization when it is created or replaced.
type OrganizationAPIKeyRotation struct {
	client *vaultwarden.Client
}

// OrganizationAPIKeyRotationModel describes the resource data model.
type OrganizationAPIKeyRotationModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Triggers       types.Map    `tfsdk:"triggers"`
	RevisionDate   types.String `tfsdk:"revision_date"`
}

func (r *OrganizationAPIKeyRotation) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_api_key_rotation"
}

func (r *OrganizationAPIKeyRotation) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Rotates the API key of an organization: creating the resource, or replacing it when " +
			"`triggers` change, gives the organization a new client secret and invalidates the old one. " +
			"The secret is not stored: read it with the `vaultwarden_organization_api_key` ephemeral resource.\n\n" +
			"Destroying the resource leaves the current key unchanged. Rotating requires the provider user's master " +
			"password and, on Vaultwarden 1.37 and later, the `Owner` role in the organization (1.36 also allows `Admin`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "ID of the organization.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "ID of the organization. Changing it rotates the key of the new organization.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"triggers": schema.MapAttribute{
				MarkdownDescription: "Arbitrary values that rotate the key again when they change, for example " +
					"the `id` of a `time_rotating` resource.",
				ElementType: types.StringType,
				Optional:    true,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"revision_date": schema.StringAttribute{
				MarkdownDescription: "When the API key was last rotated.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *OrganizationAPIKeyRotation) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrganizationAPIKeyRotation) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrganizationAPIKeyRotationModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey, err := r.client.GetOrganizationAPIKey(ctx, data.OrganizationID.ValueString(), true)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error rotating organization API key",
			"Could not rotate the API key of organization "+data.OrganizationID.ValueString()+": "+err.Error(),
		)
		return
	}

	data.ID = data.OrganizationID
	data.RevisionDate = types.StringValue(apiKey.RevisionDate)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationAPIKeyRotation) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrganizationAPIKeyRotationModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Reading the key without rotating it shows a rotation made outside Terraform
	apiKey, err := r.client.GetOrganizationAPIKey(ctx, data.OrganizationID.ValueString(), false)
	if err != nil {
		// The organization was deleted outside Terraform
		if vaultwarden.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading organization API key",
			"Could not read the API key of organization "+data.OrganizationID.ValueString()+": "+err.Error(),
		)
		return
	}

	data.RevisionDate = types.StringValue(apiKey.RevisionDate)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update is never called: every attribute that can change forces a replacement.
func (r *OrganizationAPIKeyRotation) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data OrganizationAPIKeyRotationModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete leaves the key unchanged: rotating it would invalidate a key that may still be in use.
func (r *OrganizationAPIKeyRotation) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}
