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
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

func TestAccOrganizationGroup(t *testing.T) {
	orgName := gofakeit.Company()
	externalID := gofakeit.UUID()
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccOrganizationGroupConfig(orgName, `
  name        = "Before"
  access_all  = true
  external_id = "`+externalID+`"
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("vaultwarden_organization_group.test", "id"),
					resource.TestCheckResourceAttrPair("vaultwarden_organization_group.test", "organization_id", "vaultwarden_organization.test", "id"),
					resource.TestCheckResourceAttr("vaultwarden_organization_group.test", "name", "Before"),
					resource.TestCheckResourceAttr("vaultwarden_organization_group.test", "access_all", "true"),
					resource.TestCheckResourceAttr("vaultwarden_organization_group.test", "external_id", externalID),
					testAccCaptureGroupIDs(ids),
				),
			},
			// Grant the group access to a collection outside Terraform, then update the group:
			// the access is kept
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).SetOrganizationCollectionAccess(ctx, ids["org"], ids["collection"], models.CollectionAccessDetails{
						Groups: []models.CollectionAccess{{ID: ids["group"], ReadOnly: true}},
					}))
				},
				Config: testAccOrganizationGroupConfig(orgName, `
  name        = "After"
  external_id = "`+externalID+`"
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_group.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_group.test", "name", "After"),
					resource.TestCheckResourceAttr("vaultwarden_organization_group.test", "access_all", "false"),
					resource.TestCheckResourceAttr("vaultwarden_organization_group.test", "external_id", externalID),
					testAccCheckGroupCollectionAccessPreserved(t, ids),
				),
			},
			// Import
			{
				ResourceName:      "vaultwarden_organization_group.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return ids["org"] + "/" + ids["group"], nil
				},
			},
			// Deleted outside Terraform: it is recreated
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).DeleteOrganizationGroup(ctx, ids["org"], ids["group"]))
				},
				Config: testAccOrganizationGroupConfig(orgName, `
  name        = "After"
  external_id = "`+externalID+`"
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_group.test", plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureGroupIDs(ids),
			},
			// Vaultwarden cannot change the external ID: the group is replaced
			{
				Config: testAccOrganizationGroupConfig(orgName, `
  name = "After"
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_group.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("vaultwarden_organization_group.test", "external_id"),
				),
			},
		},
	})
}

func TestAccOrganizationGroupInvalidImportID(t *testing.T) {
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

resource "vaultwarden_organization_group" "test" {
  organization_id = "org"
  name            = "Group"
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword),
				ResourceName:  "vaultwarden_organization_group.test",
				ImportState:   true,
				ImportStateId: "org",
				ExpectError:   regexp.MustCompile(`Invalid ID format`),
			},
		},
	})
}

func TestAccOrganizationGroupDataSource(t *testing.T) {
	orgName := gofakeit.Company()
	externalID := gofakeit.UUID()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccOrganizationGroupConfig(orgName, `
  name        = "Engineering"
  external_id = "`+externalID+`"
`) + `
resource "vaultwarden_organization_group" "other" {
  organization_id = vaultwarden_organization.test.id
  name            = "Other"
}

data "vaultwarden_organization_group" "by_id" {
  organization_id = vaultwarden_organization.test.id
  id              = vaultwarden_organization_group.test.id
}

data "vaultwarden_organization_group" "by_name" {
  organization_id = vaultwarden_organization.test.id
  name            = "Engineering"

  depends_on = [vaultwarden_organization_group.test, vaultwarden_organization_group.other]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.vaultwarden_organization_group.by_id", "id", "vaultwarden_organization_group.test", "id"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_group.by_id", "name", "Engineering"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_group.by_id", "external_id", externalID),
					resource.TestCheckResourceAttrPair("data.vaultwarden_organization_group.by_name", "id", "vaultwarden_organization_group.test", "id"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_group.by_name", "external_id", externalID),
				),
			},
			{
				Config: testAccOrganizationGroupConfig(orgName, `
  name = "Engineering"
`) + `
data "vaultwarden_organization_group" "missing" {
  organization_id = vaultwarden_organization.test.id
  name            = "Missing"

  depends_on = [vaultwarden_organization_group.test]
}
`,
				ExpectError: regexp.MustCompile(`Organization Group Not Found`),
			},
			{
				Config: testAccOrganizationGroupConfig(orgName, `
  name = "Engineering"
`) + `
resource "vaultwarden_organization_group" "other" {
  organization_id = vaultwarden_organization.test.id
  name            = "Engineering"
}

data "vaultwarden_organization_group" "ambiguous" {
  organization_id = vaultwarden_organization.test.id
  name            = "Engineering"

  depends_on = [vaultwarden_organization_group.test, vaultwarden_organization_group.other]
}
`,
				ExpectError: regexp.MustCompile(`Multiple Organization Groups Found`),
			},
		},
	})
}

// testAccCaptureGroupIDs stores the IDs of the resources in testAccOrganizationGroupConfig.
func testAccCaptureGroupIDs(ids map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resources := s.RootModule().Resources
		ids["org"] = resources["vaultwarden_organization.test"].Primary.ID
		ids["collection"] = resources["vaultwarden_organization_collection.test"].Primary.ID
		ids["group"] = resources["vaultwarden_organization_group.test"].Primary.ID
		return nil
	}
}

// testAccCheckGroupCollectionAccessPreserved checks that the group still has read-only access to the collection.
func testAccCheckGroupCollectionAccessPreserved(t *testing.T, ids map[string]string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		access, err := testAccClient(ctx, t).GetOrganizationCollectionAccess(ctx, ids["org"], ids["collection"])
		if err != nil {
			return err
		}
		for _, group := range access.Groups {
			if group.ID == ids["group"] && group.ReadOnly {
				return nil
			}
		}
		return fmt.Errorf("read-only access of group %s to collection %s was not preserved: %+v", ids["group"], ids["collection"], access.Groups)
	}
}

func testAccOrganizationGroupConfig(orgName, groupAttributes string) string {
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

resource "vaultwarden_organization_collection" "test" {
  organization_id = vaultwarden_organization.test.id
  name            = "Collection"
}

resource "vaultwarden_organization_group" "test" {
  organization_id = vaultwarden_organization.test.id
%[6]s}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, groupAttributes)
}
