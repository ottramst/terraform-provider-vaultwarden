package provider

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ItemAttachment{}
var _ resource.ResourceWithConfigure = &ItemAttachment{}
var _ resource.ResourceWithImportState = &ItemAttachment{}
var _ resource.ResourceWithModifyPlan = &ItemAttachment{}

func ItemAttachmentResource() resource.Resource {
	return &ItemAttachment{}
}

// ItemAttachment defines the resource implementation.
type ItemAttachment struct {
	client *vaultwarden.Client
}

// ItemAttachmentModel describes the resource data model.
type ItemAttachmentModel struct {
	ID            types.String `tfsdk:"id"`
	ItemID        types.String `tfsdk:"item_id"`
	File          types.String `tfsdk:"file"`
	Content       types.String `tfsdk:"content"`
	ContentBase64 types.String `tfsdk:"content_base64"`
	FileName      types.String `tfsdk:"file_name"`
	ContentSHA256 types.String `tfsdk:"content_sha256"`
	Size          types.Int64  `tfsdk:"size"`
	SizeName      types.String `tfsdk:"size_name"`
}

func (r *ItemAttachment) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_item_attachment"
}

func (r *ItemAttachment) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	contentSources := []path.Expression{path.MatchRoot("file"), path.MatchRoot("content"), path.MatchRoot("content_base64")}

	resp.Schema = schema.Schema{
		MarkdownDescription: "This resource attaches a file to an item, in the personal vault of the provider user or in an organization. " +
			"The file is encrypted like Bitwarden clients do: with its own attachment key, which is encrypted with the key of the item.\n\n" +
			"Attachments cannot be changed: a different content or file name replaces the attachment. Changes to the content are detected " +
			"with its SHA-256 hash (`content_sha256`), so changing only the path of `file` keeps the attachment. Empty files are not supported. " +
			"Use the `vaultwarden_item_attachment` ephemeral resource to read the content of an attachment.\n\n" +
			"Vaultwarden limits the storage of attachments with `USER_ATTACHMENT_LIMIT` and `ORG_ATTACHMENT_LIMIT`, and accepts files up to 525 MB.\n\n" +
			"Requires `email` and `master_password` to be set in the provider configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "ID of the attachment",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"item_id": schema.StringAttribute{
				MarkdownDescription: "ID of the item to attach the file to. Changing it replaces the attachment",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"file": schema.StringAttribute{
				MarkdownDescription: "Path of the file to attach. Exactly one of `file`, `content` and `content_base64` must be set",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(contentSources...),
					stringvalidator.LengthAtLeast(1),
				},
			},
			"content": schema.StringAttribute{
				MarkdownDescription: "Content of the attachment, as a UTF-8 string. Requires `file_name`",
				Optional:            true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("file_name")),
					stringvalidator.LengthAtLeast(1),
				},
			},
			"content_base64": schema.StringAttribute{
				MarkdownDescription: "Content of the attachment, base64-encoded, for binary files. Requires `file_name`",
				Optional:            true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("file_name")),
					stringvalidator.LengthAtLeast(1),
				},
			},
			"file_name": schema.StringAttribute{
				MarkdownDescription: "Name of the attachment. Defaults to the name of `file`. Changing it replaces the attachment",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"content_sha256": schema.StringAttribute{
				MarkdownDescription: "Hex-encoded SHA-256 hash of the content of the attachment",
				Computed:            true,
			},
			"size": schema.Int64Attribute{
				MarkdownDescription: "Size of the encrypted attachment in bytes",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"size_name": schema.StringAttribute{
				MarkdownDescription: "Size of the encrypted attachment in a human-readable form, such as `1.2 KB`",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ItemAttachment) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ModifyPlan sets the file name and the hash of the content, and replaces the attachment when
// either changes. The content of a file is only known when it is read, so it is read on every plan.
func (r *ItemAttachment) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Destroy
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan ItemAttachmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.FileName.IsUnknown() && !plan.File.IsUnknown() && !plan.File.IsNull() {
		plan.FileName = types.StringValue(filepath.Base(plan.File.ValueString()))
	}

	plan.ContentSHA256 = types.StringUnknown()
	if content, known := attachmentContent(&plan, &resp.Diagnostics); known {
		plan.ContentSHA256 = types.StringValue(sha256Hex(content))
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if !req.State.Raw.IsNull() {
		var state ItemAttachmentModel
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
		// The hash of an imported attachment is read from the server
		if !plan.ContentSHA256.Equal(state.ContentSHA256) {
			resp.RequiresReplace = append(resp.RequiresReplace, path.Root("content_sha256"))
		}
		if !plan.FileName.Equal(state.FileName) {
			resp.RequiresReplace = append(resp.RequiresReplace, path.Root("file_name"))
		}
	}

	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// attachmentContent returns the content of an attachment from file, content or content_base64,
// and whether it is known.
func attachmentContent(data *ItemAttachmentModel, diags *diag.Diagnostics) ([]byte, bool) {
	switch {
	case data.File.IsUnknown() || data.Content.IsUnknown() || data.ContentBase64.IsUnknown():
		return nil, false
	case !data.File.IsNull():
		content, err := os.ReadFile(data.File.ValueString())
		if err != nil {
			diags.AddAttributeError(path.Root("file"), "Error reading attachment file", "Could not read the file to attach: "+err.Error())
			return nil, false
		}
		return content, true
	case !data.Content.IsNull():
		return []byte(data.Content.ValueString()), true
	case !data.ContentBase64.IsNull():
		content, err := base64.StdEncoding.DecodeString(data.ContentBase64.ValueString())
		if err != nil {
			diags.AddAttributeError(path.Root("content_base64"), "Invalid base64 content", "Could not decode content_base64: "+err.Error())
			return nil, false
		}
		return content, true
	}
	return nil, false
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func (r *ItemAttachment) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ItemAttachmentModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	content, _ := attachmentContent(&data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	hash := types.StringValue(sha256Hex(content))
	if !data.ContentSHA256.IsUnknown() && !data.ContentSHA256.Equal(hash) {
		resp.Diagnostics.AddAttributeError(path.Root("file"), "Attachment file changed",
			"The file to attach changed after the plan was made. Run the plan again.")
		return
	}
	data.ContentSHA256 = hash

	attachment, err := r.client.CreateAttachment(ctx, data.ItemID.ValueString(), data.FileName.ValueString(), content)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating Vaultwarden item attachment",
			"Could not attach the file to item "+data.ItemID.ValueString()+", unexpected error: "+err.Error(),
		)
		return
	}
	updateItemAttachmentModel(&data, attachment)

	tflog.Trace(ctx, fmt.Sprintf("created a new item attachment with ID: %s", data.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ItemAttachment) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ItemAttachmentModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	attachment, err := r.client.GetAttachment(ctx, data.ItemID.ValueString(), data.ID.ValueString())
	if err != nil {
		// Deleted outside Terraform
		if vaultwarden.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden item attachment",
			"Could not read attachment "+data.ID.ValueString()+" of item "+data.ItemID.ValueString()+", unexpected error: "+err.Error(),
		)
		return
	}
	updateItemAttachmentModel(&data, attachment)

	// An imported attachment has no hash yet: download it to compute it
	if data.ContentSHA256.IsNull() {
		content, err := r.client.GetAttachmentContent(ctx, data.ItemID.ValueString(), data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Error reading Vaultwarden item attachment",
				"Could not download attachment "+data.ID.ValueString()+" of item "+data.ItemID.ValueString()+", unexpected error: "+err.Error(),
			)
			return
		}
		data.ContentSHA256 = types.StringValue(sha256Hex(content))
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update only saves the configuration: changes to the content or the file name replace the
// attachment, so an update only changes where the same content comes from.
func (r *ItemAttachment) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ItemAttachmentModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ItemAttachment) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ItemAttachmentModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteAttachment(ctx, data.ItemID.ValueString(), data.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Vaultwarden item attachment",
			"Could not delete attachment "+data.ID.ValueString()+" of item "+data.ItemID.ValueString()+", unexpected error: "+err.Error(),
		)
		return
	}
}

func (r *ItemAttachment) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	itemID, attachmentID, ok := strings.Cut(req.ID, "/")
	if !ok || itemID == "" || attachmentID == "" || strings.Contains(attachmentID, "/") {
		resp.Diagnostics.AddError(
			"Invalid ID format",
			"Expected import identifier with format: item_id/attachment_id",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("item_id"), itemID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), attachmentID)...)
}

// updateItemAttachmentModel copies the attachment read from the server into data.
func updateItemAttachmentModel(data *ItemAttachmentModel, attachment *models.ItemAttachment) {
	data.ID = types.StringValue(attachment.ID)
	data.FileName = types.StringValue(attachment.FileName)
	data.Size = types.Int64Value(attachment.Size)
	data.SizeName = types.StringValue(attachment.SizeName)
}
