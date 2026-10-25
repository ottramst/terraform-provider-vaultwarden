package provider

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

func TestAccItemLogin(t *testing.T) {
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckItemDestroyed(t, ids),
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccItemLoginConfig(`
  folder_id = vaultwarden_folder.a.id
  name      = "Example"
  notes     = "Notes"
  favorite  = true
  username  = "user@example.com"
  password  = "p4ssw0rd!"
  totp      = "JBSWY3DPEHPK3PXP"
  uris = [
    { uri = "https://example.com/login", match = "starts_with" },
    { uri = "https://example.org" },
  ]
  fields = [
    { name = "Text", value = "visible" },
    { name = "Secret", value = "hidden value", type = "hidden" },
    { name = "Enabled", value = "true", type = "boolean" },
    { name = "Linked", type = "linked", linked_id = 100 },
  ]
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("vaultwarden_item_login.test", "id"),
					resource.TestCheckResourceAttrPair("vaultwarden_item_login.test", "folder_id", "vaultwarden_folder.a", "id"),
					resource.TestCheckNoResourceAttr("vaultwarden_item_login.test", "organization_id"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "collection_ids.#", "0"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "name", "Example"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "notes", "Notes"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "favorite", "true"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "reprompt", "false"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "username", "user@example.com"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "password", "p4ssw0rd!"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "totp", "JBSWY3DPEHPK3PXP"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "uris.#", "2"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "uris.0.match", "starts_with"),
					resource.TestCheckNoResourceAttr("vaultwarden_item_login.test", "uris.1.match"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "fields.#", "4"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "fields.0.type", "text"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "fields.1.value", "hidden value"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "fields.3.linked_id", "100"),
					testAccCaptureItemID(ids, "vaultwarden_item_login.test"),
					testAccCheckItem(t, ids, func(item *testItem) error {
						if !item.HasKey || item.Login.Password != "p4ssw0rd!" || len(item.Login.URIs) != 2 || len(item.Fields) != 4 {
							return fmt.Errorf("unexpected item %+v, login %+v", item, item.Login)
						}
						return nil
					}),
				),
			},
			// Update the password and URIs, and move the item to another folder
			{
				Config: testAccItemLoginConfig(`
  folder_id = vaultwarden_folder.b.id
  name      = "Example"
  username  = "user@example.com"
  password  = "n3w-p4ssw0rd!"
  reprompt  = true
  uris = [
    { uri = "https://example.net", match = "never" },
  ]
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_login.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("vaultwarden_item_login.test", "folder_id", "vaultwarden_folder.b", "id"),
					resource.TestCheckNoResourceAttr("vaultwarden_item_login.test", "notes"),
					resource.TestCheckNoResourceAttr("vaultwarden_item_login.test", "totp"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "favorite", "false"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "reprompt", "true"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "password", "n3w-p4ssw0rd!"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "uris.#", "1"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "uris.0.match", "never"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "fields.#", "0"),
					testAccCheckItem(t, ids, func(item *testItem) error {
						if len(item.PasswordHistory) != 1 || item.PasswordHistory[0].Password != "p4ssw0rd!" {
							return fmt.Errorf("expected the previous password in the history, got %+v", item.PasswordHistory)
						}
						return nil
					}),
				),
			},
			// Import
			{
				ResourceName:      "vaultwarden_item_login.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Move the item out of the folder
			{
				Config: testAccItemLoginConfig(`
  name     = "Example"
  username = "user@example.com"
  password = "n3w-p4ssw0rd!"
  reprompt = true
  uris = [
    { uri = "https://example.net", match = "never" },
  ]
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_login.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("vaultwarden_item_login.test", "folder_id"),
					testAccCheckItem(t, ids, func(item *testItem) error {
						if item.FolderID != "" {
							return fmt.Errorf("item is still in folder %s", item.FolderID)
						}
						return nil
					}),
				),
			},
			// Changed outside Terraform: the change is reverted
			{
				PreConfig: func() {
					ctx := context.Background()
					client := testAccClient(ctx, t)
					item, err := client.GetItem(ctx, ids["item"])
					testAccNoError(t, err)
					item.Name = "Changed"
					item.Login.Password = "changed"
					_, err = client.UpdateItem(ctx, *item)
					testAccNoError(t, err)
				},
				Config: testAccItemLoginConfig(`
  name     = "Example"
  username = "user@example.com"
  password = "n3w-p4ssw0rd!"
  reprompt = true
  uris = [
    { uri = "https://example.net", match = "never" },
  ]
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_login.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "name", "Example"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "password", "n3w-p4ssw0rd!"),
				),
			},
			// Deleted outside Terraform: it is recreated
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).DeleteItem(ctx, ids["item"]))
				},
				Config: testAccItemLoginConfig(`
  name     = "Example"
  username = "user@example.com"
  password = "n3w-p4ssw0rd!"
  reprompt = true
  uris = [
    { uri = "https://example.net", match = "never" },
  ]
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_login.test", plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureItemID(ids, "vaultwarden_item_login.test"),
			},
		},
	})
}

func TestAccItemLoginOrganization(t *testing.T) {
	orgName := gofakeit.Company()
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create in a collection
			{
				Config: testAccItemOrganizationConfig(orgName, `
  organization_id = vaultwarden_organization.test.id
  collection_ids  = [vaultwarden_organization_collection.a.id]
  folder_id       = vaultwarden_folder.test.id
  name            = "Shared"
  username        = "shared"
  password        = "shared password"
  uris            = [{ uri = "https://shared.example.com" }]
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("vaultwarden_item_login.test", "organization_id", "vaultwarden_organization.test", "id"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "collection_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair("vaultwarden_item_login.test", "collection_ids.*", "vaultwarden_organization_collection.a", "id"),
					resource.TestCheckResourceAttrPair("vaultwarden_item_login.test", "folder_id", "vaultwarden_folder.test", "id"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "password", "shared password"),
					testAccCaptureItemID(ids, "vaultwarden_item_login.test"),
					testAccCaptureItemOrganizationIDs(ids),
				),
			},
			// Move it to the other collection
			{
				Config: testAccItemOrganizationConfig(orgName, `
  organization_id = vaultwarden_organization.test.id
  collection_ids  = [vaultwarden_organization_collection.b.id]
  folder_id       = vaultwarden_folder.test.id
  name            = "Shared"
  username        = "shared"
  password        = "shared password"
  uris            = [{ uri = "https://shared.example.com" }]
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_login.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "collection_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair("vaultwarden_item_login.test", "collection_ids.*", "vaultwarden_organization_collection.b", "id"),
					testAccCheckItem(t, ids, func(item *testItem) error {
						if !slices.Equal(item.CollectionIDs, []string{ids["collection_b"]}) {
							return fmt.Errorf("expected the item in collection %s, got %v", ids["collection_b"], item.CollectionIDs)
						}
						return nil
					}),
				),
			},
			// Assign it to both collections, and change the password
			{
				Config: testAccItemOrganizationConfig(orgName, `
  organization_id = vaultwarden_organization.test.id
  collection_ids  = [vaultwarden_organization_collection.a.id, vaultwarden_organization_collection.b.id]
  name            = "Shared"
  username        = "shared"
  password        = "new shared password"
  uris            = [{ uri = "https://shared.example.com" }]
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "collection_ids.#", "2"),
					resource.TestCheckNoResourceAttr("vaultwarden_item_login.test", "folder_id"),
					testAccCheckItem(t, ids, func(item *testItem) error {
						if len(item.CollectionIDs) != 2 || len(item.PasswordHistory) != 1 || item.PasswordHistory[0].Password != "shared password" {
							return fmt.Errorf("unexpected item %+v", item)
						}
						return nil
					}),
				),
			},
			// Import
			{
				ResourceName:      "vaultwarden_item_login.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Moving the item to the personal vault replaces it
			{
				Config: testAccItemOrganizationConfig(orgName, `
  name     = "Shared"
  username = "shared"
  password = "new shared password"
  uris     = [{ uri = "https://shared.example.com" }]
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_login.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("vaultwarden_item_login.test", "organization_id"),
					resource.TestCheckResourceAttr("vaultwarden_item_login.test", "collection_ids.#", "0"),
				),
			},
		},
	})
}

func TestAccItemSecureNote(t *testing.T) {
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckItemDestroyed(t, ids),
		Steps: []resource.TestStep{
			{
				Config: testAccItemSecureNoteConfig(`
  name  = "Note"
  notes = "Line one\nLine two"
  fields = [{ name = "Key", value = "Value" }]
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_item_secure_note.test", "name", "Note"),
					resource.TestCheckResourceAttr("vaultwarden_item_secure_note.test", "notes", "Line one\nLine two"),
					resource.TestCheckResourceAttr("vaultwarden_item_secure_note.test", "fields.#", "1"),
					testAccCaptureItemID(ids, "vaultwarden_item_secure_note.test"),
				),
			},
			{
				Config: testAccItemSecureNoteConfig(`
  name  = "Note"
  notes = "Changed"
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_secure_note.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_item_secure_note.test", "notes", "Changed"),
					resource.TestCheckResourceAttr("vaultwarden_item_secure_note.test", "fields.#", "0"),
				),
			},
			{
				ResourceName:      "vaultwarden_item_secure_note.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Deleted outside Terraform: it is recreated
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).DeleteItem(ctx, ids["item"]))
				},
				Config: testAccItemSecureNoteConfig(`
  name  = "Note"
  notes = "Changed"
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_secure_note.test", plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureItemID(ids, "vaultwarden_item_secure_note.test"),
			},
			// Importing a login as a secure note fails
			{
				Config: testAccItemSecureNoteConfig(`
  name  = "Note"
  notes = "Changed"
`) + `
resource "vaultwarden_item_login" "other" {
  name = "Login"
}
`,
			},
			{
				Config: testAccItemSecureNoteConfig(`
  name  = "Note"
  notes = "Changed"
`) + `
resource "vaultwarden_item_login" "other" {
  name = "Login"
}
`,
				ResourceName: "vaultwarden_item_secure_note.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["vaultwarden_item_login.other"].Primary.ID, nil
				},
				ExpectError: regexp.MustCompile(`is a login, not a secure note`),
			},
		},
	})
}

