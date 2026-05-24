package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// optionalString maps an API string to a Terraform value, treating "" as null
// so optional, unset attributes do not show a perpetual diff.
func optionalString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func stringListToSlice(ctx context.Context, l types.List, diags *diag.Diagnostics) []string {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(l.ElementsAs(ctx, &out, false)...)
	return out
}

func sliceToStringList(ctx context.Context, s []string, diags *diag.Diagnostics) types.List {
	if len(s) == 0 {
		return types.ListNull(types.StringType)
	}
	v, d := types.ListValueFrom(ctx, types.StringType, s)
	diags.Append(d...)
	return v
}

func stringMapToMap(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	out := map[string]string{}
	diags.Append(m.ElementsAs(ctx, &out, false)...)
	return out
}

func mapToStringMap(ctx context.Context, m map[string]string, diags *diag.Diagnostics) types.Map {
	if len(m) == 0 {
		return types.MapNull(types.StringType)
	}
	v, d := types.MapValueFrom(ctx, types.StringType, m)
	diags.Append(d...)
	return v
}
