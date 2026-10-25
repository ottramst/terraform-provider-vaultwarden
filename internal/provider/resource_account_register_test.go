package provider

import (
	"context"
	"fmt"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
	"regexp"
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
