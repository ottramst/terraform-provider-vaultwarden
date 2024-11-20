package provider

import (
	"fmt"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
	"testing"
)

func TestAccOrganizationCollection(t *testing.T) {
	// Generate random data for the test
	orgName := gofakeit.Company()
	collectionName := gofakeit.ProductName()
	updatedCollectionName := gofakeit.ProductName()
	externalID := gofakeit.UUID()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccOrganizationCollectionConfig(orgName, collectionName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_collection.test", "name", collectionName),
					resource.TestCheckResourceAttrSet("vaultwarden_organization_collection.test", "id"),
					resource.TestCheckResourceAttrSet("vaultwarden_organization_collection.test", "organization_id"),
					// external_id should be null/empty initially
					resource.TestCheckNoResourceAttr("vaultwarden_organization_collection.test", "external_id"),
				),
			},
			// Update and Read testing
			{
				Config: testAccOrganizationCollectionConfigUpdated(orgName, updatedCollectionName, externalID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_collection.test", "name", updatedCollectionName),
					resource.TestCheckResourceAttr("vaultwarden_organization_collection.test", "external_id", externalID),
					resource.TestCheckResourceAttrSet("vaultwarden_organization_collection.test", "id"),
					resource.TestCheckResourceAttrSet("vaultwarden_organization_collection.test", "organization_id"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "vaultwarden_organization_collection.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["vaultwarden_organization_collection.test"]
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
func testAccOrganizationCollectionConfig(orgName, collectionName string) string {
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

resource "vaultwarden_organization_collection" "test" {
    organization_id = vaultwarden_organization.test.id
    name           = %[6]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, collectionName)
}

// Updated configuration with modified name and external_id
func testAccOrganizationCollectionConfigUpdated(orgName, collectionName, externalID string) string {
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

resource "vaultwarden_organization_collection" "test" {
    organization_id = vaultwarden_organization.test.id
    name           = %[6]q
    external_id    = %[7]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, collectionName, externalID)
}
