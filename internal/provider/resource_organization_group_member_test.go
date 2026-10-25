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

const testAccGroupMemberAFirst = `
resource "vaultwarden_organization_group_member" "a_first" {
  organization_id      = vaultwarden_organization.test.id
  group_id             = vaultwarden_organization_group.first.id
  organization_user_id = vaultwarden_organization_user.a.id
}
`

const testAccGroupMemberBFirst = `
resource "vaultwarden_organization_group_member" "b_first" {
  organization_id      = vaultwarden_organization.test.id
  group_id             = vaultwarden_organization_group.first.id
  organization_user_id = vaultwarden_organization_user.b.id
}
`

const testAccGroupMemberASecond = `
resource "vaultwarden_organization_group_member" "a_second" {
  organization_id      = vaultwarden_organization.test.id
  group_id             = vaultwarden_organization_group.second.id
  organization_user_id = vaultwarden_organization_user.a.id
}
`

func TestAccOrganizationGroupMember(t *testing.T) {
	orgName := gofakeit.Company()
	aEmail := gofakeit.Email()
	bEmail := gofakeit.Email()
	ids := map[string]string{}
	config := func(firstName, aType string, members ...string) string {
		return testAccOrganizationGroupMemberConfig(orgName, firstName, aEmail, aType, bEmail, members...)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Several members of the same group, created in parallel
			{
				Config: config("First", "User", testAccGroupMemberAFirst, testAccGroupMemberBFirst),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("vaultwarden_organization_group_member.a_first", "group_id", "vaultwarden_organization_group.first", "id"),
					resource.TestCheckResourceAttrPair("vaultwarden_organization_group_member.a_first", "organization_user_id", "vaultwarden_organization_user.a", "id"),
					testAccCaptureGroupMemberIDs(ids),
					testAccCheckGroupMembers(t, ids, "first", "a", "b"),
					testAccCheckGroupMembers(t, ids, "second"),
				),
			},
			// Adding a user to another group keeps its other groups and its collection access
			{
				Config: config("First", "User", testAccGroupMemberAFirst, testAccGroupMemberBFirst, testAccGroupMemberASecond),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckGroupMembers(t, ids, "first", "a", "b"),
					testAccCheckGroupMembers(t, ids, "second", "a"),
					testAccCheckGroupMemberCollectionAccess(t, ids),
				),
			},
			// Updating the group keeps its members
			{
				Config: config("Renamed", "User", testAccGroupMemberAFirst, testAccGroupMemberBFirst, testAccGroupMemberASecond),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_group.first", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckGroupMembers(t, ids, "first", "a", "b"),
					testAccCheckGroupMembers(t, ids, "second", "a"),
				),
			},
			// Updating the organization user keeps its groups
			{
				Config: config("Renamed", "Manager", testAccGroupMemberAFirst, testAccGroupMemberBFirst, testAccGroupMemberASecond),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_user.a", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckGroupMembers(t, ids, "first", "a", "b"),
					testAccCheckGroupMembers(t, ids, "second", "a"),
				),
			},
			// Import
			{
				ResourceName:      "vaultwarden_organization_group_member.a_second",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Removed outside Terraform: it is recreated
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).RemoveOrganizationGroupMember(ctx, ids["org"], ids["first"], ids["b"]))
				},
				Config: config("Renamed", "Manager", testAccGroupMemberAFirst, testAccGroupMemberBFirst, testAccGroupMemberASecond),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_group_member.b_first", plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCheckGroupMembers(t, ids, "first", "a", "b"),
			},
			// Removing one member keeps the others
			{
				Config: config("Renamed", "Manager", testAccGroupMemberBFirst, testAccGroupMemberASecond),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckGroupMembers(t, ids, "first", "b"),
					testAccCheckGroupMembers(t, ids, "second", "a"),
				),
			},
		},
	})
}

func TestAccOrganizationGroupMemberInvalidImportID(t *testing.T) {
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

resource "vaultwarden_organization_group_member" "test" {
  organization_id      = "org"
  group_id             = "group"
  organization_user_id = "user"
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword),
				ResourceName:  "vaultwarden_organization_group_member.test",
				ImportState:   true,
				ImportStateId: "org/group",
				ExpectError:   regexp.MustCompile(`Invalid ID format`),
			},
		},
	})
}

// testAccCaptureGroupMemberIDs stores the IDs of the resources in testAccOrganizationGroupMemberConfig.
func testAccCaptureGroupMemberIDs(ids map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resources := s.RootModule().Resources
		ids["org"] = resources["vaultwarden_organization.test"].Primary.ID
		ids["first"] = resources["vaultwarden_organization_group.first"].Primary.ID
		ids["second"] = resources["vaultwarden_organization_group.second"].Primary.ID
		ids["a"] = resources["vaultwarden_organization_user.a"].Primary.ID
		ids["b"] = resources["vaultwarden_organization_user.b"].Primary.ID
		ids["collection"] = resources["vaultwarden_organization_collection.test"].Primary.ID
		return nil
	}
}

// testAccCheckGroupMembers checks that exactly the given organization users are members of the group.
func testAccCheckGroupMembers(t *testing.T, ids map[string]string, group string, users ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		members, err := testAccClient(ctx, t).GetOrganizationGroupMembers(ctx, ids["org"], ids[group])
		if err != nil {
			return err
		}

		want := map[string]bool{}
		for _, user := range users {
			want[ids[user]] = true
		}
		got := map[string]bool{}
		for _, member := range members {
			got[member] = true
		}
		if len(got) != len(want) {
			return fmt.Errorf("group %s has members %s, want %s", group, testAccKeys(got), testAccKeys(want))
		}
		for id := range want {
			if !got[id] {
				return fmt.Errorf("group %s has members %s, want %s", group, testAccKeys(got), testAccKeys(want))
			}
		}
		return nil
	}
}

// testAccCheckGroupMemberCollectionAccess checks that user a still has read-only access to the collection.
func testAccCheckGroupMemberCollectionAccess(t *testing.T, ids map[string]string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		access, err := testAccClient(ctx, t).GetOrganizationCollectionAccess(ctx, ids["org"], ids["collection"])
		if err != nil {
			return err
		}
		for _, user := range access.Users {
			if user.ID == ids["a"] && user.ReadOnly {
				return nil
			}
		}
		return fmt.Errorf("read-only access of organization user %s to collection %s was not preserved: %+v", ids["a"], ids["collection"], access.Users)
	}
}

func testAccOrganizationGroupMemberConfig(orgName, firstName, aEmail, aType, bEmail string, members ...string) string {
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

resource "vaultwarden_organization_group" "first" {
  organization_id = vaultwarden_organization.test.id
  name            = %[6]q
}

resource "vaultwarden_organization_group" "second" {
  organization_id = vaultwarden_organization.test.id
  name            = "Second"
}

resource "vaultwarden_organization_collection" "test" {
  organization_id = vaultwarden_organization.test.id
  name            = "Collection"
}

resource "vaultwarden_organization_user" "a" {
  organization_id = vaultwarden_organization.test.id
  email           = %[7]q
  type            = %[8]q
}

resource "vaultwarden_organization_user" "b" {
  organization_id = vaultwarden_organization.test.id
  email           = %[9]q
}

resource "vaultwarden_organization_collection_user" "a" {
  organization_id      = vaultwarden_organization.test.id
  collection_id        = vaultwarden_organization_collection.test.id
  organization_user_id = vaultwarden_organization_user.a.id
  read_only            = true
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, firstName, aEmail, aType, bEmail)
	for _, member := range members {
		config += member
	}
	return config
}
