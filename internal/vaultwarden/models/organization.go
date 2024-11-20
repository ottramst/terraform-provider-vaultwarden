package models

// Organization represents a Vaultwarden organization
type Organization struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	BillingEmail   string  `json:"billingEmail,omitempty"`
	CollectionName string  `json:"collectionName,omitempty"`
	Key            string  `json:"key"`
	Keys           KeyPair `json:"keys,omitempty"`
	PlanType       int64   `json:"planType"`
	Enabled        bool    `json:"enabled,omitempty"`
}

// OrganizationCollections represents a list of collections in an organization
type OrganizationCollections struct {
	ContinuationToken string       `json:"continuationToken"`
	Data              []Collection `json:"data"`
	Object            string       `json:"object"`
}
