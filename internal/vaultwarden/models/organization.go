package models

import "fmt"

// UserOrgStatus represents the status of a user in an organization.
type UserOrgStatus int64

const (
	UserOrgStatusRevoked   UserOrgStatus = -1
	UserOrgStatusInvited   UserOrgStatus = 0
	UserOrgStatusAccepted  UserOrgStatus = 1
	UserOrgStatusConfirmed UserOrgStatus = 2
)

// String returns the string representation of the user org status.
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

// FromString returns the user org status from the string representation.
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

// UserOrgType represents the user organization type (access level/role).
type UserOrgType int64

const (
	UserOrgTypeOwner   UserOrgType = 0
	UserOrgTypeAdmin   UserOrgType = 1
	UserOrgTypeUser    UserOrgType = 2
	UserOrgTypeManager UserOrgType = 3
	// UserOrgTypeCustom is the custom role. Vaultwarden 1.33+ reports managers with this type.
	UserOrgTypeCustom UserOrgType = 4
)

// String returns the string representation of the user org type.
func (t *UserOrgType) String() string {
	switch *t {
	case UserOrgTypeOwner:
		return "Owner"
	case UserOrgTypeAdmin:
		return "Admin"
	case UserOrgTypeUser:
		return "User"
	case UserOrgTypeManager, UserOrgTypeCustom:
		return "Manager"
	default:
		return "Unknown"
	}
}

// FromString returns the user org type from the string representation.
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

// Organization represents a Vaultwarden organization.
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

// OrganizationCollections represents a list of collections in an organization.
type OrganizationCollections struct {
	ContinuationToken string       `json:"continuationToken"`
	Data              []Collection `json:"data"`
	Object            string       `json:"object"`
}

// OrganizationUserDetails represents a user in an organization.
type OrganizationUserDetails struct {
	ID     string `json:"id"`
	UserID string `json:"userId"`
	Email  string `json:"email"`
	// Name is only returned once the user has accepted the invitation.
	Name             *string       `json:"name"`
	ExternalID       *string       `json:"externalId"`
	TwoFactorEnabled bool          `json:"twoFactorEnabled"`
	Status           UserOrgStatus `json:"status"`
	Type             UserOrgType   `json:"type"`
	// AccessAll is not returned by Vaultwarden 1.37.4 and later, see HasAccessAll.
	AccessAll   *bool           `json:"accessAll,omitempty"`
	Permissions map[string]bool `json:"permissions,omitempty"`
	// Collections and Groups are only returned when requested, and only the direct assignments.
	Collections []CollectionAccess `json:"collections,omitempty"`
	Groups      []string           `json:"groups,omitempty"`
}

// ManageAllCollectionsPermissions are the custom role permissions that Vaultwarden
// treats as access to all collections for a manager.
var ManageAllCollectionsPermissions = map[string]bool{
	"createNewCollections": true,
	"editAnyCollection":    true,
	"deleteAnyCollection":  true,
}

// HasAccessAll reports whether the user has access to all collections. Older Vaultwarden
// versions return it directly; newer versions derive it from the type and permissions.
func (u *OrganizationUserDetails) HasAccessAll() bool {
	if u.AccessAll != nil {
		return *u.AccessAll
	}

	switch u.Type {
	case UserOrgTypeOwner, UserOrgTypeAdmin:
		return true
	case UserOrgTypeManager, UserOrgTypeCustom:
		for permission := range ManageAllCollectionsPermissions {
			if !u.Permissions[permission] {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// OrganizationUsers represents a list of users in an organization.
type OrganizationUsers struct {
	ContinuationToken string                    `json:"continuationToken"`
	Data              []OrganizationUserDetails `json:"data"`
	Object            string                    `json:"object"`
}
