package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// sensitiveFunc reports whether the attribute at a path of a schema is sensitive.
type sensitiveFunc func(path.Path) (bool, diag.Diagnostics)

// TestItemSecretsAreSensitive checks that the secrets of items are marked sensitive in all
// resources, data sources and ephemeral resources.
func TestItemSecretsAreSensitive(t *testing.T) {
	ctx := context.Background()

	resourceSchema := func(r resource.Resource) sensitiveFunc {
		resp := &resource.SchemaResponse{}
		r.Schema(ctx, resource.SchemaRequest{}, resp)
		return func(p path.Path) (bool, diag.Diagnostics) {
			attribute, diags := resp.Schema.AttributeAtPath(ctx, p)
			return diags == nil && attribute.IsSensitive(), diags
		}
	}
	dataSourceSchema := func(d datasource.DataSource) sensitiveFunc {
		resp := &datasource.SchemaResponse{}
		d.Schema(ctx, datasource.SchemaRequest{}, resp)
		return func(p path.Path) (bool, diag.Diagnostics) {
			attribute, diags := resp.Schema.AttributeAtPath(ctx, p)
			return diags == nil && attribute.IsSensitive(), diags
		}
	}
	ephemeralSchema := func(e ephemeral.EphemeralResource) sensitiveFunc {
		resp := &ephemeral.SchemaResponse{}
		e.Schema(ctx, ephemeral.SchemaRequest{}, resp)
		return func(p path.Path) (bool, diag.Diagnostics) {
			attribute, diags := resp.Schema.AttributeAtPath(ctx, p)
			return diags == nil && attribute.IsSensitive(), diags
		}
	}

	secrets := []path.Path{
		path.Root("notes"),
		path.Root("fields").AtListIndex(0).AtName("value"),
	}
	loginSecrets := append([]path.Path{path.Root("password"), path.Root("totp")}, secrets...)

	for name, schema := range map[string]sensitiveFunc{
		"resource login":          resourceSchema(ItemLoginResource()),
		"data source login":       dataSourceSchema(NewItemLoginDataSource()),
		"ephemeral login":         ephemeralSchema(NewItemLoginEphemeralResource()),
		"resource secure note":    resourceSchema(ItemSecureNoteResource()),
		"data source secure note": dataSourceSchema(NewItemSecureNoteDataSource()),
		"ephemeral secure note":   ephemeralSchema(NewItemSecureNoteEphemeralResource()),
	} {
		paths := secrets
		if strings.HasSuffix(name, "login") {
			paths = loginSecrets
		}
		for _, p := range paths {
			sensitive, diags := schema(p)
			if diags.HasError() {
				t.Errorf("%s: %s: %v", name, p, diags)
			} else if !sensitive {
				t.Errorf("%s: %s is not sensitive", name, p)
			}
		}
	}
}
