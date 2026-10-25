package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ ephemeral.EphemeralResourceWithConfigure = &itemEphemeralResource[ItemLoginModel, *ItemLoginModel]{}
var _ ephemeral.EphemeralResourceWithConfigure = &itemEphemeralResource[ItemSecureNoteModel, *ItemSecureNoteModel]{}

func NewItemLoginEphemeralResource() ephemeral.EphemeralResource {
	return &itemEphemeralResource[ItemLoginModel, *ItemLoginModel]{
		typeName: "item_login",
		kind:     "login",
		attributes: map[string]schema.Attribute{
			"username": schema.StringAttribute{
				MarkdownDescription: itemDescriptions["username"],
				Computed:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: itemDescriptions["password"],
				Computed:            true,
				Sensitive:           true,
			},
			"totp": schema.StringAttribute{
				MarkdownDescription: itemDescriptions["totp"],
				Computed:            true,
				Sensitive:           true,
			},
			"uris": schema.ListNestedAttribute{
				MarkdownDescription: itemDescriptions["uris"],
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"uri": schema.StringAttribute{
							MarkdownDescription: itemDescriptions["uri"],
							Computed:            true,
						},
						"match": schema.StringAttribute{
							MarkdownDescription: itemDescriptions["match"],
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func NewItemSecureNoteEphemeralResource() ephemeral.EphemeralResource {
	return &itemEphemeralResource[ItemSecureNoteModel, *ItemSecureNoteModel]{
		typeName: "item_secure_note",
		kind:     "secure note",
	}
}

// itemEphemeralResource implements the ephemeral resources of all item types. M is the model
// of the item type.
type itemEphemeralResource[M any, PM interface {
	*M
	itemModel
}] struct {
	client   *vaultwarden.Client
	typeName string
	kind     string
	// attributes are the attributes of the item type, added to the shared ones
	attributes map[string]schema.Attribute
}

func (e *itemEphemeralResource[M, PM]) Metadata(ctx context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + e.typeName
}

func (e *itemEphemeralResource[M, PM]) Schema(ctx context.Context, req ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	attributes := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: itemDescriptions["id"],
			Required:            true,
		},
		"organization_id": schema.StringAttribute{
			MarkdownDescription: itemDescriptions["organization_id"] + ", `null` for an item in the personal vault of the provider user",
			Computed:            true,
		},
		"collection_ids": schema.SetAttribute{
			MarkdownDescription: itemDescriptions["collection_ids"],
			ElementType:         types.StringType,
			Computed:            true,
		},
		"folder_id": schema.StringAttribute{
			MarkdownDescription: itemDescriptions["folder_id"],
			Computed:            true,
		},
		"name": schema.StringAttribute{
			MarkdownDescription: itemDescriptions["name"],
			Computed:            true,
		},
		"notes": schema.StringAttribute{
			MarkdownDescription: itemDescriptions["notes"],
			Computed:            true,
			Sensitive:           true,
		},
		"favorite": schema.BoolAttribute{
			MarkdownDescription: itemDescriptions["favorite"],
			Computed:            true,
		},
		"reprompt": schema.BoolAttribute{
			MarkdownDescription: itemDescriptions["reprompt"],
			Computed:            true,
		},
		"fields": schema.ListNestedAttribute{
			MarkdownDescription: itemDescriptions["fields"],
			Computed:            true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						MarkdownDescription: itemDescriptions["field_name"],
						Computed:            true,
					},
					"value": schema.StringAttribute{
						MarkdownDescription: itemDescriptions["field_value"],
						Computed:            true,
						Sensitive:           true,
					},
					"type": schema.StringAttribute{
						MarkdownDescription: itemDescriptions["field_type"],
						Computed:            true,
					},
					"linked_id": schema.Int64Attribute{
						MarkdownDescription: itemDescriptions["field_linked_id"],
						Computed:            true,
					},
				},
			},
		},
	}
	for name, attribute := range e.attributes {
		attributes[name] = attribute
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this ephemeral resource to read a " + e.kind + " item by ID, from the personal vault of the provider user or from an organization, " +
			"without saving its secrets in the Terraform plan or state. Its values can be used in other ephemeral contexts, " +
			"such as provider configurations and write-only attributes. Requires Terraform 1.10 or later.\n\n" + itemReadDescription,
		Attributes: attributes,
	}
}

func (e *itemEphemeralResource[M, PM]) Configure(ctx context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
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

func (e *itemEphemeralResource[M, PM]) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	data := PM(new(M))
	resp.Diagnostics.Append(req.Config.Get(ctx, data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	title, detail := readItem(ctx, e.client, e.kind, data)
	if title != "" {
		resp.Diagnostics.AddError(title, detail)
		return
	}

	resp.Diagnostics.Append(resp.Result.Set(ctx, data)...)
}
