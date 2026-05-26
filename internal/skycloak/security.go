package skycloak

import (
	"context"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/apiclient"
)

// ClusterSecurity is the edge-security configuration for a cluster (IP access
// control, rate limiting, WAF, geo-blocking, bot management). The CAPTCHA
// sub-config is intentionally not surfaced here; it is preserved untouched on
// update.
type ClusterSecurity struct {
	IPAccessControl *IPAccessControl
	RateLimiting    *RateLimiting
	WAF             *WAF
	GeoBlocking     *GeoBlocking
	BotManagement   *BotManagement
}

// IPPathRule restricts a URL path to a set of IPs/CIDRs.
type IPPathRule struct {
	Path         string
	Description  string
	AllowedIPs   []string
	AllowedCIDRs []string
}

// IPAccessControl holds per-path IP allow rules.
type IPAccessControl struct {
	PathRules []IPPathRule
}

// EndpointLimit caps requests-per-minute on a path.
type EndpointLimit struct {
	Path string
	RPM  int64
}

// RateLimiting configures request-rate ceilings.
type RateLimiting struct {
	Enabled        bool
	GlobalRPM      int64
	PerIPRPM       int64
	EndpointLimits []EndpointLimit
}

// WAFCategories toggles OWASP CRS rule categories (used when preset is custom).
type WAFCategories struct {
	CrossSiteScripting  bool
	DataLeakage         bool
	JavaAttacks         bool
	LocalFileInclusion  bool
	PhpInjection        bool
	ProtocolAttacks     bool
	ProtocolEnforcement bool
	RemoteCodeExecution bool
	RemoteFileInclusion bool
	SessionFixation     bool
	SQLInjection        bool
	WebshellDetection   bool
}

// WAFRuleExclusion disables specific CRS rules, optionally scoped to paths.
type WAFRuleExclusion struct {
	RuleIDs []string
	Paths   []string
}

// WAF configures the web application firewall.
type WAF struct {
	Enabled        bool
	Mode           string
	Preset         string
	ParanoiaLevel  int64
	Categories     *WAFCategories
	ExclusionPaths []string
	RuleExclusions []WAFRuleExclusion
}

// GeoBlocking restricts access by country.
type GeoBlocking struct {
	Enabled   bool
	Mode      string
	Countries []string
}

// BotManagement configures bot detection and challenges.
type BotManagement struct {
	Enabled           bool
	Mode              string
	ChallengeMode     string
	WhitelistedAgents []string
	BlacklistedAgents []string
}

func derefIntStar(p *int) int64 {
	if p == nil {
		return 0
	}
	return int64(*p)
}

func derefSlice(p *[]string) []string {
	if p == nil {
		return nil
	}
	return *p
}

