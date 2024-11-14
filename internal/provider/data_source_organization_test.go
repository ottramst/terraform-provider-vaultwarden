package provider

import (
	"fmt"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
	"testing"
)

func TestAccOrganizationDataSource(t *testing.T) {
	// Generate random data for the test
	name := gofakeit.Company()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testAccOrganizationDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.vaultwarden_organization.test", "name", name),
				),
			},
		},
	})
}

// Base configuration
func testAccOrganizationDataSourceConfig(name string) string {
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

data "vaultwarden_organization" "test" {
  id = vaultwarden_organization.test.id
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, name)
}
