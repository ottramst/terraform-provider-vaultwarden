package vaultwarden

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// GetOrganizationPolicy returns a policy of an organization. A policy that was never set is
// returned as disabled.
func (c *Client) GetOrganizationPolicy(ctx context.Context, orgID string, policyType int) (*models.Policy, error) {
	var policy models.Policy
	if _, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/organizations/%s/policies/%d", orgID, policyType), nil, &policy); err != nil {
		if c.organizationGone(ctx, orgID, err) {
			return nil, fmt.Errorf("organization %s: %w", orgID, ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get organization policy: %w", err)
	}

	return &policy, nil
}

// policyData is the policy in an update request.
type policyData struct {
	Enabled bool            `json:"enabled"`
	Data    json.RawMessage `json:"data"`
}

// updatePolicyRequest is the request body for updating a policy. Vaultwarden 1.37 expects the
// policy wrapped in "policy", older versions expect its fields at the top level; neither
// rejects unknown fields, so both are sent.
type updatePolicyRequest struct {
	policyData
	Policy policyData `json:"policy"`
}

// UpdateOrganizationPolicy enables or disables a policy of an organization and sets its data.
// data may be nil.
func (c *Client) UpdateOrganizationPolicy(ctx context.Context, orgID string, policyType int, enabled bool, data json.RawMessage) (*models.Policy, error) {
	if data == nil {
		data = json.RawMessage("null")
	}
	policy := policyData{Enabled: enabled, Data: data}

	var resp models.Policy
	if _, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/organizations/%s/policies/%d", orgID, policyType), updatePolicyRequest{policyData: policy, Policy: policy}, &resp); err != nil {
		return nil, fmt.Errorf("failed to update organization policy: %w", err)
	}

	return &resp, nil
}
