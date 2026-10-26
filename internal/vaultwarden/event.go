package vaultwarden

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// ListOrganizationEvents returns the events of an organization between start and end, newest
// first. It needs the Admin or Owner role. Vaultwarden only logs events with ORG_EVENTS_ENABLED,
// and otherwise returns none.
//
// Vaultwarden returns 30 events per page, and the continuation token is the date of the last
// one. The next page is queried up to that date, inclusive, so events at that date that were
// already returned are skipped.
func (c *Client) ListOrganizationEvents(ctx context.Context, orgID string, start, end time.Time) ([]models.Event, error) {
	query := url.Values{}
	query.Set("start", start.UTC().Format(time.RFC3339Nano))
	query.Set("end", end.UTC().Format(time.RFC3339Nano))

	events := []models.Event{}
	// seen holds the raw events of the previous page at the date of the continuation token
	var seen map[string]bool
	for {
		var page struct {
			Data              []json.RawMessage `json:"data"`
			ContinuationToken *string           `json:"continuationToken"`
		}
		path := fmt.Sprintf("/api/organizations/%s/events?%s", orgID, query.Encode())
		if _, err := c.doRequest(ctx, http.MethodGet, path, nil, &page); err != nil {
			if c.organizationGone(ctx, orgID, err) {
				return nil, fmt.Errorf("organization %s: %w", orgID, ErrNotFound)
			}
			return nil, fmt.Errorf("failed to list organization events: %w", err)
		}

		token := valueOf(page.ContinuationToken)
		added := 0
		next := map[string]bool{}
		for _, raw := range page.Data {
			var event models.Event
			if err := json.Unmarshal(raw, &event); err != nil {
				return nil, fmt.Errorf("failed to decode event: %w", err)
			}
			if event.Date == token {
				next[string(raw)] = true
			}
			if seen[string(raw)] {
				continue
			}
			events = append(events, event)
			added++
		}

		// Stop at the last page, or when a page only repeats events, which happens when more
		// than a page of events share the same date
		if token == "" || added == 0 {
			return events, nil
		}
		query.Set("continuationToken", token)
		seen = next
	}
}
