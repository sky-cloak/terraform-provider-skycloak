package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// securityFromModel converts the Terraform model into the facade request.
func securityFromModel(ctx context.Context, m *clusterSecurityModel, diags *diag.Diagnostics) *skycloak.ClusterSecurity {
	sec := &skycloak.ClusterSecurity{}

	if len(m.IPAccessControl) > 0 {
		ipc := &skycloak.IPAccessControl{}
		for _, r := range m.IPAccessControl {
			ipc.PathRules = append(ipc.PathRules, skycloak.IPPathRule{
				Path: r.Path.ValueString(), Description: r.Description.ValueString(),
				AllowedIPs: stringListToSlice(ctx, r.AllowedIPs, diags), AllowedCIDRs: stringListToSlice(ctx, r.AllowedCIDRs, diags),
			})
		}
		sec.IPAccessControl = ipc
	}

	if rl := m.RateLimiting; rl != nil {
		out := &skycloak.RateLimiting{Enabled: rl.Enabled.ValueBool(), GlobalRPM: rl.GlobalRPM.ValueInt64(), PerIPRPM: rl.PerIPRPM.ValueInt64()}
		for _, e := range rl.EndpointLimits {
			out.EndpointLimits = append(out.EndpointLimits, skycloak.EndpointLimit{Path: e.Path.ValueString(), RPM: e.RPM.ValueInt64()})
		}
		sec.RateLimiting = out
	}

	if w := m.WAF; w != nil {
		out := &skycloak.WAF{
			Enabled: w.Enabled.ValueBool(), Mode: w.Mode.ValueString(), Preset: w.Preset.ValueString(), ParanoiaLevel: w.ParanoiaLevel.ValueInt64(),
			ExclusionPaths: stringListToSlice(ctx, w.ExclusionPaths, diags),
		}
		if c := w.Categories; c != nil {
			out.Categories = &skycloak.WAFCategories{
				CrossSiteScripting: c.CrossSiteScripting.ValueBool(), DataLeakage: c.DataLeakage.ValueBool(), JavaAttacks: c.JavaAttacks.ValueBool(),
				LocalFileInclusion: c.LocalFileInclusion.ValueBool(), PhpInjection: c.PhpInjection.ValueBool(), ProtocolAttacks: c.ProtocolAttacks.ValueBool(),
				ProtocolEnforcement: c.ProtocolEnforcement.ValueBool(), RemoteCodeExecution: c.RemoteCodeExecution.ValueBool(), RemoteFileInclusion: c.RemoteFileInclusion.ValueBool(),
				SessionFixation: c.SessionFixation.ValueBool(), SQLInjection: c.SQLInjection.ValueBool(), WebshellDetection: c.WebshellDetection.ValueBool(),
			}
		}
		for _, re := range w.RuleExclusions {
			out.RuleExclusions = append(out.RuleExclusions, skycloak.WAFRuleExclusion{
				RuleIDs: stringListToSlice(ctx, re.RuleIDs, diags), Paths: stringListToSlice(ctx, re.Paths, diags),
			})
		}
		sec.WAF = out
	}

	if g := m.GeoBlocking; g != nil {
		sec.GeoBlocking = &skycloak.GeoBlocking{Enabled: g.Enabled.ValueBool(), Mode: g.Mode.ValueString(), Countries: stringListToSlice(ctx, g.Countries, diags)}
	}

	if b := m.BotManagement; b != nil {
		sec.BotManagement = &skycloak.BotManagement{
			Enabled: b.Enabled.ValueBool(), Mode: b.Mode.ValueString(), ChallengeMode: b.ChallengeMode.ValueString(),
			WhitelistedAgents: stringListToSlice(ctx, b.WhitelistedAgents, diags), BlacklistedAgents: stringListToSlice(ctx, b.BlacklistedAgents, diags),
		}
	}

	return sec
}

