package models

// Collection represents a collection of items
type Collection struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organizationId"`
	ExternalID     string   `json:"externalId"`
	Name           string   `json:"name"`
	Groups         []string `json:"groups"`
	Users          []string `json:"users"`
	Object         string   `json:"object"`
}
