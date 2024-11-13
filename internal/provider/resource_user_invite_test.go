package provider

import (
	"fmt"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccUserInvite(t *testing.T) {
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
					resource.TestCheckResourceAttr("vaultwarden_user_invite.test", "email", email),
				),
			},
			// ImportState testing
			{
				ResourceName:      "vaultwarden_user_invite.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccExampleResourceConfig(email string) string {
	return fmt.Sprintf(`
resource "vaultwarden_user_invite" "test" {
  email = %[1]q
}
`, email)
}
