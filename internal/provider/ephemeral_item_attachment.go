package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ ephemeral.EphemeralResourceWithConfigure = &ItemAttachmentEphemeralResource{}

func NewItemAttachmentEphemeralResource() ephemeral.EphemeralResource {
	return &ItemAttachmentEphemeralResource{}
}

// ItemAttachmentEphemeralResource defines the ephemeral resource implementation.
type ItemAttachmentEphemeralResource struct {
	client *vaultwarden.Client
}

// ItemAttachmentEphemeralModel describes the ephemeral resource data model.
type ItemAttachmentEphemeralModel struct {
	ID            types.String `tfsdk:"id"`
	ItemID        types.String `tfsdk:"item_id"`
	FileName      types.String `tfsdk:"file_name"`
	Size          types.Int64  `tfsdk:"size"`
	SizeName      types.String `tfsdk:"size_name"`
	Content       types.String `tfsdk:"content"`
	ContentBase64 types.String `tfsdk:"content_base64"`
}

func (e *ItemAttachmentEphemeralResource) Metadata(ctx context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_item_attachment"
}

func (e *ItemAttachmentEphemeralResource) Schema(ctx context.Context, req ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this ephemeral resource to download and decrypt an attachment of an item, from the personal vault of the provider user " +
			"or from an organization, without saving its content in the Terraform plan or state. Its values can be used in other ephemeral contexts, " +
			"such as provider configurations and write-only attributes. Requires Terraform 1.10 or later.\n\n" +
			"Requires `email` and `master_password` to be set in the provider configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "ID of the attachment",
				Required:            true,
			},
			"item_id": schema.StringAttribute{
				MarkdownDescription: "ID of the item the attachment belongs to",
				Required:            true,
			},
			"file_name": schema.StringAttribute{
				MarkdownDescription: "Name of the attachment",
				Computed:            true,
			},
			"size": schema.Int64Attribute{
				MarkdownDescription: "Size of the encrypted attachment in bytes",
				Computed:            true,
			},
			"size_name": schema.StringAttribute{
				MarkdownDescription: "Size of the encrypted attachment in a human-readable form, such as `1.2 KB`",
				Computed:            true,
			},
			"content": schema.StringAttribute{
				MarkdownDescription: "Content of the attachment as a string, `null` when it is not valid UTF-8",
				Computed:            true,
				Sensitive:           true,
			},
			"content_base64": schema.StringAttribute{
				MarkdownDescription: "Content of the attachment, base64-encoded",
				Computed:            true,
				Sensitive:           true,
			},
		},
	}
}

func (e *ItemAttachmentEphemeralResource) Configure(ctx context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*vaultwarden.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Ephemeral Resource Configure Type",
			fmt.Sprintf("Expected *vaultwarden.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	e.client = client
}

func (e *ItemAttachmentEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var data ItemAttachmentEphemeralModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	itemID, attachmentID := data.ItemID.ValueString(), data.ID.ValueString()

	attachment, err := e.client.GetAttachment(ctx, itemID, attachmentID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden item attachment",
			"Could not read attachment "+attachmentID+" of item "+itemID+": "+err.Error(),
		)
		return
	}
	content, err := e.client.GetAttachmentContent(ctx, itemID, attachmentID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden item attachment",
			"Could not download attachment "+attachmentID+" of item "+itemID+": "+err.Error(),
		)
		return
	}

	data.FileName = types.StringValue(attachment.FileName)
	data.Size = types.Int64Value(attachment.Size)
	data.SizeName = types.StringValue(attachment.SizeName)
	data.ContentBase64 = types.StringValue(base64.StdEncoding.EncodeToString(content))
	data.Content = types.StringNull()
	if utf8.Valid(content) {
		data.Content = types.StringValue(string(content))
	}

	resp.Diagnostics.Append(resp.Result.Set(ctx, &data)...)
}
