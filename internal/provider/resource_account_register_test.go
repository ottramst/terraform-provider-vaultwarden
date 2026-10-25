package provider

import (
	"context"
	"fmt"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
	"regexp"
	"strings"
	"testing"
)

func TestAccAccountRegister(t *testing.T) {
	// Generate random data for the test
	name := gofakeit.Name()
	email := gofakeit.Email()
	password := gofakeit.Password(true, true, true, true, false, 12) // min 12 chars

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccAccountRegisterConfig(name, email, password),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_account_register.test", "name", name),
					resource.TestCheckResourceAttr("vaultwarden_account_register.test", "email", email),
					resource.TestCheckResourceAttr("vaultwarden_account_register.test", "password", password),
					resource.TestCheckResourceAttrSet("vaultwarden_account_register.test", "id"),
				),
			},
			// ImportState testing; the password cannot be read back
			{
				ResourceName:            "vaultwarden_account_register.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
			// Test duplicate registration fails
			{
				Config:      testAccAccountRegisterConfigDuplicate(name, email, password),
				ExpectError: regexp.MustCompile("user already exists"),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// New accounts use the default KDF settings, not ones from a prelogin, which are decoys for
// emails without an account. Uses a provider with only the admin token, so there is no
// provider user to take the settings from either.
func TestAccAccountRegisterDefaultKdf(t *testing.T) {
	email := gofakeit.Email()
	password := gofakeit.Password(true, true, true, true, false, 16)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "vaultwarden" {
  endpoint    = %[1]q
  admin_token = %[2]q
}

resource "vaultwarden_account_register" "test" {
  email    = %[3]q
  password = %[4]q
}
`, test.TestBaseURL, test.TestAdminToken, email, password),
				Check: func(*terraform.State) error {
					client, err := vaultwarden.New(test.TestBaseURL, vaultwarden.WithUserCredentials(email, password))
					if err != nil {
						return err
					}
					prelogin, err := client.PreLogin(context.Background())
					if err != nil {
						return err
					}
					want := models.DefaultKdfConfiguration()
					if prelogin.Kdf != want.KdfType || prelogin.KdfIterations != want.KdfIterations {
						return fmt.Errorf("account registered with KDF %d and %d iterations, want %d and %d",
							prelogin.Kdf, prelogin.KdfIterations, want.KdfType, want.KdfIterations)
					}
					return nil
				},
			},
		},
	})
}

// The write-only password is used to register the account and never stored. Changing it
// alone changes nothing; changing password_wo_version registers the account again.
func TestAccAccountRegisterWriteOnly(t *testing.T) {
	email := gofakeit.Email()
	password := gofakeit.Password(true, true, true, true, false, 16)
	newPassword := gofakeit.Password(true, true, true, true, false, 16)
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccAccountRegisterWriteOnlyConfig(email, password, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("vaultwarden_account_register.test", "password"),
					resource.TestCheckNoResourceAttr("vaultwarden_account_register.test", "password_wo"),
					resource.TestCheckResourceAttr("vaultwarden_account_register.test", "password_wo_version", "1"),
					testAccCheckNotInState(password),
					testAccCheckAccountLogin(email, password),
					testAccCaptureAccountID(ids, "first"),
				),
			},
			// A new password without a new version changes nothing
			{
				Config: testAccAccountRegisterWriteOnlyConfig(email, newPassword, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			// A new version registers the account again with the new password
			{
				Config: testAccAccountRegisterWriteOnlyConfig(email, newPassword, 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_account_register.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckNotInState(newPassword),
					testAccCheckAccountLogin(email, newPassword),
					testAccCaptureAccountID(ids, "second"),
					func(*terraform.State) error {
						if ids["first"] == ids["second"] {
							return fmt.Errorf("account %s was not registered again", ids["first"])
						}
						return nil
					},
				),
			},
			// The password cannot be read back, and an imported account has no version
			{
				ResourceName:            "vaultwarden_account_register.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password_wo_version"},
			},
		},
	})
}

// Moving the same password from password to password_wo, and back, keeps the account. The
// password is removed from state.
func TestAccAccountRegisterMoveToWriteOnly(t *testing.T) {
	email := gofakeit.Email()
	password := gofakeit.Password(true, true, true, true, false, 16)
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccAccountRegisterConfig("Example User", email, password),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_account_register.test", "password", password),
					testAccCaptureAccountID(ids, "first"),
				),
			},
			{
				Config: testAccAccountRegisterWriteOnlyConfig(email, password, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_account_register.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("vaultwarden_account_register.test", "password"),
					testAccCheckNotInState(password),
					testAccCheckAccountLogin(email, password),
					testAccCheckAccountID(ids, "first"),
				),
			},
			// Moving back stores the password again
			{
				Config: testAccAccountRegisterConfig("Example User", email, password),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_account_register.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_account_register.test", "password", password),
					resource.TestCheckNoResourceAttr("vaultwarden_account_register.test", "password_wo_version"),
					testAccCheckAccountID(ids, "first"),
				),
			},
		},
	})
}

// An imported account has no password in state: setting the password (or password_wo and its
// version) updates the state without registering the account again.
func TestAccAccountRegisterImportSetPassword(t *testing.T) {
	for name, writeOnly := range map[string]bool{"password": false, "password_wo": true} {
		t.Run(name, func(t *testing.T) {
			email := gofakeit.Email()
			password := gofakeit.Password(true, true, true, true, false, 16)
			config := testAccAccountRegisterConfig("Example User", email, password)
			if writeOnly {
				config = testAccAccountRegisterWriteOnlyConfig(email, password, 1)
			}
			ids := map[string]string{}

			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				TerraformVersionChecks: []tfversion.TerraformVersionCheck{
					tfversion.SkipBelow(tfversion.Version1_11_0),
				},
				Steps: []resource.TestStep{
					{
						Config: config,
						Check:  testAccCaptureAccountID(ids, "first"),
					},
					// Forget the account without deleting it
					{
						Config: testAccAccountRegisterProviderConfig() + `
removed {
  from = vaultwarden_account_register.test

  lifecycle {
    destroy = false
  }
}
`,
					},
					{
						Config: config + fmt.Sprintf(`
data "vaultwarden_user" "test" {
  email = %[1]q
}

import {
  to = vaultwarden_account_register.test
  id = data.vaultwarden_user.test.id
}
`, email),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectResourceAction("vaultwarden_account_register.test", plancheck.ResourceActionUpdate),
							},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							testAccCheckAccountID(ids, "first"),
							testAccCheckAccountLogin(email, password),
						),
					},
				},
			})
		})
	}
}

// Moving a different password to password_wo registers the account again with it.
func TestAccAccountRegisterMoveToWriteOnlyNewPassword(t *testing.T) {
	email := gofakeit.Email()
	password := gofakeit.Password(true, true, true, true, false, 16)
	newPassword := gofakeit.Password(true, true, true, true, false, 16)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccAccountRegisterConfig("Example User", email, password),
			},
			{
				Config: testAccAccountRegisterWriteOnlyConfig(email, newPassword, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_account_register.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: testAccCheckAccountLogin(email, newPassword),
			},
		},
	})
}

// Exactly one of password and password_wo must be set, and password_wo_version needs password_wo.
func TestAccAccountRegisterPasswordValidation(t *testing.T) {
	email := gofakeit.Email()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccAccountRegisterProviderConfig() + fmt.Sprintf(`
resource "vaultwarden_account_register" "test" {
  email = %[1]q
}
`, email),
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			{
				Config: testAccAccountRegisterProviderConfig() + fmt.Sprintf(`
resource "vaultwarden_account_register" "test" {
  email       = %[1]q
  password    = "a-password-123"
  password_wo = "a-password-123"
}
`, email),
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			{
				Config: testAccAccountRegisterProviderConfig() + fmt.Sprintf(`
resource "vaultwarden_account_register" "test" {
  email               = %[1]q
  password            = "a-password-123"
  password_wo_version = 1
}
`, email),
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
		},
	})
}

// State written by v0.6.0, when password was required, plans no changes.
func TestAccAccountRegisterUpgradeFromV060(t *testing.T) {
	name := gofakeit.Name()
	email := gofakeit.Email()
	password := gofakeit.Password(true, true, true, true, false, 16)

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"vaultwarden": {Source: "ottramst/vaultwarden", VersionConstraint: "0.6.0"},
				},
				Config: testAccAccountRegisterConfig(name, email, password),
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   testAccAccountRegisterConfig(name, email, password),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// testAccCheckNotInState checks that no attribute in the state holds the secret.
func testAccCheckNotInState(secret string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
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

// testAccCheckAccountLogin checks that the account can log in with the password.
func testAccCheckAccountLogin(email, password string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client, err := vaultwarden.New(test.TestBaseURL, vaultwarden.WithUserCredentials(email, password))
		if err != nil {
			return err
		}
		if _, err := client.GetProfile(context.Background()); err != nil {
			return fmt.Errorf("account %s cannot log in with the password: %w", email, err)
		}
		return nil
	}
}

// testAccCaptureAccountID stores the ID of the registered account under key.
func testAccCaptureAccountID(ids map[string]string, key string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["vaultwarden_account_register.test"]
		if !ok {
			return fmt.Errorf("vaultwarden_account_register.test not found in state")
		}
		ids[key] = rs.Primary.ID
		return nil
	}
}

// testAccCheckAccountID checks that the registered account is still the one stored under key.
func testAccCheckAccountID(ids map[string]string, key string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["vaultwarden_account_register.test"]
		if !ok {
			return fmt.Errorf("vaultwarden_account_register.test not found in state")
		}
		if rs.Primary.ID != ids[key] {
			return fmt.Errorf("account was registered again: ID %s, want %s", rs.Primary.ID, ids[key])
		}
		return nil
	}
}

func testAccAccountRegisterProviderConfig() string {
	return fmt.Sprintf(`
provider "vaultwarden" {
  endpoint        = %[1]q
  email           = %[2]q
  master_password = %[3]q
  admin_token     = %[4]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken)
}

func testAccAccountRegisterWriteOnlyConfig(email, password string, version int) string {
	return testAccAccountRegisterProviderConfig() + fmt.Sprintf(`
resource "vaultwarden_account_register" "test" {
  email               = %[1]q
  password_wo         = %[2]q
  password_wo_version = %[3]d
}
`, email, password, version)
}

// Base configuration.
func testAccAccountRegisterConfig(name, email, password string) string {
	return fmt.Sprintf(`
provider "vaultwarden" {
    endpoint        = %[1]q
    email           = %[2]q
    master_password = %[3]q
    admin_token     = %[4]q
}

resource "vaultwarden_account_register" "test" {
    name     = %[5]q
    email    = %[6]q
    password = %[7]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, name, email, password)
}

// Configuration that attempts to register a duplicate user.
func testAccAccountRegisterConfigDuplicate(name, email, password string) string {
	return fmt.Sprintf(`
provider "vaultwarden" {
    endpoint        = %[1]q
    email           = %[2]q
    master_password = %[3]q
    admin_token     = %[4]q
}

resource "vaultwarden_account_register" "test" {
    name     = %[5]q
    email    = %[6]q
    password = %[7]q
}

resource "vaultwarden_account_register" "test_duplicate" {
    name     = %[5]q
    email    = %[6]q
    password = %[7]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, name, email, password)
}
