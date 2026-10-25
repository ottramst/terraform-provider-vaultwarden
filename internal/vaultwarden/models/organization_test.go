package models

import (
	"encoding/json"
	"testing"
)

func TestOrganizationUserDetailsHasAccessAll(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantType string
		want     bool
	}{
		{"explicit accessAll", `{"type": 2, "accessAll": true}`, "User", true},
		{"explicit no accessAll", `{"type": 1, "accessAll": false}`, "Admin", false},
		{"owner without accessAll", `{"type": 0}`, "Owner", true},
		{"admin without accessAll", `{"type": 1}`, "Admin", true},
		{"user without accessAll", `{"type": 2}`, "User", false},
		{"manager as custom without permissions", `{"type": 4, "permissions": null}`, "Manager", false},
		{"manager as custom with manage all collections", `{"type": 4, "permissions": {"createNewCollections": true, "editAnyCollection": true, "deleteAnyCollection": true, "manageUsers": false}}`, "Manager", true},
		{"manager as custom with partial permissions", `{"type": 4, "permissions": {"createNewCollections": true, "editAnyCollection": true}}`, "Manager", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var user OrganizationUserDetails
			if err := json.Unmarshal([]byte(tt.json), &user); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := user.Type.String(); got != tt.wantType {
				t.Errorf("Type.String() = %q, want %q", got, tt.wantType)
			}
			if got := user.HasAccessAll(); got != tt.want {
				t.Errorf("HasAccessAll() = %t, want %t", got, tt.want)
			}
		})
	}
}
