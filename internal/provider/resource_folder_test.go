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
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

func TestAccFolder(t *testing.T) {
	before := "Före/" + gofakeit.UUID()
	after := "After " + gofakeit.UUID()
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckFolderDestroyed(t, ids),
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: testAccFolderConfig(before),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("vaultwarden_folder.test", "id"),
					resource.TestCheckResourceAttr("vaultwarden_folder.test", "name", before),
					testAccCaptureFolderID(ids),
					testAccCheckFolderName(t, ids, before),
				),
			},
			// Rename
			{
				Config: testAccFolderConfig(after),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_folder.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_folder.test", "name", after),
					testAccCheckFolderName(t, ids, after),
				),
			},
			// Import
			{
				ResourceName:      "vaultwarden_folder.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Renamed outside Terraform: the name is set back
			{
				PreConfig: func() {
					ctx := context.Background()
					_, err := testAccClient(ctx, t).UpdateFolder(ctx, ids["folder"], "Changed")
					testAccNoError(t, err)
				},
				Config: testAccFolderConfig(after),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_folder.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: testAccCheckFolderName(t, ids, after),
			},
			// Deleted outside Terraform: it is recreated
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).DeleteFolder(ctx, ids["folder"]))
				},
				Config: testAccFolderConfig(after),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_folder.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureFolderID(ids),
					testAccCheckFolderName(t, ids, after),
				),
			},
		},
	})
}

func TestAccFolderDataSource(t *testing.T) {
	name := "Folder " + gofakeit.UUID()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccFolderConfig(name) + `
data "vaultwarden_folder" "by_id" {
  id = vaultwarden_folder.test.id
}

data "vaultwarden_folder" "by_name" {
  name = vaultwarden_folder.test.name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.vaultwarden_folder.by_id", "id", "vaultwarden_folder.test", "id"),
					resource.TestCheckResourceAttr("data.vaultwarden_folder.by_id", "name", name),
					resource.TestCheckResourceAttrPair("data.vaultwarden_folder.by_name", "id", "vaultwarden_folder.test", "id"),
				),
			},
			{
				Config: testAccFolderConfig(name) + `
data "vaultwarden_folder" "missing" {
  name = "Missing ` + name + `"

  depends_on = [vaultwarden_folder.test]
}
`,
				ExpectError: regexp.MustCompile(`Folder Not Found`),
			},
			{
				Config: testAccFolderConfig(name) + `
data "vaultwarden_folder" "missing" {
  id = "` + gofakeit.UUID() + `"

  depends_on = [vaultwarden_folder.test]
}
`,
				ExpectError: regexp.MustCompile(`Folder Not Found`),
			},
			{
				Config: testAccFolderConfig(name) + `
data "vaultwarden_folder" "invalid" {
  id = "not-a-uuid"

  depends_on = [vaultwarden_folder.test]
}
`,
				ExpectError: regexp.MustCompile(`Folder Not Found`),
			},
			{
				Config: testAccFolderConfig(name) + `
resource "vaultwarden_folder" "other" {
  name = vaultwarden_folder.test.name
}

data "vaultwarden_folder" "ambiguous" {
  name = vaultwarden_folder.test.name

  depends_on = [vaultwarden_folder.other]
}
`,
				ExpectError: regexp.MustCompile(`Multiple Folders Found`),
			},
		},
	})
}

// testAccCaptureFolderID stores the ID of vaultwarden_folder.test.
func testAccCaptureFolderID(ids map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ids["folder"] = s.RootModule().Resources["vaultwarden_folder.test"].Primary.ID
		return nil
	}
}

// testAccCheckFolderName checks the folder name as decrypted by a separate client, which
// logs in on its own and so decrypts it with the user key from its own login.
func testAccCheckFolderName(t *testing.T, ids map[string]string, name string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		folder, err := testAccClient(ctx, t).GetFolder(ctx, ids["folder"])
		if err != nil {
			return err
		}
		if folder.Name != name {
			return fmt.Errorf("folder %s has name %q, expected %q", ids["folder"], folder.Name, name)
		}
		return nil
	}
}

// testAccCheckFolderDestroyed checks that the last folder of the test is deleted.
func testAccCheckFolderDestroyed(t *testing.T, ids map[string]string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		if _, err := testAccClient(ctx, t).GetFolder(ctx, ids["folder"]); !vaultwarden.IsNotFound(err) {
			return fmt.Errorf("folder %s was not deleted: %v", ids["folder"], err)
		}
		return nil
	}
}

func testAccFolderConfig(name string) string {
	return fmt.Sprintf(`
provider "vaultwarden" {
  endpoint        = %[1]q
  email           = %[2]q
  master_password = %[3]q
}

resource "vaultwarden_folder" "test" {
  name = %[4]q
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, name)
}
