package models

import "strconv"

// EventType is the type of an event in the event log of an organization.
type EventType int

// eventTypeNames are the names of the event types Vaultwarden logs, after Bitwarden's EventType
// enum, in snake case.
var eventTypeNames = map[EventType]string{
	1000: "user_logged_in",
	1001: "user_changed_password",
	1002: "user_updated_2fa",
	1003: "user_disabled_2fa",
	1004: "user_recovered_2fa",
	1005: "user_failed_log_in",
	1006: "user_failed_log_in_2fa",
	1007: "user_client_exported_vault",
	1010: "user_requested_device_approval",
	1100: "cipher_created",
	1101: "cipher_updated",
	1102: "cipher_deleted",
	1103: "cipher_attachment_created",
	1104: "cipher_attachment_deleted",
	1105: "cipher_shared",
	1106: "cipher_updated_collections",
	1107: "cipher_client_viewed",
	1108: "cipher_client_toggled_password_visible",
	1109: "cipher_client_toggled_hidden_field_visible",
	1110: "cipher_client_toggled_card_code_visible",
	1111: "cipher_client_copied_password",
	1112: "cipher_client_copied_hidden_field",
	1113: "cipher_client_copied_card_code",
	1114: "cipher_client_autofilled",
	1115: "cipher_soft_deleted",
	1116: "cipher_restored",
	1117: "cipher_client_toggled_card_number_visible",
	1300: "collection_created",
	1301: "collection_updated",
	1302: "collection_deleted",
	1400: "group_created",
	1401: "group_updated",
	1402: "group_deleted",
	1500: "organization_user_invited",
	1501: "organization_user_confirmed",
	1502: "organization_user_updated",
	1503: "organization_user_removed",
	1504: "organization_user_updated_groups",
	1505: "organization_user_unlinked_sso",
	1506: "organization_user_reset_password_enroll",
	1507: "organization_user_reset_password_withdraw",
	1508: "organization_user_admin_reset_password",
	1511: "organization_user_revoked",
	1512: "organization_user_restored",
	1513: "organization_user_approved_auth_request",
	1514: "organization_user_rejected_auth_request",
	1515: "organization_user_deleted",
	1516: "organization_user_left",
	1519: "organization_user_admin_reset_two_factor",
	1522: "organization_user_notification_banner_action_clicked",
	1600: "organization_updated",
	1601: "organization_purged_vault",
	1602: "organization_client_exported_vault",
	1700: "policy_updated",
}

// String returns the name of the event type, or its number for a type unknown to the provider.
func (t EventType) String() string {
	if name, ok := eventTypeNames[t]; ok {
		return name
	}
	return strconv.Itoa(int(t))
}

// Event is an entry of the event log of an organization. The IDs are empty when they do not
// apply to the event.
type Event struct {
	Type               EventType `json:"type"`
	Date               string    `json:"date"`
	ActingUserID       string    `json:"actingUserId"`
	UserID             string    `json:"userId"`
	OrganizationUserID string    `json:"organizationUserId"`
	CipherID           string    `json:"cipherId"`
	CollectionID       string    `json:"collectionId"`
	GroupID            string    `json:"groupId"`
	PolicyID           string    `json:"policyId"`
	DeviceType         *int      `json:"deviceType"`
	IPAddress          string    `json:"ipAddress"`
}
