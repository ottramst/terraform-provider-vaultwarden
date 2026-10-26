package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccOrganizationEventsDataSource(t *testing.T) {
	orgName := gofakeit.Company()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Creating the collection logs an event
			{
				Config: testAccOrganizationEventsConfig(orgName, `start = "2000-01-01T00:00:00Z"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("data.vaultwarden_organization_events.test", "end", regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T`)),
					testAccCheckCollectionCreatedEvent(),
				),
			},
			// No events before the organization was created
			{
				Config: testAccOrganizationEventsConfig(orgName, `
  start = "2000-01-01T00:00:00Z"
  end   = "2000-01-02T00:00:00+02:00"
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.vaultwarden_organization_events.test", "end", "2000-01-02T00:00:00+02:00"),
					resource.TestCheckResourceAttr("data.vaultwarden_organization_events.test", "events.#", "0"),
				),
			},
			{
				Config:      testAccOrganizationEventsConfig(orgName, `start = "yesterday"`),
				ExpectError: regexp.MustCompile(`start must be an RFC 3339 timestamp`),
			},
			{
				Config: testAccOrganizationEventsConfig(orgName, `
  start = "2000-01-02T00:00:00Z"
  end   = "2000-01-01T00:00:00Z"
`),
				ExpectError: regexp.MustCompile(`end must not be before start`),
			},
		},
	})
}

// testAccCheckCollectionCreatedEvent checks that the events include the creation of the collection.
func testAccCheckCollectionCreatedEvent() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		collectionID := s.RootModule().Resources["vaultwarden_organization_collection.test"].Primary.Attributes["id"]
		attributes := s.RootModule().Resources["data.vaultwarden_organization_events.test"].Primary.Attributes
		for i := 0; attributes[fmt.Sprintf("events.%d.type", i)] != ""; i++ {
			prefix := fmt.Sprintf("events.%d.", i)
			if attributes[prefix+"collection_id"] == collectionID {
				if attributes[prefix+"type"] != "1300" || attributes[prefix+"type_name"] != "collection_created" || attributes[prefix+"acting_user_id"] == "" {
					return fmt.Errorf("unexpected event for collection %s: type %s (%s), acting user %q", collectionID,
						attributes[prefix+"type"], attributes[prefix+"type_name"], attributes[prefix+"acting_user_id"])
				}
				return nil
			}
		}
		return fmt.Errorf("no event for collection %s in %s events", collectionID, attributes["events.#"])
	}
}

func testAccOrganizationEventsConfig(orgName, attributes string) string {
	return testAccItemProviderConfig() + fmt.Sprintf(`
resource "vaultwarden_organization" "test" {
  name = %[1]q
}

resource "vaultwarden_organization_collection" "test" {
  organization_id = vaultwarden_organization.test.id
  name            = "Audited"
}

data "vaultwarden_organization_events" "test" {
  organization_id = vaultwarden_organization.test.id
  %[2]s

  depends_on = [vaultwarden_organization_collection.test]
}
`, orgName, attributes)
}
