package provider

import (
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// itemBaseModel holds the attributes shared by all item types.
type itemBaseModel struct {
	ID             types.String     `tfsdk:"id"`
	OrganizationID types.String     `tfsdk:"organization_id"`
	CollectionIDs  []string         `tfsdk:"collection_ids"`
	FolderID       types.String     `tfsdk:"folder_id"`
	Name           types.String     `tfsdk:"name"`
	Notes          types.String     `tfsdk:"notes"`
	Favorite       types.Bool       `tfsdk:"favorite"`
	Reprompt       types.Bool       `tfsdk:"reprompt"`
	Fields         []itemFieldModel `tfsdk:"fields"`
}

// itemFieldModel is a custom field of an item.
type itemFieldModel struct {
	Name     types.String `tfsdk:"name"`
	Value    types.String `tfsdk:"value"`
	Type     types.String `tfsdk:"type"`
	LinkedID types.Int64  `tfsdk:"linked_id"`
}

// ItemLoginModel describes a login item.
type ItemLoginModel struct {
	itemBaseModel
	Username types.String        `tfsdk:"username"`
	Password types.String        `tfsdk:"password"`
	TOTP     types.String        `tfsdk:"totp"`
	URIs     []itemLoginURIModel `tfsdk:"uris"`
}

// itemLoginURIModel is a URI of a login item.
type itemLoginURIModel struct {
	URI   types.String `tfsdk:"uri"`
	Match types.String `tfsdk:"match"`
}

// ItemSecureNoteModel describes a secure note item.
type ItemSecureNoteModel struct {
	itemBaseModel
}

// itemModel is implemented by the models of all item types.
type itemModel interface {
	base() *itemBaseModel
	toItem() models.Item
	update(item *models.Item) error
}

func (m *ItemLoginModel) base() *itemBaseModel { return &m.itemBaseModel }

func (m *ItemSecureNoteModel) base() *itemBaseModel { return &m.itemBaseModel }

func (m *ItemLoginModel) toItem() models.Item {
	item := m.itemBaseModel.toItem(models.ItemTypeLogin)
	item.Login = &models.ItemLogin{
		Username: m.Username.ValueString(),
		Password: m.Password.ValueString(),
		TOTP:     m.TOTP.ValueString(),
	}
	for _, uri := range m.URIs {
		item.Login.URIs = append(item.Login.URIs, models.ItemLoginURI{
			URI:   uri.URI.ValueString(),
			Match: uriMatchFromString(uri.Match),
		})
	}
	return item
}

func (m *ItemSecureNoteModel) toItem() models.Item {
	return m.itemBaseModel.toItem(models.ItemTypeSecureNote)
}

func (m *ItemLoginModel) update(item *models.Item) error {
	if item.Type != models.ItemTypeLogin {
		return fmt.Errorf("item %s is a %s, not a login", item.ID, itemTypeName(item.Type))
	}
	m.itemBaseModel.update(item)

	login := item.Login
	if login == nil {
		login = &models.ItemLogin{}
	}
	m.Username = stringFromServer(m.Username, login.Username)
	m.Password = stringFromServer(m.Password, login.Password)
	m.TOTP = stringFromServer(m.TOTP, login.TOTP)

	uris := make([]itemLoginURIModel, 0, len(login.URIs))
	for _, uri := range login.URIs {
		uris = append(uris, itemLoginURIModel{
			URI:   types.StringValue(uri.URI),
			Match: uriMatchToString(uri.Match),
		})
	}
	m.URIs = uris
	return nil
}

func (m *ItemSecureNoteModel) update(item *models.Item) error {
	if item.Type != models.ItemTypeSecureNote {
		return fmt.Errorf("item %s is a %s, not a secure note", item.ID, itemTypeName(item.Type))
	}
	m.itemBaseModel.update(item)
	return nil
}

// toItem returns the item for the shared attributes.
func (m *itemBaseModel) toItem(itemType models.ItemType) models.Item {
	item := models.Item{
		ID:             m.ID.ValueString(),
		Type:           itemType,
		OrganizationID: m.OrganizationID.ValueString(),
		CollectionIDs:  m.CollectionIDs,
		FolderID:       m.FolderID.ValueString(),
		Name:           m.Name.ValueString(),
		Notes:          m.Notes.ValueString(),
		Favorite:       m.Favorite.ValueBool(),
		Reprompt:       m.Reprompt.ValueBool(),
	}
	for _, field := range m.Fields {
		converted := models.ItemField{
			Name:  field.Name.ValueString(),
			Value: field.Value.ValueString(),
			Type:  fieldTypeFromString(field.Type.ValueString()),
		}
		if !field.LinkedID.IsNull() && !field.LinkedID.IsUnknown() {
			linkedID := int(field.LinkedID.ValueInt64())
			converted.LinkedID = &linkedID
		}
		item.Fields = append(item.Fields, converted)
	}
	return item
}

// update copies the shared attributes of an item read from the server into the model.
func (m *itemBaseModel) update(item *models.Item) {
	m.ID = types.StringValue(item.ID)
	m.OrganizationID = stringOrNull(item.OrganizationID)
	m.CollectionIDs = slices.Clone(item.CollectionIDs)
	if m.CollectionIDs == nil {
		m.CollectionIDs = []string{}
	}
	slices.Sort(m.CollectionIDs)
	m.FolderID = stringOrNull(item.FolderID)
	m.Name = types.StringValue(item.Name)
	m.Notes = stringFromServer(m.Notes, item.Notes)
	m.Favorite = types.BoolValue(item.Favorite)
	m.Reprompt = types.BoolValue(item.Reprompt)

	fields := make([]itemFieldModel, 0, len(item.Fields))
	for i, field := range item.Fields {
		var prior itemFieldModel
		if i < len(m.Fields) {
			prior = m.Fields[i]
		}
		converted := itemFieldModel{
			Name:     stringFromServer(prior.Name, field.Name),
			Value:    stringFromServer(prior.Value, field.Value),
			Type:     types.StringValue(fieldTypeToString(field.Type)),
			LinkedID: types.Int64Null(),
		}
		if field.LinkedID != nil {
			converted.LinkedID = types.Int64Value(int64(*field.LinkedID))
		}
		fields = append(fields, converted)
	}
	m.Fields = fields
}

// stringFromServer returns a value read from the server. Vaultwarden stores empty values as
// null: an empty value is null, unless it was an empty string before.
func stringFromServer(prior types.String, value string) types.String {
	if value == "" && !prior.IsNull() && !prior.IsUnknown() && prior.ValueString() == "" {
		return prior
	}
	return stringOrNull(value)
}

// itemTypeName returns a readable name of an item type.
func itemTypeName(itemType models.ItemType) string {
	switch itemType {
	case models.ItemTypeLogin:
		return "login"
	case models.ItemTypeSecureNote:
		return "secure note"
	case 3:
		return "card"
	case 4:
		return "identity"
	case 5:
		return "SSH key"
	default:
		return fmt.Sprintf("item of type %d", itemType)
	}
}

// fieldTypes maps the custom field types to their names in the schema.
var fieldTypes = map[models.FieldType]string{
	models.FieldTypeText:    "text",
	models.FieldTypeHidden:  "hidden",
	models.FieldTypeBoolean: "boolean",
	models.FieldTypeLinked:  "linked",
}

func fieldTypeToString(fieldType models.FieldType) string {
	if name, ok := fieldTypes[fieldType]; ok {
		return name
	}
	return fmt.Sprintf("%d", fieldType)
}

func fieldTypeFromString(name string) models.FieldType {
	for fieldType, fieldName := range fieldTypes {
		if fieldName == name {
			return fieldType
		}
	}
	return models.FieldTypeText
}

// uriMatchTypes maps the URI match types to their names in the schema.
var uriMatchTypes = map[models.URIMatchType]string{
	models.URIMatchDomain:            "domain",
	models.URIMatchHost:              "host",
	models.URIMatchStartsWith:        "starts_with",
	models.URIMatchExact:             "exact",
	models.URIMatchRegularExpression: "regular_expression",
	models.URIMatchNever:             "never",
}

func uriMatchToString(match *models.URIMatchType) types.String {
	if match == nil {
		return types.StringNull()
	}
	if name, ok := uriMatchTypes[*match]; ok {
		return types.StringValue(name)
	}
	return types.StringValue(fmt.Sprintf("%d", *match))
}

func uriMatchFromString(name types.String) *models.URIMatchType {
	if name.IsNull() || name.IsUnknown() {
		return nil
	}
	for match, matchName := range uriMatchTypes {
		if matchName == name.ValueString() {
			return &match
		}
	}
	return nil
}

// sortedKeys returns the names of a type map, sorted, for validators and descriptions.
func sortedKeys[K comparable](names map[K]string) []string {
	values := make([]string, 0, len(names))
	for _, name := range names {
		values = append(values, name)
	}
	slices.Sort(values)
	return values
}

// itemFieldAttrTypes are the attribute types of a custom field.
var itemFieldAttrTypes = map[string]attr.Type{
	"name":      types.StringType,
	"value":     types.StringType,
	"type":      types.StringType,
	"linked_id": types.Int64Type,
}

// itemLoginURIAttrTypes are the attribute types of a login URI.
var itemLoginURIAttrTypes = map[string]attr.Type{
	"uri":   types.StringType,
	"match": types.StringType,
}

// itemResourceDescription explains how the item resources store items.
const itemResourceDescription = "Items are encrypted client-side, like Bitwarden clients do: new items get their own item key " +
	"(cipher key), which is encrypted with the key of the provider user or of the organization. " +
	"Updates keep the parts of the item that the provider does not manage, such as the password history, passkeys and attachments. " +
	"Deleting the item deletes it permanently, without moving it to the trash. An item moved to the trash outside Terraform is created again.\n\n" +
	"The secrets of the item, such as `notes`, are saved in the Terraform state. " +
	"Requires `email` and `master_password` to be set in the provider configuration."

// itemReadDescription explains which items the data sources and ephemeral resources read.
const itemReadDescription = "Items in the trash are not found. Items created by Bitwarden clients are supported, " +
	"with or without an item key; like Bitwarden clients, URIs of an item with an item key are ignored when their checksum does not match.\n\n" +
	"Requires `email` and `master_password` to be set in the provider configuration."

// itemDescriptions are the descriptions of the item attributes.
var itemDescriptions = map[string]string{
	"id":              "ID of the item",
	"organization_id": "ID of the organization that owns the item",
	"collection_ids":  "IDs of the organization collections the item is assigned to",
	"folder_id":       "ID of the provider user's folder that the item is in",
	"name":            "The name of the item",
	"notes":           "The notes of the item",
	"favorite":        "Whether the item is a favorite of the provider user",
	"reprompt":        "Whether Bitwarden clients ask for the master password before showing the item",
	"fields":          "The custom fields of the item",
	"field_name":      "The name of the custom field",
	"field_value":     "The value of the custom field. For a `boolean` field, `true` or `false`. Linked fields have no value",
	"field_type":      "The type of the custom field: `text`, `hidden`, `boolean` or `linked`",
	"field_linked_id": "For a `linked` field, the ID of the item attribute it links to, such as `100` for the username or `101` for the password of a login",
	"username":        "The username of the login",
	"password":        "The password of the login",
	"totp":            "The TOTP secret or `otpauth://` URI of the login",
	"uris":            "The URIs of the login, for autofill",
	"uri":             "The URI",
	"match":           "How Bitwarden clients match the URI for autofill: `domain`, `host`, `starts_with`, `exact`, `regular_expression` or `never`. `null` uses the client's default",
}
