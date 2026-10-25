package models

// AdminUserStatusInvited is the AdminUser.Status of a user who was invited and has not
// created an account yet.
const AdminUserStatusInvited = 1

// AdminUser is a user as returned by the admin API.
type AdminUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	// Status is 0 for a user with an account and AdminUserStatusInvited for an invited user.
	Status           int  `json:"_status"`
	Enabled          bool `json:"userEnabled"`
	EmailVerified    bool `json:"emailVerified"`
	TwoFactorEnabled bool `json:"twoFactorEnabled"`
	// CreationDate is an RFC 3339 timestamp in UTC.
	CreationDate string `json:"creationDate"`
	// LastActive is only returned by the user list, formatted for display in the server's
	// time zone, and null when the user has never logged in.
	LastActive *string `json:"lastActive"`
	// Organizations only lists the confirmed memberships of the user.
	Organizations []OrganizationMembership `json:"organizations"`
}
