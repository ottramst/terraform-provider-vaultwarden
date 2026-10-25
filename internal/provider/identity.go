package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// OrganizationObjectIdentityModel is the resource identity of an object of an organization,
// such as a collection, group or member. It holds the same values as the import ID
// "<organization_id>/<id>".
type OrganizationObjectIdentityModel struct {
	OrganizationID types.String `tfsdk:"organization_id"`
	ID             types.String `tfsdk:"id"`
}

// UserIdentityModel is the resource identity of a user of the server.
type UserIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// organizationObjectIdentitySchema returns the identity schema of an object of an organization.
func organizationObjectIdentitySchema(object string) identityschema.Schema {
	return identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"organization_id": identityschema.StringAttribute{
				Description:       "ID of the organization",
				RequiredForImport: true,
			},
			"id": identityschema.StringAttribute{
				Description:       "ID of the " + object,
				RequiredForImport: true,
			},
		},
	}
}

// setOrganizationObjectIdentity sets the identity of an object of an organization. The framework
// only passes a nil identity to resources without an identity schema.
func setOrganizationObjectIdentity(ctx context.Context, identity *tfsdk.ResourceIdentity, orgID, id types.String) diag.Diagnostics {
	if identity == nil {
		return nil
	}
	return identity.Set(ctx, OrganizationObjectIdentityModel{OrganizationID: orgID, ID: id})
}

// setUserIdentity sets the identity of a user. The framework only passes a nil identity to
// resources without an identity schema.
func setUserIdentity(ctx context.Context, identity *tfsdk.ResourceIdentity, id types.String) diag.Diagnostics {
	if identity == nil {
		return nil
	}
	return identity.Set(ctx, UserIdentityModel{ID: id})
}

// importOrganizationObject reads the organization ID and the object ID from the import ID
// "<organization_id>/<id>", or from the identity when the resource is imported with an
// identity, and sets them in the state and the identity. It returns false on an error.
func importOrganizationObject(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, idName string) (orgID, id string, ok bool) {
	if req.ID != "" {
		idParts := strings.Split(req.ID, "/")
		if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
			resp.Diagnostics.AddError(
				"Invalid ID format",
				fmt.Sprintf("Expected import identifier with format: organization_id/%s, got: %q", idName, req.ID),
			)
			return "", "", false
		}
		orgID, id = idParts[0], idParts[1]
	} else {
		var identity OrganizationObjectIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return "", "", false
		}
		orgID, id = identity.OrganizationID.ValueString(), identity.ID.ValueString()
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("organization_id"), orgID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	resp.Diagnostics.Append(setOrganizationObjectIdentity(ctx, resp.Identity, types.StringValue(orgID), types.StringValue(id))...)

	return orgID, id, !resp.Diagnostics.HasError()
}