func securityFromAPI(c *apiclient.ClusterSecurityConfig) *ClusterSecurity {
	out := &ClusterSecurity{}
	if c.IpAccessControl != nil {
		ipc := &IPAccessControl{}
		for _, r := range c.IpAccessControl.PathRules {
			ipc.PathRules = append(ipc.PathRules, IPPathRule{
				Path: r.Path, Description: strDerefPtr(r.Description),
				AllowedIPs: derefSlice(r.AllowedIps), AllowedCIDRs: derefSlice(r.AllowedCidrs),
			})
		}
		out.IPAccessControl = ipc
	}
	if c.RateLimiting != nil {
		rl := &RateLimiting{Enabled: c.RateLimiting.Enabled, GlobalRPM: derefIntStar(c.RateLimiting.GlobalRpm), PerIPRPM: derefIntStar(c.RateLimiting.PerIpRpm)}
		if c.RateLimiting.EndpointLimits != nil {
			for _, e := range *c.RateLimiting.EndpointLimits {
				rl.EndpointLimits = append(rl.EndpointLimits, EndpointLimit{Path: e.Path, RPM: int64(e.Rpm)})
			}
		}
		out.RateLimiting = rl
	}
	if c.Waf != nil {
		w := &WAF{Enabled: c.Waf.Enabled, Mode: string(c.Waf.Mode), Preset: string(c.Waf.Preset), ParanoiaLevel: int64(c.Waf.ParanoiaLevel)}
		if cat := c.Waf.EnabledCategories; cat != nil {
			w.Categories = &WAFCategories{
				CrossSiteScripting: cat.CrossSiteScripting, DataLeakage: cat.DataLeakage, JavaAttacks: cat.JavaAttacks,
				LocalFileInclusion: cat.LocalFileInclusion, PhpInjection: cat.PhpInjection, ProtocolAttacks: cat.ProtocolAttacks,
				ProtocolEnforcement: cat.ProtocolEnforcement, RemoteCodeExecution: cat.RemoteCodeExecution, RemoteFileInclusion: cat.RemoteFileInclusion,
				SessionFixation: cat.SessionFixation, SQLInjection: cat.SqlInjection, WebshellDetection: cat.WebshellDetection,
			}
		}
		if ex := c.Waf.Exclusions; ex != nil {
			w.ExclusionPaths = derefSlice(ex.Paths)
			if ex.Rules != nil {
				for _, re := range *ex.Rules {
					w.RuleExclusions = append(w.RuleExclusions, WAFRuleExclusion{RuleIDs: re.RuleIds, Paths: derefSlice(re.Paths)})
				}
			}
		}
		out.WAF = w
	}
	if c.GeoBlocking != nil {
		out.GeoBlocking = &GeoBlocking{Enabled: c.GeoBlocking.Enabled, Mode: string(c.GeoBlocking.Mode), Countries: c.GeoBlocking.Countries}
	}
	if c.BotManagement != nil {
		out.BotManagement = &BotManagement{
			Enabled: c.BotManagement.Enabled, Mode: string(c.BotManagement.Mode), ChallengeMode: string(c.BotManagement.ChallengeMode),
			WhitelistedAgents: derefSlice(c.BotManagement.WhitelistedAgents), BlacklistedAgents: derefSlice(c.BotManagement.BlacklistedAgents),
		}
	}
	return out
}

