package provider

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"time"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &Organization{}
var _ resource.ResourceWithImportState = &Organization{}

func OrganizationResource() resource.Resource {
	return &Organization{}
}

// Organization defines the resource implementation.
type Organization struct {
	client *vaultwarden.Client
}

// OrganizationModel describes the resource data model.
type OrganizationModel struct {
	ID             types.String `tfsdk:"id"`
	LastUpdated    types.String `tfsdk:"last_updated"`
	Name           types.String `tfsdk:"name"`
	BillingEmail   types.String `tfsdk:"billing_email"`
	CollectionName types.String `tfsdk:"collection_name"`
}

func (r *Organization) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (r *Organization) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This resource creates a Vaultwarden organization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the organization",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"last_updated": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Timestamp of the last update",
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the organization",
				Required:            true,
			},
			"billing_email": schema.StringAttribute{
				MarkdownDescription: "The billing email of the organization. If not specified, defaults to the authenticated user's email.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"collection_name": schema.StringAttribute{
				MarkdownDescription: "The name of the collection to create for the organization. Defaults to `Default`",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("Default Collection"),
			},
		},
	}
}

func (r *Organization) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *Organization) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrganizationModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Call the client method to create the organization
	org := models.Organization{
		Name:           data.Name.ValueString(),
		BillingEmail:   data.BillingEmail.ValueString(),
		CollectionName: data.CollectionName.ValueString(),
	}

	orgResp, err := r.client.CreateOrganization(ctx, org)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating Vaultwarden organization",
			"Could not create organization, unexpected error: "+err.Error(),
		)
		return
	}

	// Map response body to schema and populate Computed attribute values
	data.ID = types.StringValue(orgResp.ID)
	data.Name = types.StringValue(orgResp.Name)
	data.BillingEmail = types.StringValue(orgResp.BillingEmail)
	data.LastUpdated = types.StringValue(time.Now().Format(time.RFC850))

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, fmt.Sprintf("created a new organization with ID: %s", data.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *Organization) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrganizationModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Get refreshed data from the client
	orgResp, err := r.client.GetOrganization(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden organization",
			"Could not read organization with ID "+data.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// Overwrite the model with the refreshed data
	data.Name = types.StringValue(orgResp.Name)
	data.BillingEmail = types.StringValue(orgResp.BillingEmail)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *Organization) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data OrganizationModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Update the organization if needed
	org := models.Organization{
		Name:         data.Name.ValueString(),
		BillingEmail: data.BillingEmail.ValueString(),
	}

	if _, err := r.client.UpdateOrganization(ctx, data.ID.ValueString(), org); err != nil {
		resp.Diagnostics.AddError(
			"Error updating Vaultwarden organization",
			"Could not update organization, unexpected error: "+err.Error(),
		)
		return
	}

	data.LastUpdated = types.StringValue(time.Now().Format(time.RFC850))

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *Organization) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OrganizationModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Delete the organization
	if err := r.client.DeleteOrganization(ctx, data.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Vaultwarden organization",
			"Could not delete organization with ID "+data.ID.ValueString()+": "+err.Error(),
		)
		return
	}
}

func (r *Organization) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
