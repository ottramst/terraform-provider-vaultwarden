package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

// The ephemeral resource returns the provider user's personal API key, which a second provider
// uses to log in. The key does not end up in state.
func TestAccUserAPIKeyEphemeral(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccEchoProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccUserAPIKeyConfig() + `
provider "echo" {
  data = ephemeral.vaultwarden_user_api_key.test
}

resource "echo" "test" {}
`,
				Check: func(s *terraform.State) error {
					attributes := s.RootModule().Resources["echo.test"].Primary.Attributes
					ctx := context.Background()
					client := testAccClient(ctx, t)
					profile, err := client.GetProfile(ctx)
					if err != nil {
						return err
					}
					if want := "user." + profile.ID; attributes["data.client_id"] != want {
						return fmt.Errorf("client_id is %q, want %q", attributes["data.client_id"], want)
					}
					apiKey, err := client.GetUserAPIKey(ctx)
					if err != nil {
						return err
					}
					if attributes["data.client_secret"] != apiKey.ClientSecret {
						return fmt.Errorf("client_secret is not the user's API key")
					}
					return nil
				},
			},
			{
				Config: testAccUserAPIKeyConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.vaultwarden_organizations.api_key", "organizations.#"),
					func(s *terraform.State) error {
						ctx := context.Background()
						apiKey, err := testAccClient(ctx, t).GetUserAPIKey(ctx)
						if err != nil {
							return err
						}
						return testAccCheckSecretNotInState(apiKey.ClientSecret)(s)
					},
				),
			},
		},
	})
}

// testAccUserAPIKeyConfig reads the user's API key and logs in with it in a second provider.
func testAccUserAPIKeyConfig() string {
	return testAccProviderConfig() + fmt.Sprintf(`
ephemeral "vaultwarden_user_api_key" "test" {}

provider "vaultwarden" {
  alias           = "api_key"
  endpoint        = %[1]q
  email           = %[2]q
  master_password = %[3]q
  client_id       = ephemeral.vaultwarden_user_api_key.test.client_id
  client_secret   = ephemeral.vaultwarden_user_api_key.test.client_secret
}

data "vaultwarden_organizations" "api_key" {
  provider = vaultwarden.api_key
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword)
}