// applyToAPI overlays the managed sections onto an existing API config,
// leaving any field this facade does not manage (e.g. captcha) untouched.
func (s *ClusterSecurity) applyToAPI(c *apiclient.ClusterSecurityConfig) {
	if s.IPAccessControl != nil {
		rules := make([]apiclient.IPPathRule, 0, len(s.IPAccessControl.PathRules))
		for _, r := range s.IPAccessControl.PathRules {
			rule := apiclient.IPPathRule{Path: r.Path}
			rule.Description = strPtr(r.Description)
			if len(r.AllowedIPs) > 0 {
				ips := r.AllowedIPs
				rule.AllowedIps = &ips
			}
			if len(r.AllowedCIDRs) > 0 {
				cidrs := r.AllowedCIDRs
				rule.AllowedCidrs = &cidrs
			}
			rules = append(rules, rule)
		}
		c.IpAccessControl = &apiclient.IPAccessControlConfig{PathRules: rules}
	}
	if s.RateLimiting != nil {
		rl := &apiclient.RateLimitingConfig{Enabled: s.RateLimiting.Enabled}
		if s.RateLimiting.GlobalRPM > 0 {
			g := int(s.RateLimiting.GlobalRPM)
			rl.GlobalRpm = &g
		}
		if s.RateLimiting.PerIPRPM > 0 {
			p := int(s.RateLimiting.PerIPRPM)
			rl.PerIpRpm = &p
		}
		if len(s.RateLimiting.EndpointLimits) > 0 {
			lims := make([]apiclient.EndpointRateLimit, 0, len(s.RateLimiting.EndpointLimits))
			for _, e := range s.RateLimiting.EndpointLimits {
				lims = append(lims, apiclient.EndpointRateLimit{Path: e.Path, Rpm: int(e.RPM)})
			}
			rl.EndpointLimits = &lims
		}
		c.RateLimiting = rl
	}
	if s.WAF != nil {
		w := &apiclient.WAFConfig{Enabled: s.WAF.Enabled, Mode: apiclient.SecurityMode(s.WAF.Mode), Preset: apiclient.WAFPreset(s.WAF.Preset), ParanoiaLevel: int(s.WAF.ParanoiaLevel)}
		if cat := s.WAF.Categories; cat != nil {
			w.EnabledCategories = &apiclient.WAFCategories{
				CrossSiteScripting: cat.CrossSiteScripting, DataLeakage: cat.DataLeakage, JavaAttacks: cat.JavaAttacks,
				LocalFileInclusion: cat.LocalFileInclusion, PhpInjection: cat.PhpInjection, ProtocolAttacks: cat.ProtocolAttacks,
				ProtocolEnforcement: cat.ProtocolEnforcement, RemoteCodeExecution: cat.RemoteCodeExecution, RemoteFileInclusion: cat.RemoteFileInclusion,
				SessionFixation: cat.SessionFixation, SqlInjection: cat.SQLInjection, WebshellDetection: cat.WebshellDetection,
			}
		}
		if len(s.WAF.ExclusionPaths) > 0 || len(s.WAF.RuleExclusions) > 0 {
			ex := &apiclient.WAFExclusions{}
			if len(s.WAF.ExclusionPaths) > 0 {
				p := s.WAF.ExclusionPaths
				ex.Paths = &p
			}
			if len(s.WAF.RuleExclusions) > 0 {
				rules := make([]apiclient.WAFRuleExclusion, 0, len(s.WAF.RuleExclusions))
				for _, re := range s.WAF.RuleExclusions {
					rule := apiclient.WAFRuleExclusion{RuleIds: re.RuleIDs}
					if len(re.Paths) > 0 {
						p := re.Paths
						rule.Paths = &p
					}
					rules = append(rules, rule)
				}
				ex.Rules = &rules
			}
			w.Exclusions = ex
		}
		c.Waf = w
	}
	if s.GeoBlocking != nil {
		c.GeoBlocking = &apiclient.GeoBlockingConfig{Enabled: s.GeoBlocking.Enabled, Mode: apiclient.GeoBlockingMode(s.GeoBlocking.Mode), Countries: s.GeoBlocking.Countries}
	}
	if s.BotManagement != nil {
		b := &apiclient.BotManagementConfig{Enabled: s.BotManagement.Enabled, Mode: apiclient.SecurityMode(s.BotManagement.Mode), ChallengeMode: apiclient.BotChallengeMode(s.BotManagement.ChallengeMode)}
		if len(s.BotManagement.WhitelistedAgents) > 0 {
			w := s.BotManagement.WhitelistedAgents
			b.WhitelistedAgents = &w
		}
		if len(s.BotManagement.BlacklistedAgents) > 0 {
			bl := s.BotManagement.BlacklistedAgents
			b.BlacklistedAgents = &bl
		}
		c.BotManagement = b
	}
}

// GetClusterSecurity returns a cluster's edge-security configuration.
func (c *Client) GetClusterSecurity(ctx context.Context, clusterID string) (*ClusterSecurity, error) {
	resp, err := c.gen.GetClusterSecurityWithResponse(ctx, cid(clusterID))
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return securityFromAPI(resp.JSON200), nil
}

// UpdateClusterSecurity overlays the managed sections onto the cluster's current
// security config (preserving CAPTCHA and any unmanaged fields) and saves it.
func (c *Client) UpdateClusterSecurity(ctx context.Context, clusterID string, sec *ClusterSecurity) (*ClusterSecurity, error) {
	cur, err := c.gen.GetClusterSecurityWithResponse(ctx, cid(clusterID))
	if err != nil {
		return nil, err
	}
	body := apiclient.ClusterSecurityConfig{}
	if cur.JSON200 != nil {
		body = *cur.JSON200
	}
	// Clear the sections this facade manages, then re-apply from the desired state.
	body.IpAccessControl, body.RateLimiting, body.Waf, body.GeoBlocking, body.BotManagement = nil, nil, nil, nil, nil
	sec.applyToAPI(&body)

	resp, err := c.gen.UpdateClusterSecurityWithResponse(ctx, cid(clusterID), apiclient.UpdateClusterSecurityJSONRequestBody(body))
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return securityFromAPI(resp.JSON200), nil
}
