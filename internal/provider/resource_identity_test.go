package provider

import (
	"fmt"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

// identityResources are the resources with a resource identity, with whether the identity
// includes the organization ID.
var identityResources = map[string]bool{
	"vaultwarden_organization_collection.test": true,
	"vaultwarden_organization_group.test":      true,
	"vaultwarden_organization_user.test":       true,
	"vaultwarden_user.test":                    false,
}

// testAccIdentityStateChecks checks that the identity of each resource holds the IDs in its state.
func testAccIdentityStateChecks() []statecheck.StateCheck {
	var checks []statecheck.StateCheck
	for address, orgScoped := range identityResources {
		checks = append(checks, statecheck.ExpectIdentityValueMatchesState(address, tfjsonpath.New("id")))
		if orgScoped {
			checks = append(checks, statecheck.ExpectIdentityValueMatchesState(address, tfjsonpath.New("organization_id")))
		}
	}
	return checks
}

// Resources store their identity, and can be imported with an identity in an import block.
func TestAccResourceIdentity(t *testing.T) {
	orgName := gofakeit.Company()
	memberEmail := gofakeit.Email()
	userEmail := gofakeit.Email()

	steps := []resource.TestStep{
		{
			Config:            testAccIdentityConfig(orgName, memberEmail, userEmail),
			ConfigStateChecks: testAccIdentityStateChecks(),
		},
	}
	for _, address := range []string{
		"vaultwarden_organization_collection.test",
		"vaultwarden_organization_group.test",
		"vaultwarden_organization_user.test",
		"vaultwarden_user.test",
	} {
		steps = append(steps, resource.TestStep{
			ResourceName:    address,
			ImportState:     true,
			ImportStateKind: resource.ImportBlockWithResourceIdentity,
		})
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

// State written by v0.6.0, before resources had an identity, plans no changes, and the
// identity is added on refresh.
func TestAccResourceIdentityUpgradeFromV060(t *testing.T) {
	orgName := gofakeit.Company()
	memberEmail := gofakeit.Email()
	userEmail := gofakeit.Email()
	config := testAccIdentityConfig(orgName, memberEmail, userEmail)

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"vaultwarden": {Source: "ottramst/vaultwarden", VersionConstraint: "0.6.0"},
				},
				Config: config,
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				ConfigStateChecks: testAccIdentityStateChecks(),
			},
		},
	})
}

func testAccIdentityConfig(orgName, memberEmail, userEmail string) string {
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
  name            = "Collection"
}

resource "vaultwarden_organization_group" "test" {
  organization_id = vaultwarden_organization.test.id
  name            = "Group"
}

resource "vaultwarden_organization_user" "test" {
  organization_id = vaultwarden_organization.test.id
  email           = %[6]q
}

resource "vaultwarden_user" "test" {
  email = %[7]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, memberEmail, userEmail)
}
