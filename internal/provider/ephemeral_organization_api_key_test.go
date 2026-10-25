package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/echoprovider"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

// testAccEchoProviderFactories adds the echo provider, which copies ephemeral values into state
// so that tests can check them.
var testAccEchoProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"vaultwarden": providerserver.NewProtocol6WithError(New("test")()),
	"echo":        echoprovider.NewProviderServer(),
}

// The ephemeral resource returns the organization's API key, which logs in to the public API.
// Used elsewhere, the key does not end up in state.
func TestAccOrganizationAPIKeyEphemeral(t *testing.T) {
	orgName := gofakeit.Company()
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccEchoProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccOrganizationAPIKeyConfig(orgName) + `
provider "echo" {
  data = ephemeral.vaultwarden_organization_api_key.test
}

resource "echo" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureIDs(ids),
					resource.TestMatchResourceAttr("echo.test", "data.revision_date", regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T`)),
					func(s *terraform.State) error {
						attributes := s.RootModule().Resources["echo.test"].Primary.Attributes
						if want := "organization." + ids["organization"]; attributes["data.client_id"] != want {
							return fmt.Errorf("client_id is %q, want %q", attributes["data.client_id"], want)
						}
						ctx := context.Background()
						apiKey, err := testAccClient(ctx, t).GetOrganizationAPIKey(ctx, ids["organization"], false)
						if err != nil {
							return err
						}
						if attributes["data.client_secret"] != apiKey.ClientSecret {
							return fmt.Errorf("client_secret is not the organization's API key")
						}
						return testAccOrganizationAPIKeyLogin(attributes["data.client_id"], attributes["data.client_secret"])
					},
				),
			},
			// Without the echo provider, the key is not stored anywhere in state
			{
				Config: testAccOrganizationAPIKeyConfig(orgName) + `
data "vaultwarden_organization" "test" {
  id = vaultwarden_organization.test.id

  lifecycle {
    postcondition {
      condition     = ephemeral.vaultwarden_organization_api_key.test.client_id == "organization.${self.id}" && length(ephemeral.vaultwarden_organization_api_key.test.client_secret) > 0
      error_message = "The ephemeral resource did not return the organization's API key."
    }
  }
}
`,
				Check: testAccCheckOrganizationAPIKeyNotInState(t, ids),
			},
		},
	})
}

// An organization that does not exist fails with a clear error.
func TestAccOrganizationAPIKeyEphemeralNotFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig() + `
ephemeral "vaultwarden_organization_api_key" "test" {
  organization_id = "00000000-0000-0000-0000-000000000000"
}

data "vaultwarden_organizations" "test" {
  lifecycle {
    postcondition {
      condition     = length(ephemeral.vaultwarden_organization_api_key.test.client_secret) > 0
      error_message = "The ephemeral resource did not return an API key."
    }
  }
}
`,
				ExpectError: regexp.MustCompile(`(?s)Error reading organization API key.*not found`),
			},
		},
	})
}

// testAccCheckOrganizationAPIKeyNotInState checks that the organization's current API key is not
// stored in state.
func testAccCheckOrganizationAPIKeyNotInState(t *testing.T, ids map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		apiKey, err := testAccClient(ctx, t).GetOrganizationAPIKey(ctx, ids["organization"], false)
		if err != nil {
			return err
		}
		return testAccCheckSecretNotInState(apiKey.ClientSecret)(s)
	}
}

// testAccCheckSecretNotInState checks that no attribute in the state holds the secret.
func testAccCheckSecretNotInState(secret string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if secret == "" {
			return fmt.Errorf("empty secret")
		}
		for _, module := range s.Modules {
			for name, rs := range module.Resources {
				for key, value := range rs.Primary.Attributes {
					if strings.Contains(value, secret) {
						return fmt.Errorf("%s.%s holds the secret in state", name, key)
					}
				}
			}
		}
		return nil
	}
}

// testAccOrganizationAPIKeyLogin logs in to the public API with an organization API key.
func testAccOrganizationAPIKeyLogin(clientID, clientSecret string) error {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"scope":         {"api.organization"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		// Vaultwarden 1.36 requires device details for every client_credentials login
		"deviceType":       {"21"},
		"deviceIdentifier": {"00000000-0000-0000-0000-000000000001"},
		"deviceName":       {"terraform-provider-vaultwarden-test"},
	}
	resp, err := http.PostForm(test.TestBaseURL+"/identity/connect/token", form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("organization API key login failed with status %d", resp.StatusCode)
	}
	return nil
}

func testAccProviderConfig() string {
	return fmt.Sprintf(`
provider "vaultwarden" {
  endpoint        = %[1]q
  email           = %[2]q
  master_password = %[3]q
  admin_token     = %[4]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken)
}

func testAccOrganizationAPIKeyConfig(orgName string) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "vaultwarden_organization" "test" {
  name = %[1]q
}

ephemeral "vaultwarden_organization_api_key" "test" {
  organization_id = vaultwarden_organization.test.id
}
`, orgName)
}
