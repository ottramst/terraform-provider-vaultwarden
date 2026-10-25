package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSourceWithConfigure = &itemDataSource[ItemLoginModel, *ItemLoginModel]{}
var _ datasource.DataSourceWithConfigure = &itemDataSource[ItemSecureNoteModel, *ItemSecureNoteModel]{}

func NewItemLoginDataSource() datasource.DataSource {
	return &itemDataSource[ItemLoginModel, *ItemLoginModel]{
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

func NewItemSecureNoteDataSource() datasource.DataSource {
	return &itemDataSource[ItemSecureNoteModel, *ItemSecureNoteModel]{
		typeName: "item_secure_note",
		kind:     "secure note",
	}
}

// itemDataSource implements the data sources of all item types. M is the model of the item type.
type itemDataSource[M any, PM interface {
	*M
	itemModel
}] struct {
	client   *vaultwarden.Client
	typeName string
	kind     string
	// attributes are the attributes of the item type, added to the shared ones
	attributes map[string]schema.Attribute
}

func (d *itemDataSource[M, PM]) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.typeName
}

func (d *itemDataSource[M, PM]) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
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
	for name, attribute := range d.attributes {
		attributes[name] = attribute
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to read a " + d.kind + " item by ID, from the personal vault of the provider user or from an organization. " +
			"The secrets it reads are saved in the Terraform state: to use them without saving them, use the `" +
			"vaultwarden_" + d.typeName + "` ephemeral resource instead (Terraform 1.10 and later).\n\n" + itemReadDescription,
		Attributes: attributes,
	}
}

func (d *itemDataSource[M, PM]) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if client := configureLookupClient(req, resp); client != nil {
		d.client = client
	}
}

func (d *itemDataSource[M, PM]) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	data := PM(new(M))
	resp.Diagnostics.Append(req.Config.Get(ctx, data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	title, detail := readItem(ctx, d.client, d.kind, data)
	if title != "" {
		resp.Diagnostics.AddError(title, detail)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

// readItem reads the item with the ID in data into data. It returns the title and detail of
// an error, if any.
func readItem[PM itemModel](ctx context.Context, client *vaultwarden.Client, kind string, data PM) (string, string) {
	itemID := data.base().ID.ValueString()
	item, err := client.GetItem(ctx, itemID)
	if err != nil {
		if vaultwarden.IsNotFound(err) {
			return "Item Not Found", "The provider user has no access to an item with ID " + itemID + ", or it is in the trash."
		}
		return "Error Reading Item", "Could not read item " + itemID + ": " + err.Error()
	}
	if err := data.update(item); err != nil {
		return "Item Is Not a " + titleCase(kind), err.Error()
	}
	return "", ""
}

// titleCase capitalizes the words of an item kind, such as "secure note".
func titleCase(kind string) string {
	words := []byte(kind)
	for i := range words {
		if (i == 0 || words[i-1] == ' ') && words[i] >= 'a' && words[i] <= 'z' {
			words[i] -= 'a' - 'A'
		}
	}
	return string(words)
}
