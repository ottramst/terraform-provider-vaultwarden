package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/echoprovider"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

const testAccItemsConfig = `
resource "vaultwarden_folder" "test" {
  name = "Data source items"
}

resource "vaultwarden_item_login" "test" {
  folder_id = vaultwarden_folder.test.id
  name      = "Login"
  notes     = "Login notes"
  username  = "user"
  password  = "secret password"
  totp      = "otpauth://totp/Example?secret=JBSWY3DPEHPK3PXP"
  uris      = [{ uri = "https://example.com", match = "host" }]
  fields    = [{ name = "PIN", value = "1234", type = "hidden" }]
}

resource "vaultwarden_item_secure_note" "test" {
  name  = "Note"
  notes = "Secret note"
}
`

func TestAccItemDataSources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccItemProviderConfig() + testAccItemsConfig + `
data "vaultwarden_item_login" "test" {
  id = vaultwarden_item_login.test.id
}

data "vaultwarden_item_secure_note" "test" {
  id = vaultwarden_item_secure_note.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.vaultwarden_item_login.test", "folder_id", "vaultwarden_folder.test", "id"),
					resource.TestCheckNoResourceAttr("data.vaultwarden_item_login.test", "organization_id"),
					resource.TestCheckResourceAttr("data.vaultwarden_item_login.test", "name", "Login"),
					resource.TestCheckResourceAttr("data.vaultwarden_item_login.test", "notes", "Login notes"),
					resource.TestCheckResourceAttr("data.vaultwarden_item_login.test", "username", "user"),
					resource.TestCheckResourceAttr("data.vaultwarden_item_login.test", "password", "secret password"),
					resource.TestCheckResourceAttr("data.vaultwarden_item_login.test", "totp", "otpauth://totp/Example?secret=JBSWY3DPEHPK3PXP"),
					resource.TestCheckResourceAttr("data.vaultwarden_item_login.test", "uris.0.uri", "https://example.com"),
					resource.TestCheckResourceAttr("data.vaultwarden_item_login.test", "uris.0.match", "host"),
					resource.TestCheckResourceAttr("data.vaultwarden_item_login.test", "fields.0.value", "1234"),
					resource.TestCheckResourceAttr("data.vaultwarden_item_login.test", "fields.0.type", "hidden"),
					resource.TestCheckResourceAttr("data.vaultwarden_item_secure_note.test", "name", "Note"),
					resource.TestCheckResourceAttr("data.vaultwarden_item_secure_note.test", "notes", "Secret note"),
				),
			},
			{
				Config: testAccItemProviderConfig() + testAccItemsConfig + `
data "vaultwarden_item_login" "test" {
  id = vaultwarden_item_secure_note.test.id
}
`,
				ExpectError: regexp.MustCompile(`is a secure note, not a login`),
			},
			{
				Config: testAccItemProviderConfig() + testAccItemsConfig + `
data "vaultwarden_item_secure_note" "test" {
  id = "00000000-0000-0000-0000-000000000000"

  depends_on = [vaultwarden_item_secure_note.test]
}
`,
				ExpectError: regexp.MustCompile(`Item Not Found`),
			},
		},
	})
}

func TestAccItemEphemeralResources(t *testing.T) {
	factories := map[string]func() (tfprotov6.ProviderServer, error){
		"echo": echoprovider.NewProviderServer(),
	}
	for name, factory := range testAccProtoV6ProviderFactories {
		factories[name] = factory
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: factories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccItemProviderConfig() + testAccItemsConfig,
			},
			{
				Config: testAccItemProviderConfig() + testAccItemsConfig + `
ephemeral "vaultwarden_item_login" "test" {
  id = vaultwarden_item_login.test.id
}

ephemeral "vaultwarden_item_secure_note" "test" {
  id = vaultwarden_item_secure_note.test.id
}

provider "echo" {
  data = {
    password = ephemeral.vaultwarden_item_login.test.password
    totp     = ephemeral.vaultwarden_item_login.test.totp
    uri      = ephemeral.vaultwarden_item_login.test.uris[0].uri
    field    = ephemeral.vaultwarden_item_login.test.fields[0].value
    note     = ephemeral.vaultwarden_item_secure_note.test.notes
  }
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data"), knownvalue.ObjectExact(map[string]knownvalue.Check{
						"password": knownvalue.StringExact("secret password"),
						"totp":     knownvalue.StringExact("otpauth://totp/Example?secret=JBSWY3DPEHPK3PXP"),
						"uri":      knownvalue.StringExact("https://example.com"),
						"field":    knownvalue.StringExact("1234"),
						"note":     knownvalue.StringExact("Secret note"),
					})),
				},
			},
			{
				Config: testAccItemProviderConfig() + testAccItemsConfig + `
ephemeral "vaultwarden_item_secure_note" "test" {
  id = vaultwarden_item_login.test.id
}

provider "echo" {
  data = ephemeral.vaultwarden_item_secure_note.test.notes
}

resource "echo" "test" {}
`,
				ExpectError: regexp.MustCompile(`is a login, not a secure note`),
			},
			// A valid configuration for the destroy after the test
			{
				Config: testAccItemProviderConfig() + testAccItemsConfig,
			},
		},
	})
}
