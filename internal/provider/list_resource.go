package provider

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden"
)

// configureListResourceClient returns the client passed to a list resource by the provider, or
// nil when the provider is not configured yet.
func configureListResourceClient(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *vaultwarden.Client {
	if req.ProviderData == nil {
		return nil
	}

	client, ok := req.ProviderData.(*vaultwarden.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected List Resource Configure Type",
			fmt.Sprintf("Expected *vaultwarden.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return nil
	}

	return client
}

// listOrganizationIDAttribute is the organization_id attribute of the list resources of
// objects of an organization.
func listOrganizationIDAttribute(objects string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "The ID of the organization to list the " + objects + " of",
		Required:            true,
	}
}

// listContainsAttribute is an optional filter attribute that matches a part of a value.
func listContainsAttribute(objects, value string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "Only list the " + objects + " whose " + value + " contains this text, ignoring case",
		Optional:            true,
	}
}

// containsFold reports whether value contains filter, ignoring case. A null filter matches
// every value.
func containsFold(value string, filter types.String) bool {
	return filter.IsNull() || strings.Contains(strings.ToLower(value), strings.ToLower(filter.ValueString()))
}

// newListResult returns a result with the identity and display name of an object, and its
// state when Terraform requested it.
func newListResult(ctx context.Context, req list.ListRequest, displayName string, identity, state any) list.ListResult {
	result := req.NewListResult(ctx)
	result.DisplayName = displayName
	result.Diagnostics.Append(result.Identity.Set(ctx, identity)...)
	if req.IncludeResource {
		result.Diagnostics.Append(result.Resource.Set(ctx, state)...)
	}
	return result
}

// listResults streams the result of each item. Terraform stops the stream once it has the
// number of results it requested (the limit of the list block).
func listResults[T any](items []T, result func(T) list.ListResult) iter.Seq[list.ListResult] {
	return func(push func(list.ListResult) bool) {
		for _, item := range items {
			if !push(result(item)) {
				return
			}
		}
	}
}
