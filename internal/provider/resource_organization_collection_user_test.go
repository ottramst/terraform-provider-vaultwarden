package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

func TestAccOrganizationCollectionUser(t *testing.T) {
	orgName := gofakeit.Company()
	userEmail := gofakeit.Email()
	managerEmail := gofakeit.Email()
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Several users in the same collection, created in parallel
			{
				Config: testAccOrganizationCollectionUserConfig(orgName, userEmail, managerEmail, `
resource "vaultwarden_organization_collection_user" "user_first" {
  organization_id      = vaultwarden_organization.test.id
  collection_id        = vaultwarden_organization_collection.first.id
  organization_user_id = vaultwarden_organization_user.user.id
  read_only            = true
}

resource "vaultwarden_organization_collection_user" "user_second" {
  organization_id      = vaultwarden_organization.test.id
  collection_id        = vaultwarden_organization_collection.second.id
  organization_user_id = vaultwarden_organization_user.user.id
  hide_passwords       = true
}

resource "vaultwarden_organization_collection_user" "manager_first" {
  organization_id      = vaultwarden_organization.test.id
  collection_id        = vaultwarden_organization_collection.first.id
  organization_user_id = vaultwarden_organization_user.manager.id
}
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_collection_user.user_first", "read_only", "true"),
					resource.TestCheckResourceAttr("vaultwarden_organization_collection_user.user_first", "hide_passwords", "false"),
					resource.TestCheckResourceAttr("vaultwarden_organization_collection_user.user_first", "manage", "false"),
					resource.TestCheckResourceAttr("vaultwarden_organization_collection_user.user_second", "hide_passwords", "true"),
					// A Manager that can edit and see passwords can always manage; that is not drift
					resource.TestCheckResourceAttr("vaultwarden_organization_collection_user.manager_first", "manage", "false"),
					resource.TestCheckResourceAttrPair(
						"vaultwarden_organization_collection_user.user_first", "organization_user_id",
						"vaultwarden_organization_user.user", "id",
					),
					testAccCheckCollectionUsers(t, ids, "vaultwarden_organization_collection.first", "vaultwarden_organization_user.user", "vaultwarden_organization_user.manager"),
				),
			},
			// Update the access
			{
				Config: testAccOrganizationCollectionUserConfig(orgName, userEmail, managerEmail, `
resource "vaultwarden_organization_collection_user" "user_first" {
  organization_id      = vaultwarden_organization.test.id
  collection_id        = vaultwarden_organization_collection.first.id
  organization_user_id = vaultwarden_organization_user.user.id
  manage               = true
}

resource "vaultwarden_organization_collection_user" "user_second" {
  organization_id      = vaultwarden_organization.test.id
  collection_id        = vaultwarden_organization_collection.second.id
  organization_user_id = vaultwarden_organization_user.user.id
  hide_passwords       = true
}

resource "vaultwarden_organization_collection_user" "manager_first" {
  organization_id      = vaultwarden_organization.test.id
  collection_id        = vaultwarden_organization_collection.first.id
  organization_user_id = vaultwarden_organization_user.manager.id
}
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_collection_user.user_first", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_collection_user.user_first", "read_only", "false"),
					resource.TestCheckResourceAttr("vaultwarden_organization_collection_user.user_first", "manage", "true"),
					testAccCheckCollectionUsers(t, ids, "vaultwarden_organization_collection.first", "vaultwarden_organization_user.user", "vaultwarden_organization_user.manager"),
				),
			},
			// Import testing
			{
				ResourceName:      "vaultwarden_organization_collection_user.user_first",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Removed outside Terraform: it is recreated
			{
				PreConfig: func() {
					ctx := context.Background()
					client := testAccClient(ctx, t)
					testAccNoError(t, client.RemoveOrganizationCollectionUser(ctx, ids["org"], ids["second"], ids["user"]))
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			// Removing one assignment keeps the others
			{
				Config: testAccOrganizationCollectionUserConfig(orgName, userEmail, managerEmail, `
resource "vaultwarden_organization_collection_user" "manager_first" {
  organization_id      = vaultwarden_organization.test.id
  collection_id        = vaultwarden_organization_collection.first.id
  organization_user_id = vaultwarden_organization_user.manager.id
}
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckCollectionUsers(t, ids, "vaultwarden_organization_collection.first", "vaultwarden_organization_user.manager"),
				),
			},
		},
	})
}

func TestAccOrganizationCollectionUserAccessAll(t *testing.T) {
	orgName := gofakeit.Company()
	email := gofakeit.Email()

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
  admin_token     = %[4]q
}

resource "vaultwarden_organization" "test" {
  name = %[5]q
}

resource "vaultwarden_organization_collection" "test" {
  organization_id = vaultwarden_organization.test.id
  name            = "Collection"
}

resource "vaultwarden_organization_user" "admin" {
  organization_id = vaultwarden_organization.test.id
  email           = %[6]q
  type            = "Admin"
}

resource "vaultwarden_organization_collection_user" "test" {
  organization_id      = vaultwarden_organization.test.id
  collection_id        = vaultwarden_organization_collection.test.id
  organization_user_id = vaultwarden_organization_user.admin.id
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, email),
				ExpectError: regexp.MustCompile(`Organization user has access to all collections`),
			},
		},
	})
}

func TestAccOrganizationCollectionUserInvalidImportID(t *testing.T) {
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

resource "vaultwarden_organization_collection_user" "test" {
  organization_id      = "org"
  collection_id        = "collection"
  organization_user_id = "user"
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword),
				ResourceName:  "vaultwarden_organization_collection_user.test",
				ImportState:   true,
				ImportStateId: "org/collection",
				ExpectError:   regexp.MustCompile(`Invalid ID format`),
			},
		},
	})
}

// testAccCheckCollectionUsers checks that exactly the given organization users are assigned to the
// collection, and stores the IDs used by the test in ids.
func testAccCheckCollectionUsers(t *testing.T, ids map[string]string, collection string, users ...string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resources := s.RootModule().Resources
		col, ok := resources[collection]
		if !ok {
			return fmt.Errorf("resource %s not found in state", collection)
		}
		orgID := col.Primary.Attributes["organization_id"]
		ids["org"] = orgID
		ids["user"] = resources["vaultwarden_organization_user.user"].Primary.ID
		ids["second"] = resources["vaultwarden_organization_collection.second"].Primary.ID

		ctx := context.Background()
		access, err := testAccClient(ctx, t).GetOrganizationCollectionAccess(ctx, orgID, col.Primary.ID)
		if err != nil {
			return err
		}

		want := map[string]bool{}
		for _, user := range users {
			want[resources[user].Primary.ID] = true
		}
		got := map[string]bool{}
		for _, user := range access.Users {
			got[user.ID] = true
		}
		if len(got) != len(want) {
			return fmt.Errorf("collection %s has users %s, want %s", col.Primary.ID, testAccKeys(got), testAccKeys(want))
		}
		for id := range want {
			if !got[id] {
				return fmt.Errorf("collection %s has users %s, want %s", col.Primary.ID, testAccKeys(got), testAccKeys(want))
			}
		}
		return nil
	}
}

func testAccKeys(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return "[" + strings.Join(keys, ", ") + "]"
}

func testAccOrganizationCollectionUserConfig(orgName, userEmail, managerEmail, extra string) string {
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

resource "vaultwarden_organization_collection" "first" {
  organization_id = vaultwarden_organization.test.id
  name            = "First"
}

resource "vaultwarden_organization_collection" "second" {
  organization_id = vaultwarden_organization.test.id
  name            = "Second"
}

resource "vaultwarden_organization_user" "user" {
  organization_id = vaultwarden_organization.test.id
  email           = %[6]q
}

resource "vaultwarden_organization_user" "manager" {
  organization_id = vaultwarden_organization.test.id
  email           = %[7]q
  type            = "Manager"
}
%[8]s`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, userEmail, managerEmail, extra)
}
