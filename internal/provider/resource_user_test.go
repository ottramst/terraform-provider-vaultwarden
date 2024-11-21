package provider

import (
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccUser(t *testing.T) {
	// Generate a random email address for the test
	email := gofakeit.Email()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccExampleResourceConfig(email),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_user.test", "email", email),
				),
			},
			// ImportState testing
			{
				ResourceName:      "vaultwarden_user.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccExampleResourceConfig(email string) string {
	return fmt.Sprintf(`
provider "vaultwarden" {
  endpoint = %[1]q
  email = %[2]q
  master_password = %[3]q
  admin_token = %[4]q
}

resource "vaultwarden_user" "test" {
  email = %[5]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, email)
}
