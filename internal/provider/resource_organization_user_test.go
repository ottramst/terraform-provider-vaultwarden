package provider

import (
	"fmt"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
	"testing"
)

func TestAccOrganizationUser(t *testing.T) {
	orgName := gofakeit.Company()
	email := gofakeit.Email()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccOrganizationUserConfig(orgName, email),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Organization checks
					resource.TestCheckResourceAttr("vaultwarden_organization.test", "name", orgName),
					// User checks
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "email", email),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "type", "User"), // Default type
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "status", "Invited"),
					resource.TestCheckResourceAttrSet("vaultwarden_organization_user.test", "id"),
					resource.TestCheckResourceAttrSet("vaultwarden_organization_user.test", "organization_id"),
				),
			},
			// Import testing
			{
				ResourceName:      "vaultwarden_organization_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["vaultwarden_organization_user.test"]
					if !ok {
						return "", fmt.Errorf("resource not found in state")
					}

					return fmt.Sprintf("%s/%s",
						rs.Primary.Attributes["organization_id"],
						rs.Primary.Attributes["id"]), nil
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// Base configuration
func testAccOrganizationUserConfig(orgName, email string) string {
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
	email           = %[6]q
	type            = "User"
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, email)
}
