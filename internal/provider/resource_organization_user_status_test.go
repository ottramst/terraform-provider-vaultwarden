package provider

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/test"
)

// Members get into the Accepted status without the invitation email: the test server does not
// send mail, and Vaultwarden then accepts the invitation of an email when it registers an account,
// or at once when the account already exists. See test.RegisterAccount.

// Test for https://github.com/ottramst/terraform-provider-vaultwarden/issues/53
func TestAccOrganizationUserAutoConfirm(t *testing.T) {
	orgName := gofakeit.Company()
	email := gofakeit.Email()
	password := gofakeit.Password(true, true, true, true, false, 16)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// A member without an account stays invited, without a diff on the next plan
			{
				Config: testAccOrganizationUserStatusConfig(orgName, email, true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "auto_confirm", "true"),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "revoked", "false"),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "status", "Invited"),
				),
			},
			// Once the member accepted, the next apply confirms them
			{
				PreConfig: func() {
					_, err := test.RegisterAccount(context.Background(), email, password)
					testAccNoError(t, err)
				},
				Config: testAccOrganizationUserStatusConfig(orgName, email, true, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_user.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue("vaultwarden_organization_user.test", tfjsonpath.New("status")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "status", "Confirmed"),
					testAccCheckMemberHasOrganizationKey(email, password),
				),
			},
			// Revoke the confirmed member
			{
				Config: testAccOrganizationUserStatusConfig(orgName, email, true, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "revoked", "true"),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "status", "Revoked"),
				),
			},
			// Restoring brings back the confirmed status
			{
				Config: testAccOrganizationUserStatusConfig(orgName, email, true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "revoked", "false"),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "status", "Confirmed"),
				),
			},
			{
				ResourceName:            "vaultwarden_organization_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdFunc:       testAccOrganizationUserImportStateIdFunc(),
				ImportStateVerifyIgnore: []string{"auto_confirm"},
			},
		},
	})
}

func TestAccOrganizationUserAutoConfirmExistingAccount(t *testing.T) {
	orgName := gofakeit.Company()
	email := gofakeit.Email()
	password := gofakeit.Password(true, true, true, true, false, 16)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			_, err := test.RegisterAccount(context.Background(), email, password)
			testAccNoError(t, err)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// An existing account is accepted at once, so it is confirmed on create
			{
				Config: testAccOrganizationUserStatusConfig(orgName, email, true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "status", "Confirmed"),
					testAccCheckMemberHasOrganizationKey(email, password),
				),
			},
		},
	})
}

func TestAccOrganizationUserAutoConfirmOptIn(t *testing.T) {
	orgName := gofakeit.Company()
	email := gofakeit.Email()
	password := gofakeit.Password(true, true, true, true, false, 16)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			_, err := test.RegisterAccount(context.Background(), email, password)
			testAccNoError(t, err)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Without auto_confirm, an accepted member stays accepted, without a diff
			{
				Config: testAccOrganizationUserConfigBasic(orgName, email),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "auto_confirm", "false"),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "status", "Accepted"),
				),
			},
			// Enabling it confirms the member
			{
				Config: testAccOrganizationUserStatusConfig(orgName, email, true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "status", "Confirmed"),
					testAccCheckMemberHasOrganizationKey(email, password),
				),
			},
		},
	})
}

func TestAccOrganizationUserRevoked(t *testing.T) {
	orgName := gofakeit.Company()
	email := gofakeit.Email()
	ids := map[string]string{}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Invite a member as revoked
			{
				Config: testAccOrganizationUserStatusConfig(orgName, email, false, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "revoked", "true"),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "status", "Revoked"),
				),
			},
			// Restore them to invited
			{
				Config: testAccOrganizationUserStatusConfig(orgName, email, false, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "revoked", "false"),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "status", "Invited"),
					testAccCaptureIDs(ids),
				),
			},
			// A revocation outside Terraform is undone
			{
				PreConfig: func() {
					ctx := context.Background()
					testAccNoError(t, testAccClient(ctx, t).RevokeOrganizationUser(ctx, ids["member"], ids["organization"]))
				},
				Config: testAccOrganizationUserStatusConfig(orgName, email, false, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vaultwarden_organization_user.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "revoked", "false"),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "status", "Invited"),
				),
			},
			// Revoke them again, then change their role: they stay revoked
			{
				Config: testAccOrganizationUserStatusConfig(orgName, email, false, true),
				Check:  resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "status", "Revoked"),
			},
			{
				Config: testAccOrganizationUserStatusTypeConfig(orgName, email, "Admin", false, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "type", "Admin"),
					resource.TestCheckResourceAttr("vaultwarden_organization_user.test", "status", "Revoked"),
				),
			},
		},
	})
}

// State written by v0.5.1, before auto_confirm and revoked existed, plans no changes.
func TestAccOrganizationUserUpgradeFromV051(t *testing.T) {
	orgName := gofakeit.Company()
	email := gofakeit.Email()

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"vaultwarden": {Source: "ottramst/vaultwarden", VersionConstraint: "0.5.1"},
				},
				Config: testAccOrganizationUserConfigBasic(orgName, email),
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   testAccOrganizationUserConfigBasic(orgName, email),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// testAccCheckMemberHasOrganizationKey logs in as the member and checks that they can decrypt
// the organization key, which proves that the confirmation encrypted the right key for them.
func testAccCheckMemberHasOrganizationKey(email, password string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		orgID := s.RootModule().Resources["vaultwarden_organization.test"].Primary.ID

		ownerKey, err := testAccOrganizationKey(test.TestEmail, test.TestPassword, orgID)
		if err != nil {
			return fmt.Errorf("owner: %w", err)
		}
		memberKey, err := testAccOrganizationKey(email, password, orgID)
		if err != nil {
			return fmt.Errorf("member: %w", err)
		}

		if !bytes.Equal(ownerKey, memberKey) {
			return fmt.Errorf("the member's key for organization %s differs from the owner's", orgID)
		}
		return nil
	}
}

// testAccOrganizationKey logs in as a user and returns the organization key they decrypt.
func testAccOrganizationKey(email, password, orgID string) ([]byte, error) {
	ctx := context.Background()
	client, err := vaultwarden.New(test.TestBaseURL, vaultwarden.WithUserCredentials(email, password))
	if err != nil {
		return nil, err
	}
	// The organization keys are decrypted at login, which the first request triggers
	if _, err := client.GetProfile(ctx); err != nil {
		return nil, fmt.Errorf("failed to log in: %w", err)
	}
	secret, ok := client.OrganizationSecret(orgID)
	if !ok {
		return nil, fmt.Errorf("no key for organization %s", orgID)
	}
	return secret.Key.Key, nil
}

func testAccOrganizationUserStatusConfig(orgName, email string, autoConfirm, revoked bool) string {
	return testAccOrganizationUserStatusTypeConfig(orgName, email, "User", autoConfirm, revoked)
}

func testAccOrganizationUserStatusTypeConfig(orgName, email, userType string, autoConfirm, revoked bool) string {
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

resource "vaultwarden_organization_user" "test" {
  organization_id = vaultwarden_organization.test.id
  email           = %[6]q
  type            = %[7]q
  auto_confirm    = %[8]t
  revoked         = %[9]t
}
`, test.TestBaseURL, test.TestEmail, test.TestPassword, test.TestAdminToken, orgName, email, userType, autoConfirm, revoked)
}
