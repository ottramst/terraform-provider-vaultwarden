package models

import "fmt"

// UserOrgStatus represents the status of a user in an organization
type UserOrgStatus int64

const (
	UserOrgStatusRevoked   UserOrgStatus = -1
	UserOrgStatusInvited   UserOrgStatus = 0
	UserOrgStatusAccepted  UserOrgStatus = 1
	UserOrgStatusConfirmed UserOrgStatus = 2
)

// String returns the string representation of the user org status
func (t *UserOrgStatus) String() string {
	switch *t {
	case UserOrgStatusRevoked:
		return "Revoked"
	case UserOrgStatusInvited:
		return "Invited"
	case UserOrgStatusAccepted:
		return "Accepted"
	case UserOrgStatusConfirmed:
		return "Confirmed"
	default:
		return "Unknown"
	}
}

// FromString returns the user org status from the string representation
func (t *UserOrgStatus) FromString(s string) error {
	switch s {
	case "Revoked":
		*t = UserOrgStatusRevoked
	case "Invited":
		*t = UserOrgStatusInvited
	case "Accepted":
		*t = UserOrgStatusAccepted
	case "Confirmed":
		*t = UserOrgStatusConfirmed
	default:
		return fmt.Errorf("invalid user organization status: %s. Must be one of: Revoked, Invited, Accepted, Confirmed", s)
	}
	return nil
}

// UserOrgType represents the user organization type (access level/role)
type UserOrgType int64

const (
	UserOrgTypeOwner   UserOrgType = 0
	UserOrgTypeAdmin   UserOrgType = 1
	UserOrgTypeUser    UserOrgType = 2
	UserOrgTypeManager UserOrgType = 3
)

// String returns the string representation of the user org type
func (t *UserOrgType) String() string {
	switch *t {
	case UserOrgTypeOwner:
		return "Owner"
	case UserOrgTypeAdmin:
		return "Admin"
	case UserOrgTypeUser:
		return "User"
	case UserOrgTypeManager:
		return "Manager"
	default:
		return "Unknown"
	}
}

// FromString returns the user org type from the string representation
func (t *UserOrgType) FromString(s string) error {
	switch s {
	case "Owner":
		*t = UserOrgTypeOwner
	case "Admin":
		*t = UserOrgTypeAdmin
	case "User":
		*t = UserOrgTypeUser
	case "Manager":
		*t = UserOrgTypeManager
	default:
		return fmt.Errorf("invalid user organization type: %s. Must be one of: Owner, Admin, User, Manager", s)
	}
	return nil
}

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

// OrganizationUserDetails represents a user in an organization
type OrganizationUserDetails struct {
	ID     string        `json:"id"`
	Email  string        `json:"email"`
	Status UserOrgStatus `json:"status"`
	Type   UserOrgType   `json:"type"`
}

// OrganizationUsers represents a list of users in an organization
type OrganizationUsers struct {
	ContinuationToken string                    `json:"continuationToken"`
	Data              []OrganizationUserDetails `json:"data"`
	Object            string                    `json:"object"`
}