// testItem is the item read with the test client.
type testItem = models.Item

// testAccCaptureItemID stores the ID of an item resource as ids["item"].
func testAccCaptureItemID(ids map[string]string, address string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ids["item"] = s.RootModule().Resources[address].Primary.ID
		return nil
	}
}

// testAccCaptureItemOrganizationIDs stores the IDs of the collections in testAccItemOrganizationConfig.
func testAccCaptureItemOrganizationIDs(ids map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ids["collection_a"] = s.RootModule().Resources["vaultwarden_organization_collection.a"].Primary.ID
		ids["collection_b"] = s.RootModule().Resources["vaultwarden_organization_collection.b"].Primary.ID
		return nil
	}
}

// testAccCheckItem checks the item ids["item"] as read and decrypted by the test client, which
// logs in on its own.
func testAccCheckItem(t *testing.T, ids map[string]string, check func(item *testItem) error) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		item, err := testAccClient(ctx, t).GetItem(ctx, ids["item"])
		if err != nil {
			return err
		}
		return check(item)
	}
}

// testAccCheckItemDestroyed checks that the last item of the test is deleted.
func testAccCheckItemDestroyed(t *testing.T, ids map[string]string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		if _, err := testAccClient(ctx, t).GetItem(ctx, ids["item"]); !vaultwarden.IsNotFound(err) {
			return fmt.Errorf("item %s was not deleted: %v", ids["item"], err)
		}
		return nil
	}
}

