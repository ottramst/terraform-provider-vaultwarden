package models

// OrganizationMembership is an organization a user is a confirmed member of, as listed in the
// user's profile and in the admin API's user details. Vaultwarden only lists confirmed memberships.
type OrganizationMembership struct {
	// ID is the ID of the organization.
	ID   string `json:"id"`
	Name string `json:"name"`
	// OrganizationUserID is the ID of the user's membership in the organization.
	OrganizationUserID string        `json:"organizationUserId"`
	Status             UserOrgStatus `json:"status"`
	// Type is reported as the custom role (4) for managers, see UserOrgType.String.
	Type UserOrgType `json:"type"`
}
