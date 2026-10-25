package provider

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
)

// configureLookupClient returns the client passed to a data source by the provider, or nil
// when the provider is not configured yet.
func configureLookupClient(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *vaultwarden.Client {
	if req.ProviderData == nil {
		return nil
	}

	client, ok := req.ProviderData.(*vaultwarden.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *vaultwarden.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return nil
	}

	return client
}

// stringOrNull returns a null string for an empty value.
func stringOrNull(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

// addOrganizationListError reports an error listing the objects of an organization, with a clear
// message when the organization does not exist or the provider user is not a member of it.
func addOrganizationListError(diags *diag.Diagnostics, orgID, objects string, err error) {
	if vaultwarden.IsNotFound(err) {
		diags.AddError(
			"Organization Not Found",
			fmt.Sprintf("Organization %s does not exist, or the provider user is not a member of it.", orgID),
		)
		return
	}
	diags.AddError(
		"Error Reading Organization "+objects,
		fmt.Sprintf("Could not list the %s of organization %s: %s", strings.ToLower(objects), orgID, err),
	)
}
