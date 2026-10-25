package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

// Regression test for https://github.com/ottramst/terraform-provider-vaultwarden/issues/44
func TestAccResourcesDeletedOutsideTerraform(t *testing.T) {
	orgName := gofakeit.Company()
	memberEmail := gofakeit.Email()
	userEmail := gofakeit.Email()
	accountEmail := gofakeit.Email()
	password := gofakeit.Password(true, true, true, true, false, 16)
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccLifecycleConfig(orgName, memberEmail, userEmail, accountEmail, password),
				Check:  testAccCaptureIDs(ids),
			},
			// Delete everything but the organization outside Terraform: it is recreated
			{
				PreConfig: func() {
					ctx := context.Background()
					client := testAccClient(ctx, t)
					testAccNoError(t, client.DeleteOrganizationCollection(ctx, ids["organization"], ids["collection"]))
					testAccNoError(t, client.DeleteOrganizationUser(ctx, ids["member"], ids["organization"]))
					testAccNoError(t, client.DeleteUser(ctx, ids["user"]))
					testAccNoError(t, client.DeleteUser(ctx, ids["account"]))
				},
				Config: testAccLifecycleConfig(orgName, memberEmail, userEmail, accountEmail, password),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_collection.test", plancheck.ResourceActionCreate),
						plancheck.ExpectResourceAction("vaultwarden_organization_user.test", plancheck.ResourceActionCreate),
						plancheck.ExpectResourceAction("vaultwarden_user.test", plancheck.ResourceActionCreate),
						plancheck.ExpectResourceAction("vaultwarden_account_register.test", plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCaptureIDs(ids),
			},
			// Delete the organization outside Terraform: it and its children are recreated
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).DeleteOrganization(ctx, ids["organization"]))
				},
				Config: testAccLifecycleConfig(orgName, memberEmail, userEmail, accountEmail, password),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization.test", plancheck.ResourceActionCreate),
						plancheck.ExpectResourceAction("vaultwarden_organization_collection.test", plancheck.ResourceActionCreate),
						plancheck.ExpectResourceAction("vaultwarden_organization_user.test", plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

// Regression test for https://github.com/ottramst/terraform-provider-vaultwarden/issues/45
func TestAccUpdatesPreserveCollectionAccess(t *testing.T) {
	orgName := gofakeit.Company()
	memberEmail := gofakeit.Email()
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCollectionAccessConfig(orgName, "Before", memberEmail, "User"),
				Check:  testAccCaptureIDs(ids),
			},
			// Grant access outside Terraform, then update the member
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).SetOrganizationCollectionAccess(ctx, ids["organization"], ids["collection"], models.CollectionAccessDetails{
						Users: []models.CollectionAccess{{ID: ids["member"], ReadOnly: true}},
					}))
				},
				Config: testAccCollectionAccessConfig(orgName, "Before", memberEmail, "Manager"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "type", "Manager"),
					testAccCheckCollectionAccessPreserved(t, ids),
				),
			},
			// Then update the collection
			{
				Config: testAccCollectionAccessConfig(orgName, "After", memberEmail, "Manager"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_collection.test", "name", "After"),
					testAccCheckCollectionAccessPreserved(t, ids),
				),
			},
		},
	})
}

// testAccCheckCollectionAccessPreserved checks that the member still has read-only access to the collection.
func testAccCheckCollectionAccessPreserved(t *testing.T, ids map[string]string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		access, err := testAccClient(ctx, t).GetOrganizationCollectionAccess(ctx, ids["organization"], ids["collection"])
		if err != nil {
			return err
		}
		for _, user := range access.Users {
			if user.ID == ids["member"] && user.ReadOnly {
				return nil
			}
		}
		return fmt.Errorf("read-only access of member %s to collection %s was not preserved: %+v", ids["member"], ids["collection"], access.Users)
	}
}

// testAccClient returns the shared test client.
func testAccClient(ctx context.Context, t *testing.T) *vaultwarden.Client {
	client, err := test.GetTestClient(ctx, t)
	if err != nil {
		t.Fatalf("failed to get test client: %v", err)
	}
	return client
}

func testAccNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// testAccCaptureIDs stores the IDs of the resources in testAccLifecycleConfig.
func testAccCaptureIDs(ids map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for key, address := range map[string]string{
			"organization": "vaultwarden_organization.test",
			"collection":   "vaultwarden_organization_collection.test",
			"member":       "vaultwarden_organization_user.test",
			"user":         "vaultwarden_user.test",
			"account":      "vaultwarden_account_register.test",
		} {
			if rs, ok := s.RootModule().Resources[address]; ok {
				ids[key] = rs.Primary.ID
			}
		}
		return nil
	}
}

func testAccLifecycleConfig(orgName, memberEmail, userEmail, accountEmail, password string) string {
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

resource "vaultwarden_organization_user" "test" {
  organization_id = vaultwarden_organization.test.id
  email           = %[6]q
}

resource "vaultwarden_user" "test" {
  email = %[7]q
}

resource "vaultwarden_account_register" "test" {
  email    = %[8]q
  password = %[9]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, memberEmail, userEmail, accountEmail, password)
}

func testAccCollectionAccessConfig(orgName, collectionName, memberEmail, memberType string) string {
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
  name            = %[6]q
}

resource "vaultwarden_organization_user" "test" {
  organization_id = vaultwarden_organization.test.id
  email           = %[7]q
  type            = %[8]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, collectionName, memberEmail, memberType)
}
