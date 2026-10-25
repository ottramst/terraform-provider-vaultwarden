package models

// Group represents an organization group.
type Group struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	// AccessAll is not returned by Vaultwarden 1.37 and later.
	AccessAll  *bool   `json:"accessAll,omitempty"`
	ExternalID *string `json:"externalId"`
	Object     string  `json:"object"`
}

// GroupDetails is a group with the collections assigned to it.
type GroupDetails struct {
	Group
	Collections []CollectionAccess `json:"collections"`
}

// OrganizationGroups represents a list of groups in an organization.
type OrganizationGroups struct {
	ContinuationToken string  `json:"continuationToken"`
	Data              []Group `json:"data"`
	Object            string  `json:"object"`
}
