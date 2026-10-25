package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// Creating the resource and changing triggers rotate the key; the ephemeral resource that
// depends on the rotation returns the new key. A rotation outside Terraform plans no changes,
// and an organization deleted outside Terraform plans a new rotation.
func TestAccOrganizationAPIKeyRotation(t *testing.T) {
	orgName := gofakeit.Company()
	ids := map[string]string{}
	secrets := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccEchoProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		Steps: []resource.TestStep{
			// The organization has a key before the rotation
			{
				Config: testAccProviderConfig() + fmt.Sprintf(`
resource "vaultwarden_organization" "test" {
  name = %[1]q
}
`, orgName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureIDs(ids),
					testAccCaptureOrganizationAPIKey(t, ids, secrets, "initial"),
				),
			},
			{
				Config: testAccOrganizationAPIKeyRotationConfig(orgName, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("vaultwarden_organization_api_key_rotation.test", "id", "vaultwarden_organization.test", "id"),
					resource.TestCheckResourceAttrSet("vaultwarden_organization_api_key_rotation.test", "revision_date"),
					testAccCaptureOrganizationAPIKey(t, ids, secrets, "first"),
					testAccCheckOrganizationAPIKeyRotated(secrets, "initial", "first"),
					testAccCheckEchoedOrganizationAPIKey("echo.v1", secrets, "first"),
					testAccCheckRotationRevisionDate(t, ids),
				),
			},
			{
				Config: testAccOrganizationAPIKeyRotationConfig(orgName, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_api_key_rotation.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureOrganizationAPIKey(t, ids, secrets, "second"),
					testAccCheckOrganizationAPIKeyRotated(secrets, "first", "second"),
					testAccCheckEchoedOrganizationAPIKey("echo.v2", secrets, "second"),
					testAccCheckRotationRevisionDate(t, ids),
					func(*terraform.State) error {
						if testAccOrganizationAPIKeyLogin("organization."+ids["organization"], secrets["first"]) == nil {
							return fmt.Errorf("the rotated key still logs in")
						}
						return testAccOrganizationAPIKeyLogin("organization."+ids["organization"], secrets["second"])
					},
				),
			},
			// A rotation outside Terraform only refreshes the revision date
			{
				PreConfig: func() {
					ctx := context.Background()
					_, err := testAccClient(ctx, t).GetOrganizationAPIKey(ctx, ids["organization"], true)
					testAccNoError(t, err)
				},
				Config: testAccOrganizationAPIKeyRotationConfig(orgName, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: testAccCheckRotationRevisionDate(t, ids),
			},
			// The organization is deleted outside Terraform
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).DeleteOrganization(ctx, ids["organization"]))
				},
				Config: testAccOrganizationAPIKeyRotationConfig(orgName, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_api_key_rotation.test", plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

// testAccCaptureOrganizationAPIKey stores the organization's current API key under key.
func testAccCaptureOrganizationAPIKey(t *testing.T, ids, secrets map[string]string, key string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		apiKey, err := testAccClient(ctx, t).GetOrganizationAPIKey(ctx, ids["organization"], false)
		if err != nil {
			return err
		}
		secrets[key] = apiKey.ClientSecret
		return nil
	}
}

// testAccCheckOrganizationAPIKeyRotated checks that the key changed between two captures.
func testAccCheckOrganizationAPIKeyRotated(secrets map[string]string, before, after string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if secrets[before] == secrets[after] {
			return fmt.Errorf("the API key was not rotated between %s and %s", before, after)
		}
		return nil
	}
}

// testAccCheckEchoedOrganizationAPIKey checks that the ephemeral resource returned the key.
func testAccCheckEchoedOrganizationAPIKey(address string, secrets map[string]string, key string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("%s not found in state", address)
		}
		if rs.Primary.Attributes["data.client_secret"] != secrets[key] {
			return fmt.Errorf("%s does not hold the %s API key", address, key)
		}
		return nil
	}
}

// testAccCheckRotationRevisionDate checks that the state holds the key's current revision date.
func testAccCheckRotationRevisionDate(t *testing.T, ids map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := context.Background()
		apiKey, err := testAccClient(ctx, t).GetOrganizationAPIKey(ctx, ids["organization"], false)
		if err != nil {
			return err
		}
		return resource.TestCheckResourceAttr("vaultwarden_organization_api_key_rotation.test", "revision_date", apiKey.RevisionDate)(s)
	}
}

// testAccOrganizationAPIKeyRotationConfig rotates the key for each version and echoes the
// ephemeral key into a new echo resource, which keeps the value of its first apply.
func testAccOrganizationAPIKeyRotationConfig(orgName string, version int) string {
	return testAccProviderConfig() + fmt.Sprintf(`
resource "vaultwarden_organization" "test" {
  name = %[1]q
}

resource "vaultwarden_organization_api_key_rotation" "test" {
  organization_id = vaultwarden_organization.test.id

  triggers = {
    version = %[2]d
  }
}

ephemeral "vaultwarden_organization_api_key" "test" {
  organization_id = vaultwarden_organization_api_key_rotation.test.organization_id

  depends_on = [vaultwarden_organization_api_key_rotation.test]
}

provider "echo" {
  data = ephemeral.vaultwarden_organization_api_key.test
}

resource "echo" "v%[2]d" {}
`, orgName, version)
}
