package provider

import (
	"fmt"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
	"testing"
)

func TestAccOrganization(t *testing.T) {
	// Generate random data for the test
	name := gofakeit.Company()
	updatedName := gofakeit.Company()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccOrganizationConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization.test", "name", name),
					resource.TestCheckResourceAttr("vaultwarden_organization.test", "billing_email", test.TestEmail),
					resource.TestCheckResourceAttr("vaultwarden_organization.test", "collection_name", "Default Collection"),
					resource.TestCheckResourceAttrSet("vaultwarden_organization.test", "id"),
					resource.TestCheckResourceAttrSet("vaultwarden_organization.test", "last_updated"),
				),
			},
			// Update and Read testing
			{
				Config: testAccOrganizationConfigUpdated(updatedName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization.test", "name", updatedName),
					resource.TestCheckResourceAttr("vaultwarden_organization.test", "billing_email", "updated@example.com"),
					// collection_name shouldn't change on update
					resource.TestCheckResourceAttr("vaultwarden_organization.test", "collection_name", "Default Collection"),
					resource.TestCheckResourceAttrSet("vaultwarden_organization.test", "id"),
					resource.TestCheckResourceAttrSet("vaultwarden_organization.test", "last_updated"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "vaultwarden_organization.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"collection_name", // Not returned by API
					"last_updated",    // Computed field
				},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// Base configuration
func testAccOrganizationConfig(name string) string {
	return fmt.Sprintf(`
provider "vaultwarden" {
  endpoint = %[1]q
  email = %[2]q
  master_password = %[3]q
  admin_token = %[4]q
}

resource "vaultwarden_organization" "test" {
  name = %[5]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, name)
}

// Updated configuration with modified name and billing_email
func testAccOrganizationConfigUpdated(name string) string {
	return fmt.Sprintf(`
provider "vaultwarden" {
  endpoint = %[1]q
  email = %[2]q
  master_password = %[3]q
  admin_token = %[4]q
}

resource "vaultwarden_organization" "test" {
  name = %[5]q
  billing_email = "updated@example.com"
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, name)
}
