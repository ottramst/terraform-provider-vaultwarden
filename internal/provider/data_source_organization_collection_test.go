package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

func TestAccOrganizationCollectionDataSource(t *testing.T) {
	orgName := gofakeit.Company()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccOrganizationCollectionDataSourceConfig(orgName, `
data "vaultwarden_organization_collection" "by_name" {
  organization_id = vaultwarden_organization.test.id
  name            = "Alpha"
  depends_on      = [vaultwarden_organization_collection.alpha, vaultwarden_organization_collection.beta]
}

data "vaultwarden_organization_collection" "by_id" {
  organization_id = vaultwarden_organization.test.id
  id              = vaultwarden_organization_collection.beta.id
}

data "vaultwarden_organization_collections" "test" {
  organization_id = vaultwarden_organization.test.id
  depends_on = [
    vaultwarden_organization_collection.alpha,
    vaultwarden_organization_collection.beta,
    vaultwarden_organization_collection.duplicate_1,
    vaultwarden_organization_collection.duplicate_2,
  ]
}
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Lookup by name
					resource.TestCheckResourceAttrPair("data.vaultwarden_organization_collection.by_name", "id", "vaultwarden_organization_collection.alpha", "id"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_collection.by_name", "name", "Alpha"),
					resource.TestCheckNoResourceAttr("data.vaultwarden_organization_collection.by_name", "external_id"),
					// Lookup by ID
					resource.TestCheckResourceAttrPair("data.vaultwarden_organization_collection.by_id", "id", "vaultwarden_organization_collection.beta", "id"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_collection.by_id", "name", "Beta"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_collection.by_id", "external_id", "beta-external"),
					// List, sorted by name, including the default collection of the organization
					resource.TestCheckResourceAttr("data.vaultwarden_organization_collections.test", "collections.#", "5"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_collections.test", "collections.0.name", "Alpha"),
					resource.TestCheckResourceAttrPair("data.vaultwarden_organization_collections.test", "collections.0.id", "vaultwarden_organization_collection.alpha", "id"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_collections.test", "collections.1.name", "Beta"),
					resource.TestCheckResourceAttrPair("data.vaultwarden_organization_collections.test", "collections.1.id", "vaultwarden_organization_collection.beta", "id"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_collections.test", "collections.1.external_id", "beta-external"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_collections.test", "collections.2.name", "Duplicate"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_collections.test", "collections.3.name", "Duplicate"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_collections.test", "collections.4.name", "Gamma"),
					resource.TestCheckTypeSetElemAttrPair("data.vaultwarden_organization_collections.test", "collections.*.id", "vaultwarden_organization_collection.duplicate_1", "id"),
					resource.TestCheckTypeSetElemAttrPair("data.vaultwarden_organization_collections.test", "collections.*.id", "vaultwarden_organization_collection.duplicate_2", "id"),
				),
			},
			// Two collections with the name
			{
				Config: testAccOrganizationCollectionDataSourceConfig(orgName, `
data "vaultwarden_organization_collection" "duplicate" {
  organization_id = vaultwarden_organization.test.id
  name            = "Duplicate"
}
`),
				ExpectError: regexp.MustCompile(`(?s)Multiple Organization Collections Found.*Look the collection up by\s+` + "`id`"),
			},
			// No collection with the name
			{
				Config: testAccOrganizationCollectionDataSourceConfig(orgName, `
data "vaultwarden_organization_collection" "missing" {
  organization_id = vaultwarden_organization.test.id
  name            = "Missing"
}
`),
				ExpectError: regexp.MustCompile(`Organization Collection Not Found`),
			},
			// No collection with the ID
			{
				Config: testAccOrganizationCollectionDataSourceConfig(orgName, `
data "vaultwarden_organization_collection" "missing" {
  organization_id = vaultwarden_organization.test.id
  id              = "00000000-0000-0000-0000-000000000000"
}
`),
				ExpectError: regexp.MustCompile(`(?s)has no collection with ID\s+"00000000-0000-0000-0000-000000000000"`),
			},
			// Exactly one of id and name
			{
				Config: testAccOrganizationCollectionDataSourceConfig(orgName, `
data "vaultwarden_organization_collection" "none" {
  organization_id = vaultwarden_organization.test.id
}
`),
				ExpectError: regexp.MustCompile(`(Invalid Attribute Combination|Missing Attribute Configuration)`),
			},
			// Unknown organization
			{
				Config: testAccOrganizationCollectionDataSourceConfig(orgName, `
data "vaultwarden_organization_collections" "missing" {
  organization_id = "00000000-0000-0000-0000-000000000000"
}
`),
				ExpectError: regexp.MustCompile(`Organization Not Found`),
			},
			// A valid configuration last, for the destroy
			{
				Config: testAccOrganizationCollectionDataSourceConfig(orgName, ""),
			},
		},
	})
}

func TestAccOrganizationsDataSource(t *testing.T) {
	firstName := gofakeit.Company()
	secondName := gofakeit.Company()

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

resource "vaultwarden_organization" "first" {
  name = %[4]q
}

resource "vaultwarden_organization" "second" {
  name = %[5]q
}

data "vaultwarden_organization_user" "owner" {
  organization_id = vaultwarden_organization.first.id
  email           = %[2]q
}

data "vaultwarden_organizations" "test" {
  depends_on = [vaultwarden_organization.first, vaultwarden_organization.second]
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, firstName, secondName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckTypeSetElemNestedAttrs("data.vaultwarden_organizations.test", "organizations.*", map[string]string{
						"name": firstName,
						"type": "Owner",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.vaultwarden_organizations.test", "organizations.*", map[string]string{
						"name": secondName,
						"type": "Owner",
					}),
					resource.TestCheckTypeSetElemAttrPair("data.vaultwarden_organizations.test", "organizations.*.id", "vaultwarden_organization.first", "id"),
					resource.TestCheckTypeSetElemAttrPair("data.vaultwarden_organizations.test", "organizations.*.id", "vaultwarden_organization.second", "id"),
					resource.TestCheckTypeSetElemAttrPair("data.vaultwarden_organizations.test", "organizations.*.organization_user_id", "data.vaultwarden_organization_user.owner", "id"),
				),
			},
		},
	})
}

func testAccOrganizationCollectionDataSourceConfig(orgName, dataSources string) string {
	return fmt.Sprintf(`
provider "vaultwarden" {
  endpoint        = %[1]q
  email           = %[2]q
  master_password = %[3]q
}

resource "vaultwarden_organization" "test" {
  name            = %[4]q
  collection_name = "Gamma"
}

resource "vaultwarden_organization_collection" "alpha" {
  organization_id = vaultwarden_organization.test.id
  name            = "Alpha"
}

resource "vaultwarden_organization_collection" "beta" {
  organization_id = vaultwarden_organization.test.id
  name            = "Beta"
  external_id     = "beta-external"
}

resource "vaultwarden_organization_collection" "duplicate_1" {
  organization_id = vaultwarden_organization.test.id
  name            = "Duplicate"
}

resource "vaultwarden_organization_collection" "duplicate_2" {
  organization_id = vaultwarden_organization.test.id
  name            = "Duplicate"
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, orgName) + dataSources
}
