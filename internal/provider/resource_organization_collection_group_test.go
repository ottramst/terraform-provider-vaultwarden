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

const testAccCollectionGroupEngineeringFirst = `
resource "vaultwarden_organization_collection_group" "engineering_first" {
  organization_id = vaultwarden_organization.test.id
  collection_id   = vaultwarden_organization_collection.first.id
  group_id        = vaultwarden_organization_group.engineering.id
  read_only       = true
}
`

const testAccCollectionGroupEngineeringFirstManage = `
resource "vaultwarden_organization_collection_group" "engineering_first" {
  organization_id = vaultwarden_organization.test.id
  collection_id   = vaultwarden_organization_collection.first.id
  group_id        = vaultwarden_organization_group.engineering.id
  manage          = true
}
`

const testAccCollectionGroupSupportFirst = `
resource "vaultwarden_organization_collection_group" "support_first" {
  organization_id = vaultwarden_organization.test.id
  collection_id   = vaultwarden_organization_collection.first.id
  group_id        = vaultwarden_organization_group.support.id
  hide_passwords  = true
}
`

const testAccCollectionGroupEngineeringSecond = `
resource "vaultwarden_organization_collection_group" "engineering_second" {
  organization_id = vaultwarden_organization.test.id
  collection_id   = vaultwarden_organization_collection.second.id
  group_id        = vaultwarden_organization_group.engineering.id
}
`

func TestAccOrganizationCollectionGroup(t *testing.T) {
	orgName := gofakeit.Company()
	userEmail := gofakeit.Email()
	ids := map[string]string{}
	config := func(collectionName, groupName string, assignments ...string) string {
		return testAccOrganizationCollectionGroupConfig(orgName, collectionName, groupName, userEmail, assignments...)
	}
	all := []string{testAccCollectionGroupEngineeringFirstManage, testAccCollectionGroupSupportFirst, testAccCollectionGroupEngineeringSecond}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Several groups in the same collection, created in parallel with a user's access
			{
				Config: config("First", "Engineering", testAccCollectionGroupEngineeringFirst, testAccCollectionGroupSupportFirst, testAccCollectionGroupEngineeringSecond),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_collection_group.engineering_first", "read_only", "true"),
					resource.TestCheckResourceAttr("vaultwarden_organization_collection_group.engineering_first", "hide_passwords", "false"),
					resource.TestCheckResourceAttr("vaultwarden_organization_collection_group.engineering_first", "manage", "false"),
					resource.TestCheckResourceAttr("vaultwarden_organization_collection_group.support_first", "hide_passwords", "true"),
					// Vaultwarden reports manage for a group that can edit and see passwords; that is not drift
					resource.TestCheckResourceAttr("vaultwarden_organization_collection_group.engineering_second", "manage", "false"),
					resource.TestCheckResourceAttrPair("vaultwarden_organization_collection_group.engineering_first", "group_id", "vaultwarden_organization_group.engineering", "id"),
					testAccCaptureCollectionGroupIDs(ids),
					testAccCheckCollectionGroups(t, ids, "first", "engineering", "support"),
					testAccCheckCollectionGroups(t, ids, "second", "engineering"),
					testAccCheckCollectionGroupUserAccess(t, ids),
				),
			},
			// Update the access
			{
				Config: config("First", "Engineering", all...),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_collection_group.engineering_first", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_collection_group.engineering_first", "read_only", "false"),
					resource.TestCheckResourceAttr("vaultwarden_organization_collection_group.engineering_first", "manage", "true"),
					testAccCheckCollectionGroups(t, ids, "first", "engineering", "support"),
					testAccCheckCollectionGroupUserAccess(t, ids),
				),
			},
			// Updating the collection keeps the access of the groups
			{
				Config: config("Renamed", "Engineering", all...),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_collection.first", plancheck.ResourceActionUpdate),
					},
				},
				Check: testAccCheckCollectionGroups(t, ids, "first", "engineering", "support"),
			},
			// Updating the group keeps its access
			{
				Config: config("Renamed", "Developers", all...),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_group.engineering", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckCollectionGroups(t, ids, "first", "engineering", "support"),
					testAccCheckCollectionGroups(t, ids, "second", "engineering"),
				),
			},
			// Import
			{
				ResourceName:      "vaultwarden_organization_collection_group.support_first",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Removed outside Terraform: it is recreated
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).RemoveOrganizationCollectionGroup(ctx, ids["org"], ids["first"], ids["support"]))
				},
				Config: config("Renamed", "Developers", all...),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_collection_group.support_first", plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCheckCollectionGroups(t, ids, "first", "engineering", "support"),
			},
			// Removing one assignment keeps the others
			{
				Config: config("Renamed", "Developers", testAccCollectionGroupSupportFirst, testAccCollectionGroupEngineeringSecond),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckCollectionGroups(t, ids, "first", "support"),
					testAccCheckCollectionGroups(t, ids, "second", "engineering"),
					testAccCheckCollectionGroupUserAccess(t, ids),
				),
			},
		},
	})
}

