package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// readFile reads a local file referenced by an upload attribute.
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// fileSHA256 returns the hex-encoded SHA-256 of a local file's contents. Used by
// upload resources to detect when a file's bytes change between plans.
func fileSHA256(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

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
