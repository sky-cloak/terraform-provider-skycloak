package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

// The public API used to report every US cluster as `us`, East or West, so a
// US West cluster may sit in state (and in configuration) as `us`. Once the
// API reports it as `us-west`, a plain copy of the API value into state would
// plan a destroy and recreate of a cluster that never moved. The guard below
// keeps such a cluster as `us` until its configuration says `us-west`, and lets
// that relabel apply in place. See docs/adr/0004.
const (
	locationUS     = "us"
	locationUSWest = "us-west"

	// apiLocationKey is the private state key holding the location the API
	// last reported, so plan-time logic can tell a US West cluster recorded as
	// `us` from a genuine US East cluster.
	apiLocationKey = "api_location"
)

// privateKeySetter is the part of resource private state the guard writes to.
type privateKeySetter interface {
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

// reconcileClusterLocation returns the location to store in state, given the
// location recorded (or planned) before this call and the one the API reports.
// A cluster recorded as `us` that the API reports as `us-west` keeps `us`, with
// a warning asking the user to update the configuration; any other value is
// taken as reported.
func reconcileClusterLocation(prior, reported string, diags *diag.Diagnostics) string {
	if prior == locationUS && reported == locationUSWest {
		diags.AddAttributeWarning(path.Root("location"),
			"Cluster location is now reported as us-west",
			`Skycloak now reports this cluster as "us-west" (US West). It was previously reported as "us", `+
				`which now means US East only. The provider keeps "us" in state so the cluster is not replaced. `+
				`Set location = "us-west" in the configuration: that change is applied in place and does not replace the cluster.`)
		return locationUS
	}
	return reported
}

// clusterLocationChangeRequiresReplace reports whether changing a cluster's
// location from state to config is a real move. The only change that is not
// is a relabel between `us` and `us-west` of a cluster the API reports as
// `us-west`: both labels name the same US West cluster. An unknown reported
// location (never recorded) is treated as a move.
func clusterLocationChangeRequiresReplace(state, config, reported string) bool {
	if state == config {
		return false
	}
	isUS := func(l string) bool { return l == locationUS || l == locationUSWest }
	return !isUS(state) || !isUS(config) || reported != locationUSWest
}

// requiresReplaceUnlessUSWestRelabel is the location attribute's
// RequiresReplaceIf function.
func requiresReplaceUnlessUSWestRelabel(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	if req.PlanValue.IsUnknown() {
		resp.RequiresReplace = true
		return
	}
	var reported string
	raw, diags := req.Private.GetKey(ctx, apiLocationKey)
	resp.Diagnostics.Append(diags...)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &reported); err != nil {
			reported = ""
		}
	}
	resp.RequiresReplace = clusterLocationChangeRequiresReplace(req.StateValue.ValueString(), req.PlanValue.ValueString(), reported)
}

// setAPILocation records the location the API reported in private state.
func setAPILocation(ctx context.Context, private privateKeySetter, reported string) diag.Diagnostics {
	raw, err := json.Marshal(reported)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Unable to record cluster location", err.Error())
		return diags
	}
	return private.SetKey(ctx, apiLocationKey, raw)
}