func TestAccOrganizationCollectionGroupInvalidImportID(t *testing.T) {
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

resource "vaultwarden_organization_collection_group" "test" {
  organization_id = "org"
  collection_id   = "collection"
  group_id        = "group"
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword),
				ResourceName:  "vaultwarden_organization_collection_group.test",
				ImportState:   true,
				ImportStateId: "org/collection",
				ExpectError:   regexp.MustCompile(`Invalid ID format`),
			},
		},
	})
}

// testAccCaptureCollectionGroupIDs stores the IDs of the resources in testAccOrganizationCollectionGroupConfig.
func testAccCaptureCollectionGroupIDs(ids map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resources := s.RootModule().Resources
		ids["org"] = resources["vaultwarden_organization.test"].Primary.ID
		ids["first"] = resources["vaultwarden_organization_collection.first"].Primary.ID
		ids["second"] = resources["vaultwarden_organization_collection.second"].Primary.ID
		ids["engineering"] = resources["vaultwarden_organization_group.engineering"].Primary.ID
		ids["support"] = resources["vaultwarden_organization_group.support"].Primary.ID
		ids["user"] = resources["vaultwarden_organization_user.test"].Primary.ID
		return nil
	}
}

// testAccCheckCollectionGroups checks that exactly the given groups have access to the collection.
func testAccCheckCollectionGroups(t *testing.T, ids map[string]string, collection string, groups ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		access, err := testAccClient(ctx, t).GetOrganizationCollectionAccess(ctx, ids["org"], ids[collection])
		if err != nil {
			return err
		}

		want := map[string]bool{}
		for _, group := range groups {
			want[ids[group]] = true
		}
		got := map[string]bool{}
		for _, group := range access.Groups {
			got[group.ID] = true
		}
		if len(got) != len(want) {
			return fmt.Errorf("collection %s has groups %s, want %s", collection, testAccKeys(got), testAccKeys(want))
		}
		for id := range want {
			if !got[id] {
				return fmt.Errorf("collection %s has groups %s, want %s", collection, testAccKeys(got), testAccKeys(want))
			}
		}
		return nil
	}
}

// testAccCheckCollectionGroupUserAccess checks that the user still has read-only access to the first collection.
func testAccCheckCollectionGroupUserAccess(t *testing.T, ids map[string]string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		access, err := testAccClient(ctx, t).GetOrganizationCollectionAccess(ctx, ids["org"], ids["first"])
		if err != nil {
			return err
		}
		for _, user := range access.Users {
			if user.ID == ids["user"] && user.ReadOnly {
				return nil
			}
		}
		return fmt.Errorf("read-only access of organization user %s to collection %s was not preserved: %+v", ids["user"], ids["first"], access.Users)
	}
}

func testAccOrganizationCollectionGroupConfig(orgName, collectionName, groupName, userEmail string, assignments ...string) string {
	config := fmt.Sprintf(`
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
  name            = %[6]q
}

resource "vaultwarden_organization_collection" "second" {
  organization_id = vaultwarden_organization.test.id
  name            = "Second"
}

resource "vaultwarden_organization_group" "engineering" {
  organization_id = vaultwarden_organization.test.id
  name            = %[7]q
}

resource "vaultwarden_organization_group" "support" {
  organization_id = vaultwarden_organization.test.id
  name            = "Support"
}

resource "vaultwarden_organization_user" "test" {
  organization_id = vaultwarden_organization.test.id
  email           = %[8]q
}

resource "vaultwarden_organization_collection_user" "test" {
  organization_id      = vaultwarden_organization.test.id
  collection_id        = vaultwarden_organization_collection.first.id
  organization_user_id = vaultwarden_organization_user.test.id
  read_only            = true
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, collectionName, groupName, userEmail)
	for _, assignment := range assignments {
		config += assignment
	}
	return config
}
