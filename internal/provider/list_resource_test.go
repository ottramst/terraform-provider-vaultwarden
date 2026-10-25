package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

// TestAccListResources lists objects created in the first step with terraform query.
func TestAccListResources(t *testing.T) {
	// Users are listed from the whole server: a unique tag in their emails keeps other
	// tests' users out of the results. Inviting a member to the organization also creates
	// an account for them, so the filter only matches the "-user-" emails
	tag := strings.ToLower(gofakeit.LetterN(12))
	orgName := gofakeit.Company()
	emails := map[string]string{
		"member_a": "list-" + tag + "-member-a@example.com",
		"member_b": "list-" + tag + "-member-b@example.com",
		"user_a":   "list-" + tag + "-user-a@example.com",
		"user_b":   "list-" + tag + "-user-b@example.com",
	}
	ids := map[string]string{}

	// id returns the ID captured in the first step for a resource address.
	id := func(address string) knownvalue.Check {
		return stringExactFunc(func() string { return ids[address] })
	}
	orgIdentity := func(address string) map[string]knownvalue.Check {
		return map[string]knownvalue.Check{
			"organization_id": id("vaultwarden_organization.test"),
			"id":              id(address),
		}
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccListResourcesConfig(orgName, emails),
				Check: func(s *terraform.State) error {
					for address, rs := range s.RootModule().Resources {
						ids[address] = rs.Primary.ID
					}
					return nil
				},
			},
			{
				Query:  true,
				Config: testAccListResourcesQuery(tag),
				QueryResultChecks: []querycheck.QueryResultCheck{
					// Members: the provider user (owner) and the two invited members
					querycheck.ExpectLength("vaultwarden_organization_user.all", 3),
					querycheck.ExpectIdentity("vaultwarden_organization_user.all", orgIdentity("vaultwarden_organization_user.a")),
					querycheck.ExpectIdentity("vaultwarden_organization_user.all", orgIdentity("vaultwarden_organization_user.b")),
					querycheck.ExpectResourceDisplayName("vaultwarden_organization_user.all",
						queryfilter.ByResourceIdentity(orgIdentity("vaultwarden_organization_user.a")), knownvalue.StringExact(emails["member_a"])),
					querycheck.ExpectLength("vaultwarden_organization_user.filtered", 1),
					querycheck.ExpectResourceKnownValues("vaultwarden_organization_user.filtered",
						queryfilter.ByResourceIdentity(orgIdentity("vaultwarden_organization_user.b")), []querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("email"), KnownValue: knownvalue.StringExact(emails["member_b"])},
							{Path: tfjsonpath.New("organization_id"), KnownValue: id("vaultwarden_organization.test")},
							{Path: tfjsonpath.New("type"), KnownValue: knownvalue.StringExact("Manager")},
							{Path: tfjsonpath.New("access_all"), KnownValue: knownvalue.Bool(true)},
							{Path: tfjsonpath.New("status"), KnownValue: knownvalue.StringExact("Invited")},
							{Path: tfjsonpath.New("revoked"), KnownValue: knownvalue.Bool(false)},
							{Path: tfjsonpath.New("auto_confirm"), KnownValue: knownvalue.Bool(false)},
						}),

					// Collections: Vaultwarden also creates a "Default Collection" with the organization
					querycheck.ExpectLength("vaultwarden_organization_collection.all", 3),
					querycheck.ExpectIdentity("vaultwarden_organization_collection.all", orgIdentity("vaultwarden_organization_collection.a")),
					querycheck.ExpectIdentity("vaultwarden_organization_collection.all", orgIdentity("vaultwarden_organization_collection.b")),
					querycheck.ExpectResourceDisplayName("vaultwarden_organization_collection.all",
						queryfilter.ByResourceIdentity(orgIdentity("vaultwarden_organization_collection.a")), knownvalue.StringExact("Alpha collection")),
					querycheck.ExpectLength("vaultwarden_organization_collection.filtered", 1),
					querycheck.ExpectResourceKnownValues("vaultwarden_organization_collection.filtered",
						queryfilter.ByResourceIdentity(orgIdentity("vaultwarden_organization_collection.b")), []querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact("Beta collection")},
							{Path: tfjsonpath.New("external_id"), KnownValue: knownvalue.StringExact("beta")},
						}),

					// Groups
					querycheck.ExpectLength("vaultwarden_organization_group.all", 2),
					querycheck.ExpectIdentity("vaultwarden_organization_group.all", orgIdentity("vaultwarden_organization_group.a")),
					querycheck.ExpectIdentity("vaultwarden_organization_group.all", orgIdentity("vaultwarden_organization_group.b")),
					querycheck.ExpectResourceDisplayName("vaultwarden_organization_group.all",
						queryfilter.ByResourceIdentity(orgIdentity("vaultwarden_organization_group.a")), knownvalue.StringExact("Alpha group")),
					querycheck.ExpectLength("vaultwarden_organization_group.filtered", 1),
					querycheck.ExpectResourceKnownValues("vaultwarden_organization_group.filtered",
						queryfilter.ByResourceIdentity(orgIdentity("vaultwarden_organization_group.b")), []querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact("Beta group")},
							{Path: tfjsonpath.New("organization_id"), KnownValue: id("vaultwarden_organization.test")},
							{Path: tfjsonpath.New("external_id"), KnownValue: knownvalue.StringExact("beta")},
						}),

					// Users
					querycheck.ExpectLength("vaultwarden_user.tagged", 2),
					querycheck.ExpectIdentity("vaultwarden_user.tagged", map[string]knownvalue.Check{"id": id("vaultwarden_user.a")}),
					querycheck.ExpectIdentity("vaultwarden_user.tagged", map[string]knownvalue.Check{"id": id("vaultwarden_user.b")}),
					querycheck.ExpectResourceKnownValues("vaultwarden_user.tagged",
						queryfilter.ByDisplayName(knownvalue.StringExact(emails["user_b"])), []querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("id"), KnownValue: id("vaultwarden_user.b")},
							{Path: tfjsonpath.New("email"), KnownValue: knownvalue.StringExact(emails["user_b"])},
							{Path: tfjsonpath.New("enabled"), KnownValue: knownvalue.Bool(false)},
						}),
				},
			},
			// An organization the provider user is not a member of
			{
				Query: true,
				Config: `
list "vaultwarden_organization_collection" "missing" {
  provider = vaultwarden
  config {
    organization_id = "00000000-0000-0000-0000-000000000000"
  }
}
`,
				ExpectError: regexp.MustCompile(`Organization Not Found`),
			},
		},
	})
}