func testAccItemProviderConfig() string {
	return fmt.Sprintf(`
provider "vaultwarden" {
  endpoint        = %[1]q
  email           = %[2]q
  master_password = %[3]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword)
}

func testAccItemLoginConfig(attributes string) string {
	return testAccItemProviderConfig() + `
resource "vaultwarden_folder" "a" {
  name = "Item folder A"
}

resource "vaultwarden_folder" "b" {
  name = "Item folder B"
}

resource "vaultwarden_item_login" "test" {` + attributes + `}
`
}

func testAccItemSecureNoteConfig(attributes string) string {
	return testAccItemProviderConfig() + `
resource "vaultwarden_item_secure_note" "test" {` + attributes + `}
`
}

func testAccItemOrganizationConfig(orgName, attributes string) string {
	return testAccItemProviderConfig() + fmt.Sprintf(`
resource "vaultwarden_organization" "test" {
  name = %[1]q
}

resource "vaultwarden_organization_collection" "a" {
  organization_id = vaultwarden_organization.test.id
  name            = "A"
}

resource "vaultwarden_organization_collection" "b" {
  organization_id = vaultwarden_organization.test.id
  name            = "B"
}

resource "vaultwarden_folder" "test" {
  name = "Organization items"
}

resource "vaultwarden_item_login" "test" {`, orgName) + attributes + `}
`
}
