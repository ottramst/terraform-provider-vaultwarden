package provider

import (
	"context"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Test for https://github.com/ottramst/terraform-provider-vaultwarden/issues/56
func TestAccUserEnabled(t *testing.T) {
	email := gofakeit.Email()
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Invite a disabled user
			{
				Config: testAccUserEnabledConfig(email, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_user.test", "enabled", "false"),
					testAccCaptureIDs(ids),
					testAccCheckUserEnabled(t, ids, false),
				),
			},
			// Enable them in place
			{
				Config: testAccUserEnabledConfig(email, true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_user.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_user.test", "enabled", "true"),
					testAccCheckUserEnabled(t, ids, true),
				),
			},
			// Disable them in place
			{
				Config: testAccUserEnabledConfig(email, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_user.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_user.test", "enabled", "false"),
					testAccCheckUserEnabled(t, ids, false),
				),
			},
			{
				ResourceName:      "vaultwarden_user.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Removing the attribute enables them, the default
			{
				Config: testAccExampleResourceConfig(email),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_user.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_user.test", "enabled", "true"),
					testAccCheckUserEnabled(t, ids, true),
				),
			},
			// A user disabled outside Terraform is enabled again
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).SetUserEnabled(ctx, ids["user"], false))
				},
				Config: testAccExampleResourceConfig(email),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_user.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: testAccCheckUserEnabled(t, ids, true),
			},
		},
	})
}

// State written by v0.5.1, before enabled existed, plans no changes.
func TestAccUserUpgradeFromV051(t *testing.T) {
	email := gofakeit.Email()

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"vaultwarden": {Source: "ottramst/vaultwarden", VersionConstraint: "0.5.1"},
				},
				Config: testAccExampleResourceConfig(email),
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   testAccExampleResourceConfig(email),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// testAccCheckUserEnabled checks on the server whether the user is enabled.
func testAccCheckUserEnabled(t *testing.T, ids map[string]string, enabled bool) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		user, err := testAccClient(ctx, t).GetUser(ctx, ids["user"])
		if err != nil {
			return err
		}
		if user.UserEnabled == nil || *user.UserEnabled != enabled {
			return fmt.Errorf("expected user %s to have enabled %t, got %v", ids["user"], enabled, user.UserEnabled)
		}
		return nil
	}
}

func testAccUserEnabledConfig(email string, enabled bool) string {
	return fmt.Sprintf(`
provider "vaultwarden" {
  endpoint        = %[1]q
  email           = %[2]q
  master_password = %[3]q
  admin_token     = %[4]q
}

resource "vaultwarden_user" "test" {
  email   = %[5]q
  enabled = %[6]t
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, email, enabled)
}

func TestAccUser(t *testing.T) {
	// Generate a random email address for the test
	email := gofakeit.Email()
	newEmail := gofakeit.Email()

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
			// Changing the email replaces the user
			{
				Config: testAccExampleResourceConfig(newEmail),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_user.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("vaultwarden_user.test", "email", newEmail),
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