// applySecurityToModel copies the facade state back into the Terraform model.
func applySecurityToModel(ctx context.Context, sec *skycloak.ClusterSecurity, m *clusterSecurityModel, diags *diag.Diagnostics) {
	m.ID = types.StringValue(m.ClusterID.ValueString() + "/security")

	m.IPAccessControl = nil
	if sec.IPAccessControl != nil {
		for _, r := range sec.IPAccessControl.PathRules {
			m.IPAccessControl = append(m.IPAccessControl, ipPathRuleModel{
				Path: types.StringValue(r.Path), Description: optionalString(r.Description),
				AllowedIPs: sliceToStringList(ctx, r.AllowedIPs, diags), AllowedCIDRs: sliceToStringList(ctx, r.AllowedCIDRs, diags),
			})
		}
	}

	if rl := sec.RateLimiting; rl != nil {
		out := &rateLimitingModel{Enabled: types.BoolValue(rl.Enabled), GlobalRPM: optionalInt64(rl.GlobalRPM), PerIPRPM: optionalInt64(rl.PerIPRPM)}
		for _, e := range rl.EndpointLimits {
			out.EndpointLimits = append(out.EndpointLimits, endpointLimitModel{Path: types.StringValue(e.Path), RPM: types.Int64Value(e.RPM)})
		}
		m.RateLimiting = out
	} else {
		m.RateLimiting = nil
	}

	if w := sec.WAF; w != nil {
		out := &wafModel{
			Enabled: types.BoolValue(w.Enabled), Mode: types.StringValue(w.Mode), Preset: types.StringValue(w.Preset), ParanoiaLevel: types.Int64Value(w.ParanoiaLevel),
			ExclusionPaths: sliceToStringList(ctx, w.ExclusionPaths, diags),
		}
		if c := w.Categories; c != nil {
			out.Categories = &wafCategoriesModel{
				CrossSiteScripting: types.BoolValue(c.CrossSiteScripting), DataLeakage: types.BoolValue(c.DataLeakage), JavaAttacks: types.BoolValue(c.JavaAttacks),
				LocalFileInclusion: types.BoolValue(c.LocalFileInclusion), PhpInjection: types.BoolValue(c.PhpInjection), ProtocolAttacks: types.BoolValue(c.ProtocolAttacks),
				ProtocolEnforcement: types.BoolValue(c.ProtocolEnforcement), RemoteCodeExecution: types.BoolValue(c.RemoteCodeExecution), RemoteFileInclusion: types.BoolValue(c.RemoteFileInclusion),
				SessionFixation: types.BoolValue(c.SessionFixation), SQLInjection: types.BoolValue(c.SQLInjection), WebshellDetection: types.BoolValue(c.WebshellDetection),
			}
		}
		for _, re := range w.RuleExclusions {
			out.RuleExclusions = append(out.RuleExclusions, wafRuleExclusionModel{
				RuleIDs: sliceToStringList(ctx, re.RuleIDs, diags), Paths: sliceToStringList(ctx, re.Paths, diags),
			})
		}
		m.WAF = out
	} else {
		m.WAF = nil
	}

	if g := sec.GeoBlocking; g != nil {
		m.GeoBlocking = &geoBlockingModel{Enabled: types.BoolValue(g.Enabled), Mode: types.StringValue(g.Mode), Countries: sliceToStringList(ctx, g.Countries, diags)}
	} else {
		m.GeoBlocking = nil
	}

	if b := sec.BotManagement; b != nil {
		m.BotManagement = &botManagementModel{
			Enabled: types.BoolValue(b.Enabled), Mode: types.StringValue(b.Mode), ChallengeMode: types.StringValue(b.ChallengeMode),
			WhitelistedAgents: sliceToStringList(ctx, b.WhitelistedAgents, diags), BlacklistedAgents: sliceToStringList(ctx, b.BlacklistedAgents, diags),
		}
	} else {
		m.BotManagement = nil
	}
}

func optionalInt64(v int64) types.Int64 {
	if v == 0 {
		return types.Int64Null()
	}
	return types.Int64Value(v)
}
