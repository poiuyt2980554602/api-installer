package service

import (
	"net/url"
	"strings"
	"time"
)

const (
	AccountSubsiteRoutePolicyAuto          = "auto"
	AccountSubsiteRoutePolicyMasterDirect  = "master_direct"
	AccountSubsiteRoutePolicySubsiteRelay  = "subsite_relay"
	AccountSubsiteRoutePolicyLocalOnly     = "local_only"
	accountSubsiteRoutePolicyKey           = "subsite_route_policy"
	accountSubsiteRoutePolicyResolvedKey   = "subsite_route_policy_resolved"
	accountSubsiteRoutePolicyReasonKey     = "subsite_route_policy_reason"
	accountSubsiteRoutePolicyUpdatedAtKey  = "subsite_route_policy_updated_at"
)

func NormalizeAccountSubsiteRoutePolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case AccountSubsiteRoutePolicyMasterDirect:
		return AccountSubsiteRoutePolicyMasterDirect
	case AccountSubsiteRoutePolicySubsiteRelay:
		return AccountSubsiteRoutePolicySubsiteRelay
	case AccountSubsiteRoutePolicyLocalOnly:
		return AccountSubsiteRoutePolicyLocalOnly
	default:
		return AccountSubsiteRoutePolicyAuto
	}
}

func AccountSubsiteRoutePolicy(account *Account) string {
	if account == nil {
		return AccountSubsiteRoutePolicyAuto
	}
	return NormalizeAccountSubsiteRoutePolicy(account.GetExtraString(accountSubsiteRoutePolicyKey))
}

func AccountSubsiteRoutePolicyResolved(account *Account) string {
	if account == nil {
		return AccountSubsiteRoutePolicyLocalOnly
	}
	if explicit := AccountSubsiteRoutePolicy(account); explicit != AccountSubsiteRoutePolicyAuto {
		return explicit
	}
	if policy := NormalizeAccountSubsiteRoutePolicy(account.GetExtraString(accountSubsiteRoutePolicyResolvedKey)); policy != AccountSubsiteRoutePolicyAuto {
		return policy
	}
	resolved, _ := ClassifyAccountSubsiteRoutePolicy(account)
	return resolved
}

func AccountSubsiteRoutePolicyReason(account *Account) string {
	if account == nil {
		return "账号不存在"
	}
	if reason := strings.TrimSpace(account.GetExtraString(accountSubsiteRoutePolicyReasonKey)); reason != "" {
		return reason
	}
	_, reason := ClassifyAccountSubsiteRoutePolicy(account)
	return reason
}

func IsAccountSubsiteRelayEligible(account *Account) bool {
	return AccountSubsiteRoutePolicyResolved(account) == AccountSubsiteRoutePolicySubsiteRelay
}

func IsAccountMasterDirect(account *Account) bool {
	return AccountSubsiteRoutePolicyResolved(account) == AccountSubsiteRoutePolicyMasterDirect
}

func ClassifyAccountSubsiteRoutePolicy(account *Account) (string, string) {
	if account == nil {
		return AccountSubsiteRoutePolicyLocalOnly, "账号不存在"
	}
	explicit := AccountSubsiteRoutePolicy(account)
	switch explicit {
	case AccountSubsiteRoutePolicyMasterDirect:
		return AccountSubsiteRoutePolicyMasterDirect, "管理员设置为主站直连"
	case AccountSubsiteRoutePolicySubsiteRelay:
		return AccountSubsiteRoutePolicySubsiteRelay, "管理员设置为子站转发"
	case AccountSubsiteRoutePolicyLocalOnly:
		return AccountSubsiteRoutePolicyLocalOnly, "管理员设置为仅主站本地"
	}

	switch strings.ToLower(strings.TrimSpace(account.Type)) {
	case AccountTypeAPIKey, AccountTypeUpstream, AccountTypeBedrock, AccountTypeServiceAccount:
		return AccountSubsiteRoutePolicyMasterDirect, "外部 API Key / 上游透传账号由主站直连，不进入子站池"
	}
	if hasCustomSubsiteRouteBaseURL(account) {
		return AccountSubsiteRoutePolicyMasterDirect, "账号配置了自定义 Base URL / 上游地址，由主站直连"
	}
	if account.IsOAuth() {
		return AccountSubsiteRoutePolicySubsiteRelay, "OAuth / Setup Token 账号允许进入子站池"
	}
	return AccountSubsiteRoutePolicyLocalOnly, "账号类型暂不支持子站转发"
}

func ApplyAccountSubsiteRoutePolicy(account *Account) bool {
	if account == nil {
		return false
	}
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	resolved, reason := ClassifyAccountSubsiteRoutePolicy(account)
	changed := false
	if stringFromAny(account.Extra[accountSubsiteRoutePolicyKey]) == "" {
		account.Extra[accountSubsiteRoutePolicyKey] = AccountSubsiteRoutePolicyAuto
		changed = true
	}
	if stringFromAny(account.Extra[accountSubsiteRoutePolicyResolvedKey]) != resolved {
		account.Extra[accountSubsiteRoutePolicyResolvedKey] = resolved
		changed = true
	}
	if stringFromAny(account.Extra[accountSubsiteRoutePolicyReasonKey]) != reason {
		account.Extra[accountSubsiteRoutePolicyReasonKey] = reason
		changed = true
	}
	if changed {
		account.Extra[accountSubsiteRoutePolicyUpdatedAtKey] = nowRFC3339()
	}
	return changed
}

func hasCustomSubsiteRouteBaseURL(account *Account) bool {
	if account == nil {
		return false
	}
	for _, values := range []map[string]any{account.Credentials, account.Extra} {
		for _, key := range []string{
			"base_url",
			"api_base_url",
			"custom_base_url",
			"upstream",
			"upstream_url",
			"upstream_base_url",
			"upstream_endpoint",
		} {
			if isCustomSubsiteRouteURL(account.Platform, stringFromAny(values[key])) {
				return true
			}
		}
	}
	if account.getExtraBool("custom_base_url_enabled") {
		return true
	}
	return false
}

func isCustomSubsiteRouteURL(platform, raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return true
	}
	host := strings.ToLower(parsed.Hostname())
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case PlatformOpenAI:
		return host != "" && host != "api.openai.com" && host != "chatgpt.com"
	case PlatformAnthropic:
		return host != "" && host != "api.anthropic.com"
	case PlatformGemini:
		return host != "" && !strings.HasSuffix(host, "googleapis.com")
	default:
		return true
	}
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
