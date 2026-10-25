package vaultwarden

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// ErrGroupsDisabled is returned, wrapped, when the server has organization groups disabled.
var ErrGroupsDisabled = errors.New("organization groups are disabled on the Vaultwarden server, enable them with the ORG_GROUPS_ENABLED=true server setting")

// groupRequest is the request body for creating and updating a group. Vaultwarden replaces the
// group's collection and member assignments with the ones sent, and ignores externalId on updates.
type groupRequest struct {
	Name        string                    `json:"name"`
	AccessAll   bool                      `json:"accessAll"`
	ExternalID  *string                   `json:"externalId"`
	Collections []models.CollectionAccess `json:"collections"`
	Users       []string                  `json:"users"`
}

// groupError maps the errors of the group endpoints. Vaultwarden answers 400 both for a group
// that does not exist and when groups are disabled, and 401 for a deleted organization.
func (c *Client) groupError(ctx context.Context, orgID, groupID string, err error) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusBadRequest {
		switch {
		case strings.Contains(apiErr.Body, "Group support is disabled"):
			return ErrGroupsDisabled
		case strings.Contains(apiErr.Body, "Group not found"), strings.Contains(apiErr.Body, "Group could not be found"):
			return fmt.Errorf("group %s in organization %s: %w", groupID, orgID, ErrNotFound)
		}
	}
	if c.organizationGone(ctx, orgID, err) {
		return fmt.Errorf("organization %s: %w", orgID, ErrNotFound)
	}

	return err
}

// CreateOrganizationGroup creates a group in an organization, without collections or members.
func (c *Client) CreateOrganizationGroup(ctx context.Context, orgID string, group models.Group) (*models.Group, error) {
	req := groupRequest{
		Name:        group.Name,
		ExternalID:  group.ExternalID,
		Collections: []models.CollectionAccess{},
		Users:       []string{},
	}
	if group.AccessAll != nil {
		req.AccessAll = *group.AccessAll
	}

	var resp models.Group
	if _, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/organizations/%s/groups", orgID), req, &resp); err != nil {
		return nil, fmt.Errorf("failed to create organization group: %w", c.groupError(ctx, orgID, "", err))
	}

	return &resp, nil
}

// GetOrganizationGroup returns a group with the collections assigned to it.
func (c *Client) GetOrganizationGroup(ctx context.Context, orgID, groupID string) (*models.GroupDetails, error) {
	var group models.GroupDetails
	if _, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/organizations/%s/groups/%s/details", orgID, groupID), nil, &group); err != nil {
		return nil, fmt.Errorf("failed to get organization group: %w", c.groupError(ctx, orgID, groupID, err))
	}

	return &group, nil
}

// GetOrganizationGroups returns the groups of an organization. Vaultwarden returns no groups
// when groups are disabled.
func (c *Client) GetOrganizationGroups(ctx context.Context, orgID string) ([]models.Group, error) {
	var groups models.OrganizationGroups
	if _, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/organizations/%s/groups", orgID), nil, &groups); err != nil {
		return nil, fmt.Errorf("failed to list organization groups: %w", c.groupError(ctx, orgID, "", err))
	}

	return groups.Data, nil
}

// GetOrganizationGroupMembers returns the IDs of the organization users in a group.
func (c *Client) GetOrganizationGroupMembers(ctx context.Context, orgID, groupID string) ([]string, error) {
	var members []string
	if _, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/organizations/%s/groups/%s/users", orgID, groupID), nil, &members); err != nil {
		return nil, fmt.Errorf("failed to get organization group members: %w", c.groupError(ctx, orgID, groupID, err))
	}

	return members, nil
}

// UpdateOrganizationGroup updates the name and access_all of a group. The group's collection and
// member assignments are preserved. Vaultwarden does not change the external ID of a group.
func (c *Client) UpdateOrganizationGroup(ctx context.Context, orgID, groupID string, group models.Group) (*models.Group, error) {
	// Vaultwarden replaces the group's collection and member assignments with the ones sent,
	// so send back the current ones
	defer c.lockOrganizationAccess(orgID)()

	current, err := c.GetOrganizationGroup(ctx, orgID, groupID)
	if err != nil {
		return nil, err
	}
	members, err := c.GetOrganizationGroupMembers(ctx, orgID, groupID)
	if err != nil {
		return nil, err
	}

	req := groupRequest{
		Name:        group.Name,
		ExternalID:  current.ExternalID,
		Collections: current.Collections,
		Users:       members,
	}
	if group.AccessAll != nil {
		req.AccessAll = *group.AccessAll
	}
	if req.Collections == nil {
		req.Collections = []models.CollectionAccess{}
	}
	if req.Users == nil {
		req.Users = []string{}
	}

	var resp models.Group
	if _, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/organizations/%s/groups/%s", orgID, groupID), req, &resp); err != nil {
		return nil, fmt.Errorf("failed to update organization group: %w", c.groupError(ctx, orgID, groupID, err))
	}

	return &resp, nil
}

// DeleteOrganizationGroup deletes a group from an organization.
func (c *Client) DeleteOrganizationGroup(ctx context.Context, orgID, groupID string) error {
	if _, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/organizations/%s/groups/%s", orgID, groupID), nil, nil); err != nil {
		err = c.groupError(ctx, orgID, groupID, err)
		// Already deleted
		if IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete organization group: %w", err)
	}

	return nil
}

// GetOrganizationGroupMember returns ErrNotFound unless the organization user is a member of the group.
func (c *Client) GetOrganizationGroupMember(ctx context.Context, orgID, groupID, userID string) error {
	members, err := c.GetOrganizationGroupMembers(ctx, orgID, groupID)
	if err != nil {
		return err
	}
	if !slices.Contains(members, userID) {
		return fmt.Errorf("organization user %s in group %s: %w", userID, groupID, ErrNotFound)
	}

	return nil
}

// AddOrganizationGroupMember adds an organization user to a group. The other members of the
// group and the other groups of the user are preserved.
func (c *Client) AddOrganizationGroupMember(ctx context.Context, orgID, groupID, userID string) error {
	defer c.lockOrganizationAccess(orgID)()

	// Also checks that the group exists: Vaultwarden 1.36 does not validate the groups sent
	members, err := c.GetOrganizationGroupMembers(ctx, orgID, groupID)
	if err != nil {
		return err
	}
	if slices.Contains(members, userID) {
		return nil
	}

	return c.setOrganizationUserGroups(ctx, orgID, userID, func(groups []string) []string {
		return append(groups, groupID)
	})
}

// RemoveOrganizationGroupMember removes an organization user from a group. The other members of
// the group and the other groups of the user are preserved.
func (c *Client) RemoveOrganizationGroupMember(ctx context.Context, orgID, groupID, userID string) error {
	defer c.lockOrganizationAccess(orgID)()

	members, err := c.GetOrganizationGroupMembers(ctx, orgID, groupID)
	if err != nil {
		// Group or organization already deleted
		if IsNotFound(err) {
			return nil
		}
		return err
	}
	if !slices.Contains(members, userID) {
		return nil
	}

	err = c.setOrganizationUserGroups(ctx, orgID, userID, func(groups []string) []string {
		return slices.DeleteFunc(groups, func(id string) bool { return id == groupID })
	})
	// Organization user already removed
	if IsNotFound(err) {
		return nil
	}

	return err
}
