package models

// Organization represents a Vaultwarden organization
type Organization struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	BillingEmail   string  `json:"billingEmail"`
	CollectionName string  `json:"collectionName"`
	Key            string  `json:"key"`
	Keys           KeyPair `json:"keys"`
	PlanType       int64   `json:"planType"`
}
