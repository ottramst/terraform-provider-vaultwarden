package provider

import (
	"fmt"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
			// Test duplicate registration fails
			{
				Config:      testAccAccountRegisterConfigDuplicate(name, email, password),
				ExpectError: regexp.MustCompile("user already exists"),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// Base configuration
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

// Configuration that attempts to register a duplicate user
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