// stringExactFunc checks that a value is the string returned by the function, which is called
// when the check runs: for values only known after an earlier test step.
type stringExactFunc func() string

func (f stringExactFunc) CheckValue(value any) error {
	got, ok := value.(string)
	if !ok {
		return fmt.Errorf("expected string value for StringExact check, got: %T", value)
	}
	if got != f() {
		return fmt.Errorf("expected value %s for StringExact check, got: %s", f(), got)
	}
	return nil
}

func (f stringExactFunc) String() string {
	return f()
}

func testAccListResourcesConfig(orgName string, emails map[string]string) string {
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

resource "vaultwarden_organization_user" "a" {
  organization_id = vaultwarden_organization.test.id
  email           = %[6]q
}

resource "vaultwarden_organization_user" "b" {
  organization_id = vaultwarden_organization.test.id
  email           = %[7]q
  type            = "Manager"
  access_all      = true
}

resource "vaultwarden_organization_collection" "a" {
  organization_id = vaultwarden_organization.test.id
  name            = "Alpha collection"
}

resource "vaultwarden_organization_collection" "b" {
  organization_id = vaultwarden_organization.test.id
  name            = "Beta collection"
  external_id     = "beta"
}

resource "vaultwarden_organization_group" "a" {
  organization_id = vaultwarden_organization.test.id
  name            = "Alpha group"
}

resource "vaultwarden_organization_group" "b" {
  organization_id = vaultwarden_organization.test.id
  name            = "Beta group"
  external_id     = "beta"
}

resource "vaultwarden_user" "a" {
  email = %[8]q
}

resource "vaultwarden_user" "b" {
  email   = %[9]q
  enabled = false
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName,
		emails["member_a"], emails["member_b"], emails["user_a"], emails["user_b"])
}

// testAccListResourcesQuery lists the objects of testAccListResourcesConfig. The filters use
// another case than the names, since they ignore case.
func testAccListResourcesQuery(tag string) string {
	return fmt.Sprintf(`
list "vaultwarden_organization_user" "all" {
  provider = vaultwarden
  config {
    organization_id = vaultwarden_organization.test.id
  }
}

list "vaultwarden_organization_user" "filtered" {
  provider         = vaultwarden
  include_resource = true
  config {
    organization_id = vaultwarden_organization.test.id
    email_contains  = "MEMBER-B"
  }
}

list "vaultwarden_organization_collection" "all" {
  provider = vaultwarden
  config {
    organization_id = vaultwarden_organization.test.id
  }
}

list "vaultwarden_organization_collection" "filtered" {
  provider         = vaultwarden
  include_resource = true
  config {
    organization_id = vaultwarden_organization.test.id
    name_contains   = "BETA"
  }
}

list "vaultwarden_organization_group" "all" {
  provider = vaultwarden
  config {
    organization_id = vaultwarden_organization.test.id
  }
}

list "vaultwarden_organization_group" "filtered" {
  provider         = vaultwarden
  include_resource = true
  config {
    organization_id = vaultwarden_organization.test.id
    name_contains   = "BETA"
  }
}

list "vaultwarden_user" "tagged" {
  provider         = vaultwarden
  include_resource = true
  config {
    email_contains = %[1]q
  }
}
`, strings.ToUpper(tag+"-user"))
}
