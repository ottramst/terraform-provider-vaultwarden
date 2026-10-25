package models

import "encoding/json"

// Policy represents an organization policy.
type Policy struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organizationId"`
	Type           int             `json:"type"`
	Data           json.RawMessage `json:"data"`
	Enabled        bool            `json:"enabled"`
}
