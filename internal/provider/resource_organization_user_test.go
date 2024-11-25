package provider

import (
	"fmt"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
	"regexp"
	"testing"
)

func TestAccOrganizationUser(t *testing.T) {
	orgName := gofakeit.Company()
	email := gofakeit.Email()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing with default values
			{
				Config: testAccOrganizationUserConfigBasic(orgName, email),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Organization checks
					resource.TestCheckResourceAttr("vaultwarden_organization.test", "name", orgName),
					// User checks - default values
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "email", email),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "type", "User"),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "access_all", "false"),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "status", "Invited"),
					resource.TestCheckResourceAttrSet("vaultwarden_organization_user.test", "id"),
					resource.TestCheckResourceAttrSet("vaultwarden_organization_user.test", "organization_id"),
				),
			},
			// Update testing with Admin role and access_all true
			{
				Config: testAccOrganizationUserConfigCustom(orgName, email, "Admin", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "email", email),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "type", "Admin"),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "access_all", "true"),
				),
			},
			// Update to Manager role with access_all false
			{
				Config: testAccOrganizationUserConfigCustom(orgName, email, "Manager", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "email", email),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "type", "Manager"),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "access_all", "false"),
				),
			},
			// Import testing
			{
				ResourceName:      "vaultwarden_organization_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccOrganizationUserImportStateIdFunc(),
			},
		},
	})
}

func TestAccOrganizationUserInvalidType(t *testing.T) {
	orgName := gofakeit.Company()
	email := gofakeit.Email()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccOrganizationUserConfigCustom(orgName, email, "InvalidType", false),
				ExpectError: regexp.MustCompile(`Invalid Attribute Value Match`),
			},
		},
	})
}

// Basic configuration with default values
func testAccOrganizationUserConfigBasic(orgName, email string) string {
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

resource "vaultwarden_organization_user" "test" {
    organization_id = vaultwarden_organization.test.id
    email          = %[6]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, email)
}

// Configuration with custom type and access_all settings
func testAccOrganizationUserConfigCustom(orgName, email, userType string, accessAll bool) string {
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

resource "vaultwarden_organization_user" "test" {
    organization_id = vaultwarden_organization.test.id
    email          = %[6]q
    type           = %[7]q
    access_all     = %[8]t
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, email, userType, accessAll)
}

// Import state function
func testAccOrganizationUserImportStateIdFunc() resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources["vaultwarden_organization_user.test"]
		if !ok {
			return "", fmt.Errorf("resource not found in state")
		}

		return fmt.Sprintf("%s/%s",
			rs.Primary.Attributes["organization_id"],
			rs.Primary.Attributes["id"]), nil
	}
}
