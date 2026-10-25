package models

// ItemType is the type of a vault item (cipher).
type ItemType int

const (
	ItemTypeLogin      ItemType = 1
	ItemTypeSecureNote ItemType = 2
)

// FieldType is the type of a custom field of an item.
type FieldType int

const (
	FieldTypeText    FieldType = 0
	FieldTypeHidden  FieldType = 1
	FieldTypeBoolean FieldType = 2
	FieldTypeLinked  FieldType = 3
)

// URIMatchType is how a login URI is matched for autofill. A nil match uses the client's
// default match detection.
type URIMatchType int

const (
	URIMatchDomain            URIMatchType = 0
	URIMatchHost              URIMatchType = 1
	URIMatchStartsWith        URIMatchType = 2
	URIMatchExact             URIMatchType = 3
	URIMatchRegularExpression URIMatchType = 4
	URIMatchNever             URIMatchType = 5
)

// Item is a vault item with its fields decrypted. Items are owned either by the user (personal
// vault) or by an organization, and then assigned to collections.
type Item struct {
	ID             string
	Type           ItemType
	OrganizationID string
	CollectionIDs  []string
	// FolderID and Favorite are per user, also for organization items
	FolderID string
	Favorite bool
	// Reprompt requires the master password to view the item in Bitwarden clients
	Reprompt bool
	Name     string
	Notes    string
	Fields   []ItemField
	Login    *ItemLogin
	// PasswordHistory are the previous passwords of a login, newest first
	PasswordHistory []ItemPasswordHistory
	RevisionDate    string
	// HasKey reports whether the item's fields are encrypted with an item key (cipher key)
	HasKey bool
}

// ItemField is a custom field of an item.
type ItemField struct {
	Name     string
	Value    string
	Type     FieldType
	LinkedID *int
}

// ItemLogin is the login data of a login item.
type ItemLogin struct {
	Username string
	Password string
	TOTP     string
	URIs     []ItemLoginURI
}

// ItemLoginURI is a URI of a login item.
type ItemLoginURI struct {
	URI   string
	Match *URIMatchType
}

// ItemPasswordHistory is a previous password of a login item.
type ItemPasswordHistory struct {
	Password     string
	LastUsedDate string
}
