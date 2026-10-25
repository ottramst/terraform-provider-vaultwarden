package models

// Collection represents a collection of items.
type Collection struct {
	ID             string             `json:"id"`
	OrganizationID string             `json:"organizationId"`
	ExternalID     string             `json:"externalId"`
	Name           string             `json:"name"`
	Groups         []CollectionAccess `json:"groups"`
	Users          []CollectionAccess `json:"users"`
	Object         string             `json:"object"`
}

// CollectionAccessDetails are the users and groups assigned to a collection.
type CollectionAccessDetails struct {
	Users  []CollectionAccess `json:"users"`
	Groups []CollectionAccess `json:"groups"`
}

// CollectionAccess is the access of an organization user or group to a collection.
type CollectionAccess struct {
	ID            string `json:"id"`
	ReadOnly      bool   `json:"readOnly"`
	HidePasswords bool   `json:"hidePasswords"`
	Manage        bool   `json:"manage"`
}
