package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.ResourceWithConfigure = &itemResource[ItemLoginModel, *ItemLoginModel]{}
var _ resource.ResourceWithImportState = &itemResource[ItemLoginModel, *ItemLoginModel]{}
var _ resource.ResourceWithConfigure = &itemResource[ItemSecureNoteModel, *ItemSecureNoteModel]{}
var _ resource.ResourceWithImportState = &itemResource[ItemSecureNoteModel, *ItemSecureNoteModel]{}

func ItemLoginResource() resource.Resource {
	return &itemResource[ItemLoginModel, *ItemLoginModel]{
		typeName: "item_login",
		kind:     "login",
		description: "This resource creates a login item, with a username, password, TOTP secret and URIs, " +
			"in the personal vault of the provider user or in an organization.",
		attributes: map[string]schema.Attribute{
			"username": schema.StringAttribute{
				MarkdownDescription: itemDescriptions["username"],
				Optional:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: itemDescriptions["password"] + ". When it changes, the previous password is added to the item's password history, like Bitwarden clients do",
				Optional:            true,
				Sensitive:           true,
			},
			"totp": schema.StringAttribute{
				MarkdownDescription: itemDescriptions["totp"],
				Optional:            true,
				Sensitive:           true,
			},
			"uris": schema.ListNestedAttribute{
				MarkdownDescription: itemDescriptions["uris"] + ". Defaults to none",
				Optional:            true,
				Computed:            true,
				Default:             listdefault.StaticValue(types.ListValueMust(types.ObjectType{AttrTypes: itemLoginURIAttrTypes}, []attr.Value{})),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"uri": schema.StringAttribute{
							MarkdownDescription: itemDescriptions["uri"],
							Required:            true,
							Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
						},
						"match": schema.StringAttribute{
							MarkdownDescription: itemDescriptions["match"],
							Optional:            true,
							Validators:          []validator.String{stringvalidator.OneOf(sortedKeys(uriMatchTypes)...)},
						},
					},
				},
			},
		},
	}
}

func ItemSecureNoteResource() resource.Resource {
	return &itemResource[ItemSecureNoteModel, *ItemSecureNoteModel]{
		typeName: "item_secure_note",
		kind:     "secure note",
		description: "This resource creates a secure note item in the personal vault of the provider user or in an organization. " +
			"The note's text is the `notes` attribute.",
	}
}

// itemResource implements the resources of all item types. M is the model of the item type.
type itemResource[M any, PM interface {
	*M
	itemModel
}] struct {
	client      *vaultwarden.Client
	typeName    string
	kind        string
	description string
	// attributes are the attributes of the item type, added to the shared ones
	attributes map[string]schema.Attribute
}

func (r *itemResource[M, PM]) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.typeName
}

func (r *itemResource[M, PM]) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: itemDescriptions["id"],
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"organization_id": schema.StringAttribute{
			MarkdownDescription: itemDescriptions["organization_id"] + ". Leave unset for an item in the personal vault of the provider user. " +
				"Changing it replaces the item, because Bitwarden cannot move items out of an organization",
			Optional: true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"collection_ids": schema.SetAttribute{
			MarkdownDescription: itemDescriptions["collection_ids"] + ". Requires `organization_id`. Defaults to none, " +
				"which only organization owners and admins can use; other members must assign the item to a collection they can edit",
			ElementType: types.StringType,
			Optional:    true,
			Computed:    true,
			Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
			Validators: []validator.Set{
				setvalidator.AlsoRequires(path.MatchRoot("organization_id")),
			},
		},
		"folder_id": schema.StringAttribute{
			MarkdownDescription: itemDescriptions["folder_id"] + ". Folders are personal: the item is shown in the folder only for the provider user, also for organization items",
			Optional:            true,
		},
		"name": schema.StringAttribute{
			MarkdownDescription: itemDescriptions["name"],
			Required:            true,
			Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
		},
		"notes": schema.StringAttribute{
			MarkdownDescription: itemDescriptions["notes"],
			Optional:            true,
			Sensitive:           true,
		},
		"favorite": schema.BoolAttribute{
			MarkdownDescription: itemDescriptions["favorite"] + ". Defaults to `false`",
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(false),
		},
		"reprompt": schema.BoolAttribute{
			MarkdownDescription: itemDescriptions["reprompt"] + ". Defaults to `false`",
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(false),
		},
		"fields": schema.ListNestedAttribute{
			MarkdownDescription: itemDescriptions["fields"] + ". Defaults to none",
			Optional:            true,
			Computed:            true,
			Default:             listdefault.StaticValue(types.ListValueMust(types.ObjectType{AttrTypes: itemFieldAttrTypes}, []attr.Value{})),
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						MarkdownDescription: itemDescriptions["field_name"],
						Optional:            true,
					},
					"value": schema.StringAttribute{
						MarkdownDescription: itemDescriptions["field_value"],
						Optional:            true,
						Sensitive:           true,
					},
					"type": schema.StringAttribute{
						MarkdownDescription: itemDescriptions["field_type"] + ". Defaults to `text`",
						Optional:            true,
						Computed:            true,
						Default:             stringdefault.StaticString("text"),
						Validators:          []validator.String{stringvalidator.OneOf(sortedKeys(fieldTypes)...)},
					},
					"linked_id": schema.Int64Attribute{
						MarkdownDescription: itemDescriptions["field_linked_id"],
						Optional:            true,
						Validators:          []validator.Int64{int64validator.AtLeast(0)},
					},
				},
			},
		},
	}
	for name, attribute := range r.attributes {
		attributes[name] = attribute
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: r.description + "\n\n" + itemResourceDescription,
		Attributes:          attributes,
	}
}

func (r *itemResource[M, PM]) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *itemResource[M, PM]) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	data := PM(new(M))

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	item, err := r.client.CreateItem(ctx, data.toItem())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating Vaultwarden "+r.kind+" item",
			"Could not create item, unexpected error: "+err.Error(),
		)
		return
	}
	if err := data.update(item); err != nil {
		resp.Diagnostics.AddError("Error reading Vaultwarden "+r.kind+" item", err.Error())
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created a new %s item with ID: %s", r.kind, item.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *itemResource[M, PM]) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	data := PM(new(M))

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	itemID := data.base().ID.ValueString()
	item, err := r.client.GetItem(ctx, itemID)
	if err != nil {
		// Deleted or moved to the trash outside Terraform
		if vaultwarden.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading Vaultwarden "+r.kind+" item",
			"Could not read item "+itemID+", unexpected error: "+err.Error(),
		)
		return
	}
	if err := data.update(item); err != nil {
		resp.Diagnostics.AddError("Error reading Vaultwarden "+r.kind+" item", err.Error())
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *itemResource[M, PM]) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	data := PM(new(M))

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	item, err := r.client.UpdateItem(ctx, data.toItem())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating Vaultwarden "+r.kind+" item",
			"Could not update item "+data.base().ID.ValueString()+", unexpected error: "+err.Error(),
		)
		return
	}
	if err := data.update(item); err != nil {
		resp.Diagnostics.AddError("Error reading Vaultwarden "+r.kind+" item", err.Error())
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *itemResource[M, PM]) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	data := PM(new(M))

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteItem(ctx, data.base().ID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Vaultwarden "+r.kind+" item",
			"Could not delete item "+data.base().ID.ValueString()+", unexpected error: "+err.Error(),
		)
		return
	}
}

func (r *itemResource[M, PM]) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if strings.TrimSpace(req.ID) == "" {
		resp.Diagnostics.AddError("Invalid ID format", "Expected the ID of the item")
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
