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
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/encryptedstring"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"strings"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &OrganizationCollection{}
var _ resource.ResourceWithConfigure = &OrganizationCollection{}
var _ resource.ResourceWithImportState = &OrganizationCollection{}

func OrganizationCollectionResource() resource.Resource {
	return &OrganizationCollection{}
}

// OrganizationCollection defines the resource implementation.
type OrganizationCollection struct {
	client *vaultwarden.Client
}

// OrganizationCollectionModel describes the resource data model.
type OrganizationCollectionModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	ExternalID     types.String `tfsdk:"external_id"`
	Name           types.String `tfsdk:"name"`
	// TODO: Add groups
	// TODO: Add users
}

func (r *OrganizationCollection) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_collection"
}

func (r *OrganizationCollection) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This resource creates a Vaultwarden organization collection.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the organization collection",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				MarkdownDescription: "ID of the organization that the collection belongs to",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"external_id": schema.StringAttribute{
				MarkdownDescription: "An optional identifier that can be assigned to the collection for integration with external systems. This identifier is not generated by Vaultwarden and must be provided explicitly. It is typically used to link the collection to external systems, such as directory services (e.g., LDAP, Active Directory) or custom automation workflows.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the organization collection",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *OrganizationCollection) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrganizationCollection) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrganizationCollectionModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Call the client method to create the organization
	collection := models.Collection{
		Name: data.Name.ValueString(),
	}

	// Set external_id if it's not null in the plan
	if !data.ExternalID.IsNull() {
		collection.ExternalID = data.ExternalID.ValueString()
	}

	collResp, err := r.client.CreateOrganizationCollection(ctx, data.OrganizationID.ValueString(), collection)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating Vaultwarden organization collection",
			"Could not create organization collection, unexpected error: "+err.Error(),
		)
		return
	}

	// Map response body to schema and populate Computed attribute values
	data.ID = types.StringValue(collResp.ID)

	// If we're trying to set an external_id, but the API returns empty or null,
	// keep our desired value from the configuration
	// See: https://github.com/dani-garcia/vaultwarden/pull/3690
	if collResp.ExternalID == "" && !data.ExternalID.IsNull() {
		// Keep the existing external_id from our state
	} else if collResp.ExternalID == "" {
		data.ExternalID = types.StringNull()
	} else {
		data.ExternalID = types.StringValue(collResp.ExternalID)
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, fmt.Sprintf("created a new organization with ID: %s", data.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationCollection) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrganizationCollectionModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Get refreshed data from the client
	collResp, err := r.client.GetOrganizationCollection(ctx, data.OrganizationID.ValueString(), data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden organization collection",
			"Could not read organization collection, unexpected error: "+err.Error(),
		)
		return
	}

	// Get organization data from cache
	orgSecret, exists := r.client.AuthState.Organizations[data.OrganizationID.ValueString()]
	if !exists {
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden organization collection",
			"Could not read organization collection, organization not found or not authenticated",
		)
		return
	}

	// Convert the collection name to an EncryptedString
	encryptedName, err := encryptedstring.NewFromEncryptedValue(collResp.Name)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden organization collection",
			"Could not read organization collection, failed to parse encrypted collection name: "+err.Error(),
		)
		return
	}

	// Decrypt the collection name
	decryptedBytes, err := crypt.Decrypt(encryptedName, &orgSecret.Key)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error decrypting collection name",
			err.Error(),
		)
		return
	}

	// Overwrite the model with the refreshed data
	data.Name = types.StringValue(string(decryptedBytes))

	// If we're trying to set an external_id, but the API returns empty or null,
	// keep our desired value from the configuration
	// See: https://github.com/dani-garcia/vaultwarden/pull/3690
	if collResp.ExternalID == "" && !data.ExternalID.IsNull() {
		// Keep the existing external_id from our state
	} else if collResp.ExternalID == "" {
		data.ExternalID = types.StringNull()
	} else {
		data.ExternalID = types.StringValue(collResp.ExternalID)
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationCollection) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data OrganizationCollectionModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Update the organization collection if needed
	collection := models.Collection{
		Name:       data.Name.ValueString(),
		ExternalID: data.ExternalID.ValueString(),
	}

	if _, err := r.client.UpdateOrganizationCollection(ctx, data.OrganizationID.ValueString(), data.ID.ValueString(), collection); err != nil {
		resp.Diagnostics.AddError(
			"Error updating Vaultwarden organization collection",
			"Could not update organization collection, unexpected error: "+err.Error(),
		)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationCollection) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OrganizationCollectionModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Delete the organization collection
	if err := r.client.DeleteOrganizationCollection(ctx, data.OrganizationID.ValueString(), data.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Vaultwarden organization collection",
			"Could not delete organization collection, unexpected error: "+err.Error(),
		)
		return
	}
}

func (r *OrganizationCollection) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, "/")
	if len(idParts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid ID format",
			"Expected import identifier with format: organization_id/collection_id",
		)
		return
	}

	organizationID := idParts[0]
	collectionID := idParts[1]

	// Set the organization_id and id attributes
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("organization_id"), organizationID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), collectionID)...)

	// After setting the IDs, fetch the current state of the resource
	collection, err := r.client.GetOrganizationCollection(ctx, idParts[0], idParts[1])
	if err != nil {
		resp.Diagnostics.AddError(
			"Error importing organization collection",
			fmt.Sprintf("Cannot read organization collection %s: %v", req.ID, err),
		)
		return
	}

	// Get organization data from cache
	orgSecret, exists := r.client.AuthState.Organizations[idParts[0]]
	if !exists {
		resp.Diagnostics.AddError(
			"Error importing organization collection",
			"Could not read organization collection, organization not found or not authenticated",
		)
		return
	}

	// Convert and decrypt the name
	encryptedName, err := encryptedstring.NewFromEncryptedValue(collection.Name)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error importing organization collection",
			fmt.Sprintf("Failed to parse encrypted name: %v", err),
		)
		return
	}

	decryptedBytes, err := crypt.Decrypt(encryptedName, &orgSecret.Key)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error importing organization collection",
			fmt.Sprintf("Failed to decrypt name: %v", err),
		)
		return
	}

	// Set the name
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), string(decryptedBytes))...)

	// Set external_id if it exists
	if collection.ExternalID != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("external_id"), collection.ExternalID)...)
	}
}
