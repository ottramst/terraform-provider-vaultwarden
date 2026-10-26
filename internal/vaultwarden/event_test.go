package vaultwarden_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// More events than fit on a page are all returned once: Vaultwarden returns 30 events per page.
func TestAccOrganizationEventsPagination(t *testing.T) {
	ctx, client, _, _ := itemTestSetup(t)

	start := time.Now().Add(-time.Minute)
	org, err := client.CreateOrganization(ctx, models.Organization{Name: "Events", CollectionName: "Default"})
	must(t, err)
	t.Cleanup(func() { _ = client.DeleteOrganization(context.Background(), org.ID) })

	created := map[string]bool{}
	for i := range 70 {
		collection, err := client.CreateOrganizationCollection(ctx, org.ID, models.Collection{Name: fmt.Sprintf("Collection %d", i)})
		must(t, err)
		created[collection.ID] = true
	}

	events, err := client.ListOrganizationEvents(ctx, org.ID, start, time.Now().Add(time.Minute))
	must(t, err)
	found := map[string]int{}
	for i, event := range events {
		if i > 0 && event.Date > events[i-1].Date {
			t.Fatalf("events are not sorted newest first: %s after %s", event.Date, events[i-1].Date)
		}
		if event.Type == models.EventType(1300) {
			found[event.CollectionID]++
		}
	}
	for id := range created {
		if found[id] != 1 {
			t.Errorf("collection %s has %d collection_created events, want 1", id, found[id])
		}
	}

	// An organization that does not exist is not found
	if _, err := client.ListOrganizationEvents(ctx, "00000000-0000-0000-0000-000000000000", start, time.Now()); !vaultwarden.IsNotFound(err) {
		t.Errorf("events of a missing organization: %v", err)
	}
}
