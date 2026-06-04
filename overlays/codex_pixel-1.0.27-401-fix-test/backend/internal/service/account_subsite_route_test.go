package service

import "testing"

func TestClassifyAccountSubsiteRoutePolicy(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    string
	}{
		{
			name:    "apikey goes master direct",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test"}},
			want:    AccountSubsiteRoutePolicyMasterDirect,
		},
		{
			name:    "upstream goes master direct",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeUpstream, Credentials: map[string]any{"base_url": "https://relay.example.com"}},
			want:    AccountSubsiteRoutePolicyMasterDirect,
		},
		{
			name:    "oauth official account goes subsite relay",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			want:    AccountSubsiteRoutePolicySubsiteRelay,
		},
		{
			name:    "oauth custom base url goes master direct",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"custom_base_url": "https://relay.example.com"}},
			want:    AccountSubsiteRoutePolicyMasterDirect,
		},
		{
			name:    "explicit subsite relay wins",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"subsite_route_policy": AccountSubsiteRoutePolicySubsiteRelay}},
			want:    AccountSubsiteRoutePolicySubsiteRelay,
		},
		{
			name:    "explicit master direct wins",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"subsite_route_policy": AccountSubsiteRoutePolicyMasterDirect}},
			want:    AccountSubsiteRoutePolicyMasterDirect,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := ClassifyAccountSubsiteRoutePolicy(tt.account)
			if got != tt.want {
				t.Fatalf("ClassifyAccountSubsiteRoutePolicy()=%q want %q", got, tt.want)
			}
		})
	}
}

func TestApplyAccountSubsiteRoutePolicyPersistsResolvedDecision(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}}
	if !ApplyAccountSubsiteRoutePolicy(account) {
		t.Fatal("expected route policy to change")
	}
	if got := AccountSubsiteRoutePolicyResolved(account); got != AccountSubsiteRoutePolicyMasterDirect {
		t.Fatalf("resolved policy=%q want %q", got, AccountSubsiteRoutePolicyMasterDirect)
	}
	if account.GetExtraString("subsite_route_policy_reason") == "" {
		t.Fatal("expected route policy reason")
	}
}
