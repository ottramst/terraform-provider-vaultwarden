package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

func TestAccOrganizationPolicy(t *testing.T) {
	orgName := gofakeit.Company()
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// A policy with settings, and one without
			{
				Config: testAccOrganizationPolicyConfig(orgName, true, `jsonencode({ minLength = 12, requireUpper = true })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_policy.master_password", "type", "master_password"),
					resource.TestCheckResourceAttr("vaultwarden_organization_policy.master_password", "enabled", "true"),
					resource.TestCheckResourceAttr("vaultwarden_organization_policy.master_password", "data", `{"minLength":12,"requireUpper":true}`),
					resource.TestCheckResourceAttr("vaultwarden_organization_policy.disable_send", "enabled", "true"),
					resource.TestCheckNoResourceAttr("vaultwarden_organization_policy.disable_send", "data"),
					testAccCheckOrganizationPolicy(t, ids, "master_password", 1, true, `{"minLength":12,"requireUpper":true}`),
				),
			},
			// Change the settings, and disable the other policy
			{
				Config: testAccOrganizationPolicyConfig(orgName, false, `jsonencode({ minLength = 16, requireUpper = true, requireSpecial = true })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_policy.disable_send", "enabled", "false"),
					testAccCheckOrganizationPolicy(t, ids, "master_password", 1, true, `{"minLength":16,"requireSpecial":true,"requireUpper":true}`),
					testAccCheckOrganizationPolicy(t, ids, "disable_send", 6, false, ""),
				),
			},
			// Import testing
			{
				ResourceName:      "vaultwarden_organization_policy.master_password",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "vaultwarden_organization_policy.disable_send",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Removing a policy from the configuration disables it
			{
				Config: testAccOrganizationPolicyConfigOrganizationOnly(orgName),
				Check:  testAccCheckOrganizationPolicy(t, ids, "master_password", 1, false, ""),
			},
			// Organization deleted outside Terraform: the policies are recreated with it
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).DeleteOrganization(ctx, ids["organization"]))
				},
				Config: testAccOrganizationPolicyConfig(orgName, false, `jsonencode({ minLength = 16, requireUpper = true, requireSpecial = true })`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_policy.master_password", plancheck.ResourceActionCreate),
						plancheck.ExpectResourceAction("vaultwarden_organization_policy.disable_send", plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCheckOrganizationPolicy(t, ids, "master_password", 1, true, `{"minLength":16,"requireSpecial":true,"requireUpper":true}`),
			},
		},
	})
}

func TestAccOrganizationPolicyInvalidImportID(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "vaultwarden" {
  endpoint        = %[1]q
  email           = %[2]q
  master_password = %[3]q
}

resource "vaultwarden_organization_policy" "test" {
  organization_id = "org"
  type            = "single_org"
  enabled         = true
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword),
				ResourceName:  "vaultwarden_organization_policy.test",
				ImportState:   true,
				ImportStateId: "org/require_sso",
				ExpectError:   regexp.MustCompile(`Unknown policy type require_sso`),
			},
		},
	})
}

// testAccCheckOrganizationPolicy checks a policy on the server, and stores the organization ID in ids.
func testAccCheckOrganizationPolicy(t *testing.T, ids map[string]string, name string, policyType int, enabled bool, data string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ids["organization"] = s.RootModule().Resources["vaultwarden_organization.test"].Primary.ID

		ctx := context.Background()
		policy, err := testAccClient(ctx, t).GetOrganizationPolicy(ctx, ids["organization"], policyType)
		if err != nil {
			return err
		}
		if policy.Enabled != enabled {
			return fmt.Errorf("policy %s enabled = %t, want %t", name, policy.Enabled, enabled)
		}
		if data != "" && string(policy.Data) != data {
			return fmt.Errorf("policy %s data = %s, want %s", name, policy.Data, data)
		}
		return nil
	}
}

func testAccOrganizationPolicyConfig(orgName string, disableSend bool, masterPasswordData string) string {
	return fmt.Sprintf(`
provider "vaultwarden" {
  endpoint        = %[1]q
  email           = %[2]q
  master_password = %[3]q
}

resource "vaultwarden_organization" "test" {
  name = %[4]q
}

resource "vaultwarden_organization_policy" "master_password" {
  organization_id = vaultwarden_organization.test.id
  type            = "master_password"
  enabled         = true
  data            = %[5]s
}

resource "vaultwarden_organization_policy" "disable_send" {
  organization_id = vaultwarden_organization.test.id
  type            = "disable_send"
  enabled         = %[6]t
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, orgName, masterPasswordData, disableSend)
}

func testAccOrganizationPolicyConfigOrganizationOnly(orgName string) string {
	return fmt.Sprintf(`
provider "vaultwarden" {
  endpoint        = %[1]q
  email           = %[2]q
  master_password = %[3]q
}

resource "vaultwarden_organization" "test" {
  name = %[4]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, orgName)
}
