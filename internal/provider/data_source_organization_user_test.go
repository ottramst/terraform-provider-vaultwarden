package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

func TestAccOrganizationUserDataSource(t *testing.T) {
	orgName := gofakeit.Company()
	adminEmail := strings.ToLower(gofakeit.Email())
	userEmail := strings.ToLower(gofakeit.Email())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccOrganizationUserDataSourceConfig(orgName, adminEmail, userEmail, fmt.Sprintf(`
data "vaultwarden_organization_user" "by_id" {
  organization_id = vaultwarden_organization.test.id
  id              = vaultwarden_organization_user.admin.id
}

data "vaultwarden_organization_user" "by_email" {
  organization_id = vaultwarden_organization.test.id
  # Emails are compared case-insensitively
  email = upper(vaultwarden_organization_user.user.email)
}

data "vaultwarden_organization_user" "owner" {
  organization_id = vaultwarden_organization.test.id
  email           = %[1]q
  depends_on      = [vaultwarden_organization_user.admin, vaultwarden_organization_user.user]
}

data "vaultwarden_organization_users" "test" {
  organization_id = vaultwarden_organization.test.id
  depends_on      = [vaultwarden_organization_user.admin, vaultwarden_organization_user.user]
}
`, test.TestEmail)),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Lookup by ID
					resource.TestCheckResourceAttrPair("data.vaultwarden_organization_user.by_id", "id", "vaultwarden_organization_user.admin", "id"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_user.by_id", "email", adminEmail),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_user.by_id", "type", "Admin"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_user.by_id", "status", "Invited"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_user.by_id", "access_all", "true"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_user.by_id", "two_factor_enabled", "false"),
					resource.TestCheckResourceAttrSet("data.vaultwarden_organization_user.by_id", "user_id"),
					resource.TestCheckNoResourceAttr("data.vaultwarden_organization_user.by_id", "name"),
					resource.TestCheckNoResourceAttr("data.vaultwarden_organization_user.by_id", "external_id"),
					// Lookup by email
					resource.TestCheckResourceAttrPair("data.vaultwarden_organization_user.by_email", "id", "vaultwarden_organization_user.user", "id"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_user.by_email", "email", userEmail),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_user.by_email", "type", "User"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_user.by_email", "access_all", "false"),
					// The provider user owns the organization
					resource.TestCheckResourceAttr("data.vaultwarden_organization_user.owner", "type", "Owner"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_user.owner", "status", "Confirmed"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_user.owner", "access_all", "true"),
					// List
					resource.TestCheckResourceAttr("data.vaultwarden_organization_users.test", "users.#", "3"),
					resource.TestCheckTypeSetElemNestedAttrs("data.vaultwarden_organization_users.test", "users.*", map[string]string{
						"email":      adminEmail,
						"type":       "Admin",
						"status":     "Invited",
						"access_all": "true",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.vaultwarden_organization_users.test", "users.*", map[string]string{
						"email":      userEmail,
						"type":       "User",
						"status":     "Invited",
						"access_all": "false",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.vaultwarden_organization_users.test", "users.*", map[string]string{
						"email":  test.TestEmail,
						"type":   "Owner",
						"status": "Confirmed",
					}),
					resource.TestCheckTypeSetElemAttrPair("data.vaultwarden_organization_users.test", "users.*.id", "vaultwarden_organization_user.admin", "id"),
					resource.TestCheckTypeSetElemAttrPair("data.vaultwarden_organization_users.test", "users.*.id", "vaultwarden_organization_user.user", "id"),
					resource.TestCheckTypeSetElemAttrPair("data.vaultwarden_organization_users.test", "users.*.id", "data.vaultwarden_organization_user.owner", "id"),
					resource.TestCheckTypeSetElemAttrPair("data.vaultwarden_organization_users.test", "users.*.user_id", "data.vaultwarden_organization_user.owner", "user_id"),
				),
			},
			// No user with the email
			{
				Config: testAccOrganizationUserDataSourceConfig(orgName, adminEmail, userEmail, `
data "vaultwarden_organization_user" "missing" {
  organization_id = vaultwarden_organization.test.id
  email           = "missing@example.com"
}
`),
				ExpectError: regexp.MustCompile(`Organization User Not Found`),
			},
			// No user with the ID
			{
				Config: testAccOrganizationUserDataSourceConfig(orgName, adminEmail, userEmail, `
data "vaultwarden_organization_user" "missing" {
  organization_id = vaultwarden_organization.test.id
  id              = "00000000-0000-0000-0000-000000000000"
}
`),
				ExpectError: regexp.MustCompile(`(?s)has no user with ID\s+"00000000-0000-0000-0000-000000000000"`),
			},
			// Exactly one of id and email
			{
				Config: testAccOrganizationUserDataSourceConfig(orgName, adminEmail, userEmail, `
data "vaultwarden_organization_user" "both" {
  organization_id = vaultwarden_organization.test.id
  id              = vaultwarden_organization_user.admin.id
  email           = vaultwarden_organization_user.admin.email
}
`),
				ExpectError: regexp.MustCompile(`(Invalid Attribute Combination|Missing Attribute Configuration)`),
			},
			// Unknown organization
			{
				Config: testAccOrganizationUserDataSourceConfig(orgName, adminEmail, userEmail, `
data "vaultwarden_organization_users" "missing" {
  organization_id = "00000000-0000-0000-0000-000000000000"
}
`),
				ExpectError: regexp.MustCompile(`Organization Not Found`),
			},
			// A valid configuration last, for the destroy
			{
				Config: testAccOrganizationUserDataSourceConfig(orgName, adminEmail, userEmail, ""),
			},
		},
	})
}

func testAccOrganizationUserDataSourceConfig(orgName, adminEmail, userEmail, dataSources string) string {
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

resource "vaultwarden_organization_user" "admin" {
  organization_id = vaultwarden_organization.test.id
  email           = %[6]q
  type            = "Admin"
}

resource "vaultwarden_organization_user" "user" {
  organization_id = vaultwarden_organization.test.id
  email           = %[7]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, adminEmail, userEmail) +
		dataSources
}
