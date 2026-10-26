package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
)

func TestAccItemAttachment(t *testing.T) {
	ids := map[string]string{}
	binary := []byte{0, 1, 2, 3, 255, 254, 253}
	file := filepath.Join(t.TempDir(), "hello.txt")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckItemAttachmentDestroyed(t, ids),
		Steps: []resource.TestStep{
			// Create from content
			{
				Config: testAccItemAttachmentConfig(`
  content   = "hello"
  file_name = "hello.txt"
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("vaultwarden_item_attachment.test", "id"),
					resource.TestCheckResourceAttrPair("vaultwarden_item_attachment.test", "item_id", "vaultwarden_item_secure_note.test", "id"),
					resource.TestCheckResourceAttr("vaultwarden_item_attachment.test", "file_name", "hello.txt"),
					resource.TestCheckResourceAttr("vaultwarden_item_attachment.test", "content_sha256", testAccSHA256([]byte("hello"))),
					resource.TestMatchResourceAttr("vaultwarden_item_attachment.test", "size", regexp.MustCompile(`^[1-9]\d*$`)),
					resource.TestCheckResourceAttrSet("vaultwarden_item_attachment.test", "size_name"),
					testAccCaptureItemAttachmentIDs(ids),
					testAccCheckItemAttachmentContent(t, ids, "hello.txt", []byte("hello")),
				),
			},
			// A different content replaces the attachment
			{
				Config: testAccItemAttachmentConfig(`
  content   = "hello, world"
  file_name = "hello.txt"
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_attachment.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckItemAttachmentReplaced(ids),
					testAccCaptureItemAttachmentIDs(ids),
					testAccCheckItemAttachmentContent(t, ids, "hello.txt", []byte("hello, world")),
				),
			},
			// The same content from a file with the same name keeps the attachment
			{
				PreConfig: func() { testAccNoError(t, os.WriteFile(file, []byte("hello, world"), 0o600)) },
				Config:    testAccItemAttachmentConfig(fmt.Sprintf("  file = %q\n", file)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_attachment.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_item_attachment.test", "file_name", "hello.txt"),
					testAccCheckItemAttachmentUnchanged(ids),
				),
			},
			// A changed file replaces the attachment
			{
				PreConfig: func() { testAccNoError(t, os.WriteFile(file, []byte("changed file"), 0o600)) },
				Config:    testAccItemAttachmentConfig(fmt.Sprintf("  file = %q\n", file)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_attachment.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckItemAttachmentReplaced(ids),
					testAccCaptureItemAttachmentIDs(ids),
					resource.TestCheckResourceAttr("vaultwarden_item_attachment.test", "content_sha256", testAccSHA256([]byte("changed file"))),
					testAccCheckItemAttachmentContent(t, ids, "hello.txt", []byte("changed file")),
				),
			},
			// A different file name replaces the attachment
			{
				Config: testAccItemAttachmentConfig(fmt.Sprintf("  file = %q\n  file_name = \"renamed.txt\"\n", file)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_attachment.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckItemAttachmentReplaced(ids),
					testAccCaptureItemAttachmentIDs(ids),
					testAccCheckItemAttachmentContent(t, ids, "renamed.txt", []byte("changed file")),
				),
			},
			// Import, which reads the hash of the content from the server
			{
				ResourceName:            "vaultwarden_item_attachment.test",
				ImportState:             true,
				ImportStateIdFunc:       func(*terraform.State) (string, error) { return ids["item"] + "/" + ids["attachment"], nil },
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"file"},
			},
			// Binary content
			{
				Config: testAccItemAttachmentConfig(fmt.Sprintf(`
  content_base64 = %q
  file_name      = "binary.dat"
`, base64.StdEncoding.EncodeToString(binary))),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureItemAttachmentIDs(ids),
					resource.TestCheckResourceAttr("vaultwarden_item_attachment.test", "content_sha256", testAccSHA256(binary)),
					testAccCheckItemAttachmentContent(t, ids, "binary.dat", binary),
				),
			},
			// Updating the item keeps its attachment
			{
				Config: testAccItemAttachmentConfigWithNotes("Updated notes", fmt.Sprintf(`
  content_base64 = %q
  file_name      = "binary.dat"
`, base64.StdEncoding.EncodeToString(binary))),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_secure_note.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("vaultwarden_item_attachment.test", plancheck.ResourceActionNoop),
					},
				},
				Check: testAccCheckItemAttachmentContent(t, ids, "binary.dat", binary),
			},
			// Deleted outside Terraform: it is created again
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).DeleteAttachment(ctx, ids["item"], ids["attachment"]))
				},
				Config: testAccItemAttachmentConfigWithNotes("Updated notes", fmt.Sprintf(`
  content_base64 = %q
  file_name      = "binary.dat"
`, base64.StdEncoding.EncodeToString(binary))),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_attachment.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureItemAttachmentIDs(ids),
					testAccCheckItemAttachmentContent(t, ids, "binary.dat", binary),
				),
			},
			// Its item deleted outside Terraform: both are created again
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).DeleteItem(ctx, ids["item"]))
				},
				Config: testAccItemAttachmentConfigWithNotes("Updated notes", fmt.Sprintf(`
  content_base64 = %q
  file_name      = "binary.dat"
`, base64.StdEncoding.EncodeToString(binary))),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_item_secure_note.test", plancheck.ResourceActionCreate),
						plancheck.ExpectResourceAction("vaultwarden_item_attachment.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureItemAttachmentIDs(ids),
					testAccCheckItemAttachmentContent(t, ids, "binary.dat", binary),
				),
			},
		},
	})
}

