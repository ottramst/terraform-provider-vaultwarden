package provider

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

func TestAccUserDataSource(t *testing.T) {
	orgName := gofakeit.Company()
	// A "+" in the email checks that it is escaped in the request path
	invitedEmail := fmt.Sprintf("invited+%s@example.com", strings.ToLower(gofakeit.LetterN(10)))
	accountEmail := strings.ToLower(gofakeit.Email())
	accountName := gofakeit.Name()
	password := gofakeit.Password(true, true, true, true, false, 16)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserDataSourceConfig(orgName, invitedEmail, accountEmail, accountName, password, fmt.Sprintf(`
data "vaultwarden_user" "by_id" {
  id = vaultwarden_user.invited.id
}

data "vaultwarden_user" "by_email" {
  # Emails are compared case-insensitively
  email = upper(vaultwarden_account_register.account.email)
}

data "vaultwarden_user" "by_email_invited" {
  email = vaultwarden_user.invited.email
}

data "vaultwarden_user" "owner" {
  email      = %[1]q
  depends_on = [vaultwarden_organization.test]
}

data "vaultwarden_users" "test" {
  depends_on = [vaultwarden_user.invited, vaultwarden_account_register.account, vaultwarden_organization.test]
}
`, test.TestEmail)),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Lookup by ID: an invited user
					resource.TestCheckResourceAttrPair("data.vaultwarden_user.by_id", "id", "vaultwarden_user.invited", "id"),
					resource.TestCheckResourceAttr("data.vaultwarden_user.by_id", "email", invitedEmail),
					resource.TestCheckResourceAttr("data.vaultwarden_user.by_id", "invited", "true"),
					resource.TestCheckResourceAttr("data.vaultwarden_user.by_id", "enabled", "true"),
					resource.TestCheckResourceAttr("data.vaultwarden_user.by_id", "two_factor_enabled", "false"),
					resource.TestCheckResourceAttr("data.vaultwarden_user.by_id", "organizations.#", "0"),
					// Vaultwarden names invited users after their email
					resource.TestCheckResourceAttr("data.vaultwarden_user.by_id", "name", invitedEmail),
					resource.TestMatchResourceAttr("data.vaultwarden_user.by_id", "created_at", regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$`)),
					resource.TestCheckResourceAttrPair("data.vaultwarden_user.by_email_invited", "id", "vaultwarden_user.invited", "id"),
					// Lookup by email: a registered account
					resource.TestCheckResourceAttrPair("data.vaultwarden_user.by_email", "id", "vaultwarden_account_register.account", "id"),
					resource.TestCheckResourceAttr("data.vaultwarden_user.by_email", "email", accountEmail),
					resource.TestCheckResourceAttr("data.vaultwarden_user.by_email", "name", accountName),
					resource.TestCheckResourceAttr("data.vaultwarden_user.by_email", "invited", "false"),
					resource.TestCheckResourceAttr("data.vaultwarden_user.by_email", "enabled", "true"),
					resource.TestCheckResourceAttr("data.vaultwarden_user.by_email", "email_verified", "true"),
					// The provider user owns the organization
					resource.TestCheckResourceAttr("data.vaultwarden_user.owner", "email", test.TestEmail),
					resource.TestCheckResourceAttr("data.vaultwarden_user.owner", "invited", "false"),
					resource.TestCheckTypeSetElemNestedAttrs("data.vaultwarden_user.owner", "organizations.*", map[string]string{
						"name": orgName,
						"type": "Owner",
					}),
					resource.TestCheckTypeSetElemAttrPair("data.vaultwarden_user.owner", "organizations.*.id", "vaultwarden_organization.test", "id"),
					resource.TestCheckResourceAttrSet("data.vaultwarden_user.owner", "organizations.0.organization_user_id"),
					// List
					resource.TestCheckTypeSetElemNestedAttrs("data.vaultwarden_users.test", "users.*", map[string]string{
						"email":   invitedEmail,
						"invited": "true",
						"enabled": "true",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.vaultwarden_users.test", "users.*", map[string]string{
						"email":   accountEmail,
						"name":    accountName,
						"invited": "false",
					}),
					resource.TestCheckTypeSetElemAttrPair("data.vaultwarden_users.test", "users.*.id", "vaultwarden_user.invited", "id"),
					resource.TestCheckTypeSetElemAttrPair("data.vaultwarden_users.test", "users.*.id", "vaultwarden_account_register.account", "id"),
					resource.TestCheckTypeSetElemAttrPair("data.vaultwarden_users.test", "users.*.id", "data.vaultwarden_user.owner", "id"),
					testAccCheckUsersLastActive(test.TestEmail, true),
					testAccCheckUsersLastActive(invitedEmail, false),
				),
			},
			// No user with the email
			{
				Config: testAccUserDataSourceConfig(orgName, invitedEmail, accountEmail, accountName, password, `
data "vaultwarden_user" "missing" {
  email = "missing@example.com"
}
`),
				ExpectError: regexp.MustCompile(`(?s)User Not Found.*email\s+"missing@example.com"`),
			},
			// No user with the ID
			{
				Config: testAccUserDataSourceConfig(orgName, invitedEmail, accountEmail, accountName, password, `
data "vaultwarden_user" "missing" {
  id = "00000000-0000-0000-0000-000000000000"
}
`),
				ExpectError: regexp.MustCompile(`(?s)User Not Found.*ID\s+"00000000-0000-0000-0000-000000000000"`),
			},
			// Exactly one of id and email
			{
				Config: testAccUserDataSourceConfig(orgName, invitedEmail, accountEmail, accountName, password, `
data "vaultwarden_user" "both" {
  id    = vaultwarden_user.invited.id
  email = vaultwarden_user.invited.email
}
`),
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			// A valid configuration last, for the destroy
			{
				Config: testAccUserDataSourceConfig(orgName, invitedEmail, accountEmail, accountName, password, ""),
			},
		},
	})
}

// testAccCheckUsersLastActive checks whether the user with the email in the vaultwarden_users
// data source has a last_active time.
func testAccCheckUsersLastActive(email string, active bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["data.vaultwarden_users.test"]
		if !ok {
			return fmt.Errorf("data.vaultwarden_users.test not found")
		}
		attrs := rs.Primary.Attributes
		count, err := strconv.Atoi(attrs["users.#"])
		if err != nil {
			return fmt.Errorf("invalid users.#: %w", err)
		}
		for i := 0; i < count; i++ {
			if attrs[fmt.Sprintf("users.%d.email", i)] != email {
				continue
			}
			lastActive, set := attrs[fmt.Sprintf("users.%d.last_active", i)]
			if active && (!set || lastActive == "") {
				return fmt.Errorf("user %s: expected last_active to be set", email)
			}
			if !active && set && lastActive != "" {
				return fmt.Errorf("user %s: expected no last_active, got %q", email, lastActive)
			}
			return nil
		}
		return fmt.Errorf("user %s not found in data.vaultwarden_users.test", email)
	}
}

func testAccUserDataSourceConfig(orgName, invitedEmail, accountEmail, accountName, password, dataSources string) string {
	return fmt.Sprintf(`
provider "vaultwarden" {
  endpoint        = %[1]q
  email           = %[2]q
  master_password = %[3]q
  admin_token     = %[4]q
}

resource "vaultwarden_organization" "test" {
  name = %[5]q
}

resource "vaultwarden_user" "invited" {
  email = %[6]q
}

resource "vaultwarden_account_register" "account" {
  name     = %[8]q
  email    = %[7]q
  password = %[9]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, invitedEmail, accountEmail, accountName, password) + dataSources
}
