package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithModifyPlan = (*clusterResource)(nil)

// parseVersion splits a dotted numeric version such as 26.8.2 into its
// segments. It reports false for anything else.
func parseVersion(v string) ([]int, bool) {
	parts := strings.Split(strings.TrimSpace(v), ".")
	segments := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		segments = append(segments, n)
	}
	return segments, true
}

// isVersionDowngrade reports whether configured is lower than current. Only
// the segments both versions have are compared, so 26.1 is not lower than
// 26.1.3. Versions that cannot be parsed are never a downgrade.
func isVersionDowngrade(configured, current string) bool {
	want, ok := parseVersion(configured)
	if !ok {
		return false
	}
	have, ok := parseVersion(current)
	if !ok {
		return false
	}
	for i := 0; i < len(want) && i < len(have); i++ {
		if want[i] != have[i] {
			return want[i] < have[i]
		}
	}
	return false
}

// ModifyPlan refuses a version lower than the one the cluster runs, which the
// API would reject at apply.
func (r *clusterResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}
	var configured, current types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("version"), &configured)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("version"), &current)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if configured.IsNull() || configured.IsUnknown() || current.IsNull() || current.IsUnknown() {
		return
	}
	if isVersionDowngrade(configured.ValueString(), current.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("version"),
			"Keycloak version cannot be lowered",
			fmt.Sprintf("The configured version %q is lower than the cluster's current version %q, and a Keycloak version cannot be downgraded. Set version to %q or higher.",
				configured.ValueString(), current.ValueString(), current.ValueString()))
	}
}
