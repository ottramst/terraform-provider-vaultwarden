package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &OrganizationPolicy{}
var _ resource.ResourceWithConfigure = &OrganizationPolicy{}
var _ resource.ResourceWithImportState = &OrganizationPolicy{}

// organizationPolicyTypes maps the policy type names to Vaultwarden's policy type numbers.
var organizationPolicyTypes = map[string]int{
	"two_factor_authentication":      0,
	"master_password":                1,
	"password_generator":             2,
	"single_org":                     3,
	"personal_ownership":             5,
	"disable_send":                   6,
	"send_options":                   7,
	"reset_password":                 8,
	"remove_unlock_with_pin":         14,
	"restricted_item_types":          15,
	"uri_match_defaults":             16,
	"organization_user_notification": 20,
}

func organizationPolicyTypeNames() []string {
	names := make([]string, 0, len(organizationPolicyTypes))
	for name := range organizationPolicyTypes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func OrganizationPolicyResource() resource.Resource {
	return &OrganizationPolicy{}
}

// OrganizationPolicy defines the resource implementation.
type OrganizationPolicy struct {
	client *vaultwarden.Client
}

// OrganizationPolicyModel describes the resource data model.
type OrganizationPolicyModel struct {
	ID             types.String         `tfsdk:"id"`
	OrganizationID types.String         `tfsdk:"organization_id"`
	Type           types.String         `tfsdk:"type"`
	Enabled        types.Bool           `tfsdk:"enabled"`
	Data           jsontypes.Normalized `tfsdk:"data"`
}

func (r *OrganizationPolicy) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_policy"
}

func (r *OrganizationPolicy) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This resource manages a policy of an organization.\n\n" +
			"Policies cannot be deleted: destroying the resource disables the policy and clears its data. " +
			"Enabling some policies changes the organization's members: `two_factor_authentication` revokes members without two-step login, " +
			"and `single_org` revokes members that belong to another organization. Owners and Admins are exempt.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the policy, in the format `<organization_id>/<type>`",
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
			"type": schema.StringAttribute{
				MarkdownDescription: "The type of the policy. One of " + "`" + strings.Join(organizationPolicyTypeNames(), "`, `") + "`. " +
					"`personal_ownership` is the *Remove individual vault* policy, and `reset_password` the *Account recovery administration* policy. " +
					"`organization_user_notification` requires Vaultwarden 1.37 or later.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(organizationPolicyTypeNames()...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the policy is enabled",
				Required:            true,
			},
			"data": schema.StringAttribute{
				MarkdownDescription: "The settings of the policy as a JSON object, for the policy types that have settings, for example " +
					"`jsonencode({ minLength = 12, requireUpper = true })` for `master_password`. " +
					"The keys are the ones the Bitwarden clients use, such as `minComplexity`, `minLength`, `requireUpper`, `requireLower`, `requireNumbers` and `requireSpecial` for `master_password`, " +
					"`disableHideEmail` for `send_options`, and `autoEnrollEnabled` for `reset_password`.",
				Optional:   true,
				CustomType: jsontypes.NormalizedType{},
			},
		},
	}
}

func (r *OrganizationPolicy) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrganizationPolicy) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrganizationPolicyModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	r.update(ctx, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = types.StringValue(data.OrganizationID.ValueString() + "/" + data.Type.ValueString())

	tflog.Trace(ctx, fmt.Sprintf("created a new organization policy with ID: %s", data.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationPolicy) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrganizationPolicyModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	policy, err := r.client.GetOrganizationPolicy(ctx, data.OrganizationID.ValueString(), organizationPolicyTypes[data.Type.ValueString()])
	if err != nil {
		// The organization was deleted outside Terraform
		if vaultwarden.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden organization policy",
			"Could not read organization policy, unexpected error: "+err.Error(),
		)
		return
	}

	setOrganizationPolicyModel(&data, policy)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationPolicy) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data OrganizationPolicyModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	r.update(ctx, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrganizationPolicy) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OrganizationPolicyModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Policies cannot be deleted, so disable the policy and clear its data
	orgID := data.OrganizationID.ValueString()
	if _, err := r.client.UpdateOrganizationPolicy(ctx, orgID, organizationPolicyTypes[data.Type.ValueString()], false, nil); err != nil {
		if _, getErr := r.client.GetOrganization(ctx, orgID); vaultwarden.IsNotFound(getErr) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting Vaultwarden organization policy",
			"Could not disable organization policy: "+err.Error(),
		)
		return
	}
}

func (r *OrganizationPolicy) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	orgID, policyType, ok := strings.Cut(req.ID, "/")
	if !ok || orgID == "" {
		resp.Diagnostics.AddError(
			"Invalid ID format",
			"Expected import identifier with format: organization_id/type",
		)
		return
	}
	if _, ok := organizationPolicyTypes[policyType]; !ok {
		resp.Diagnostics.AddError(
			"Invalid policy type",
			"Unknown policy type "+policyType+". Expected one of: "+strings.Join(organizationPolicyTypeNames(), ", "),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("organization_id"), orgID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), policyType)...)
}

// update sets the policy in data and copies the server's answer into data.
func (r *OrganizationPolicy) update(ctx context.Context, data *OrganizationPolicyModel, diags *diag.Diagnostics) {
	var policyData json.RawMessage
	if !data.Data.IsNull() {
		policyData = json.RawMessage(data.Data.ValueString())
	}

	policy, err := r.client.UpdateOrganizationPolicy(ctx, data.OrganizationID.ValueString(), organizationPolicyTypes[data.Type.ValueString()], data.Enabled.ValueBool(), policyData)
	if err != nil {
		diags.AddError(
			"Error setting Vaultwarden organization policy",
			"Could not set organization policy: "+err.Error(),
		)
		return
	}

	setOrganizationPolicyModel(data, policy)
}

// setOrganizationPolicyModel copies a policy read from the server into data.
func setOrganizationPolicyModel(data *OrganizationPolicyModel, policy *models.Policy) {
	data.Enabled = types.BoolValue(policy.Enabled)

	// A policy without settings has null data. Vaultwarden 1.37 stores "{}" for policies set
	// without data, so treat an empty object as no data too.
	trimmed := strings.TrimSpace(string(policy.Data))
	if trimmed == "" || trimmed == "null" || (trimmed == "{}" && data.Data.IsNull()) {
		data.Data = jsontypes.NewNormalizedNull()
		return
	}
	data.Data = jsontypes.NewNormalizedValue(trimmed)
}