func TestAccItemAttachmentValidation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccItemAttachmentConfig(`  content = "no file name"` + "\n"),
				ExpectError: regexp.MustCompile(`(?s)Attribute "file_name" must be specified`),
			},
			{
				Config:      testAccItemAttachmentConfig(`  file_name = "nothing.txt"` + "\n"),
				ExpectError: regexp.MustCompile(`(?s)No attribute specified`),
			},
			{
				Config:      testAccItemAttachmentConfig("  content_base64 = \"not base64!\"\n  file_name = \"a.bin\"\n"),
				ExpectError: regexp.MustCompile(`Invalid base64 content`),
			},
			{
				Config:      testAccItemAttachmentConfig(fmt.Sprintf("  file = %q\n", filepath.Join(t.TempDir(), "missing.txt"))),
				ExpectError: regexp.MustCompile(`Error reading attachment file`),
			},
		},
	})
}

func TestAccItemAttachmentOrganization(t *testing.T) {
	orgName := gofakeit.Company()
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckItemAttachmentDestroyed(t, ids),
		Steps: []resource.TestStep{
			{
				Config: testAccItemOrganizationConfig(orgName, `
  organization_id = vaultwarden_organization.test.id
  collection_ids  = [vaultwarden_organization_collection.a.id]
  name            = "Shared with attachment"
`) + `
resource "vaultwarden_item_attachment" "test" {
  item_id   = vaultwarden_item_login.test.id
  content   = "shared attachment"
  file_name = "shared.txt"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("vaultwarden_item_attachment.test", "item_id", "vaultwarden_item_login.test", "id"),
					testAccCaptureItemAttachmentIDs(ids),
					testAccCheckItemAttachmentContent(t, ids, "shared.txt", []byte("shared attachment")),
				),
			},
		},
	})
}

// The ephemeral resource returns the content of an attachment. Used elsewhere, the content does
// not end up in state.
func TestAccItemAttachmentEphemeral(t *testing.T) {
	file := filepath.Join(t.TempDir(), "secret.txt")
	secret := "attachment secret " + gofakeit.UUID()
	testAccNoError(t, os.WriteFile(file, []byte(secret), 0o600))
	attachment := testAccItemAttachmentConfig(fmt.Sprintf("  file = %q\n", file)) + `
ephemeral "vaultwarden_item_attachment" "test" {
  item_id = vaultwarden_item_attachment.test.item_id
  id      = vaultwarden_item_attachment.test.id
}
`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccEchoProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		Steps: []resource.TestStep{
			{
				Config: attachment + `
provider "echo" {
  data = ephemeral.vaultwarden_item_attachment.test
}

resource "echo" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("echo.test", "data.file_name", "secret.txt"),
					resource.TestCheckResourceAttr("echo.test", "data.content", secret),
					resource.TestCheckResourceAttr("echo.test", "data.content_base64", base64.StdEncoding.EncodeToString([]byte(secret))),
					resource.TestCheckResourceAttrPair("echo.test", "data.size", "vaultwarden_item_attachment.test", "size"),
					resource.TestCheckResourceAttrPair("echo.test", "data.size_name", "vaultwarden_item_attachment.test", "size_name"),
				),
			},
			// Without the echo provider, the content is not stored anywhere in state
			{
				Config: attachment + `
data "vaultwarden_folder" "test" {
  id = vaultwarden_folder.test.id

  lifecycle {
    postcondition {
      condition     = ephemeral.vaultwarden_item_attachment.test.content == file("` + file + `")
      error_message = "The ephemeral resource did not return the content of the attachment."
    }
  }
}
`,
				Check: testAccCheckSecretNotInState(secret),
			},
		},
	})
}

func testAccSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// testAccCaptureItemAttachmentIDs stores the IDs of the attachment and its item as
// ids["attachment"] and ids["item"], and the previous attachment ID as ids["previous"].
func testAccCaptureItemAttachmentIDs(ids map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		attributes := s.RootModule().Resources["vaultwarden_item_attachment.test"].Primary.Attributes
		ids["previous"] = ids["attachment"]
		ids["attachment"] = attributes["id"]
		ids["item"] = attributes["item_id"]
		return nil
	}
}

// testAccCheckItemAttachmentReplaced checks that the attachment has a new ID.
func testAccCheckItemAttachmentReplaced(ids map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if id := s.RootModule().Resources["vaultwarden_item_attachment.test"].Primary.ID; id == ids["attachment"] {
			return fmt.Errorf("attachment %s was not replaced", id)
		}
		return nil
	}
}

// testAccCheckItemAttachmentUnchanged checks that the attachment kept its ID.
func testAccCheckItemAttachmentUnchanged(ids map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if id := s.RootModule().Resources["vaultwarden_item_attachment.test"].Primary.ID; id != ids["attachment"] {
			return fmt.Errorf("attachment %s was replaced by %s", ids["attachment"], id)
		}
		return nil
	}
}

// testAccCheckItemAttachmentContent checks the attachment ids["attachment"] as read and
// decrypted by the test client, and that the previous attachment was deleted.
func testAccCheckItemAttachmentContent(t *testing.T, ids map[string]string, fileName string, content []byte) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		client := testAccClient(ctx, t)
		attachment, err := client.GetAttachment(ctx, ids["item"], ids["attachment"])
		if err != nil {
			return err
		}
		if attachment.FileName != fileName {
			return fmt.Errorf("file name is %q, want %q", attachment.FileName, fileName)
		}
		got, err := client.GetAttachmentContent(ctx, ids["item"], ids["attachment"])
		if err != nil {
			return err
		}
		if !bytes.Equal(got, content) {
			return fmt.Errorf("content is %q, want %q", got, content)
		}
		if ids["previous"] != "" && ids["previous"] != ids["attachment"] {
			if _, err := client.GetAttachment(ctx, ids["item"], ids["previous"]); !vaultwarden.IsNotFound(err) {
				return fmt.Errorf("previous attachment %s was not deleted: %v", ids["previous"], err)
			}
		}
		return nil
	}
}

// testAccCheckItemAttachmentDestroyed checks that the last attachment of the test is deleted.
func testAccCheckItemAttachmentDestroyed(t *testing.T, ids map[string]string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		if _, err := testAccClient(ctx, t).GetAttachment(ctx, ids["item"], ids["attachment"]); !vaultwarden.IsNotFound(err) {
			return fmt.Errorf("attachment %s was not deleted: %v", ids["attachment"], err)
		}
		return nil
	}
}

func testAccItemAttachmentConfig(attributes string) string {
	return testAccItemAttachmentConfigWithNotes("Notes", attributes)
}

func testAccItemAttachmentConfigWithNotes(notes, attributes string) string {
	return testAccItemProviderConfig() + fmt.Sprintf(`
resource "vaultwarden_folder" "test" {
  name = "Attachments"
}

resource "vaultwarden_item_secure_note" "test" {
  folder_id = vaultwarden_folder.test.id
  name      = "With attachment"
  notes     = %q
}

resource "vaultwarden_item_attachment" "test" {
  item_id = vaultwarden_item_secure_note.test.id
`, notes) + attributes + `}
`
}
