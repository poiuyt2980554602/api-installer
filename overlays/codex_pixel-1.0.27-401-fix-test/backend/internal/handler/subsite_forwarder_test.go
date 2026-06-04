package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestClassifySubsiteAuthorizationSelectionFailure(t *testing.T) {
	t.Run("lease miss is retryable", func(t *testing.T) {
		code, retryable := classifySubsiteAuthorizationSelectionFailure(
			http.StatusUnauthorized,
			[]byte(`{"error":{"code":"SUBSITE_AUTHORIZE_FAILED","message":"master returned 503: SUBSITE_NO_ACCOUNT_LEASE"}}`),
		)
		require.Equal(t, "SUBSITE_NO_ACCOUNT_LEASE", code)
		require.True(t, retryable)
	})

	t.Run("insufficient funds stays client error", func(t *testing.T) {
		code, retryable := classifySubsiteAuthorizationSelectionFailure(
			http.StatusUnauthorized,
			[]byte(`{"error":{"code":"SUBSITE_AUTHORIZE_FAILED","message":"master returned 403: QUOTA_RESERVATION_INSUFFICIENT_FUNDS"}}`),
		)
		require.Equal(t, "QUOTA_RESERVATION_INSUFFICIENT_FUNDS", code)
		require.False(t, retryable)
	})

	t.Run("unrelated unauthorized response is not relay selection failure", func(t *testing.T) {
		code, retryable := classifySubsiteAuthorizationSelectionFailure(
			http.StatusUnauthorized,
			[]byte(`{"error":{"message":"invalid api key"}}`),
		)
		require.Empty(t, code)
		require.False(t, retryable)
	})
}

func TestSubsiteSupportsForwardPlatform(t *testing.T) {
	require.True(t, subsiteSupportsForwardPlatform(service.Subsite{}, service.PlatformOpenAI))
	require.True(t, subsiteSupportsForwardPlatform(service.Subsite{Capabilities: []string{"relay"}}, service.PlatformOpenAI))
	require.True(t, subsiteSupportsForwardPlatform(service.Subsite{Capabilities: []string{"openai"}}, service.PlatformOpenAI))
	require.False(t, subsiteSupportsForwardPlatform(service.Subsite{Capabilities: []string{"anthropic"}}, service.PlatformOpenAI))
}

func TestPlatformForForwardPath(t *testing.T) {
	require.Equal(t, service.PlatformOpenAI, platformForForwardPath("/v1/responses"))
	require.Equal(t, service.PlatformOpenAI, platformForForwardPath("/v1/chat/completions"))
	require.Equal(t, service.PlatformOpenAI, platformForForwardPath("/v1/images/generations"))
	require.Equal(t, service.PlatformGemini, platformForForwardPath("/v1beta/models/gemini:generateContent"))
	require.Equal(t, service.PlatformAnthropic, platformForForwardPath("/v1/messages"))
}

func TestSubsiteForwarderRouteEligibility(t *testing.T) {
	t.Run("master direct only skips subsite forwarding", func(t *testing.T) {
		forwarder := &SubsiteForwarder{
			accountRepo: &subsiteForwardAccountRepoStub{accounts: []service.Account{
				{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
			}},
			accountRoutes: newSubsiteAccountRouteStore(subsiteForwardAccountRouteTTL),
		}

		ok, reason := forwarder.shouldUseMasterDirect(context.Background(), &subsiteForwardContext{GroupID: 10, Platform: service.PlatformOpenAI})

		require.True(t, ok)
		require.Contains(t, reason, "主站直连")
	})

	t.Run("relay account keeps subsite forwarding", func(t *testing.T) {
		forwarder := &SubsiteForwarder{
			accountRepo: &subsiteForwardAccountRepoStub{accounts: []service.Account{
				{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth},
			}},
			accountRoutes: newSubsiteAccountRouteStore(subsiteForwardAccountRouteTTL),
		}

		ok, _ := forwarder.shouldUseMasterDirect(context.Background(), &subsiteForwardContext{GroupID: 10, Platform: service.PlatformOpenAI})

		require.False(t, ok)
	})

	t.Run("mixed pool prefers subsite relay", func(t *testing.T) {
		forwarder := &SubsiteForwarder{
			accountRepo: &subsiteForwardAccountRepoStub{accounts: []service.Account{
				{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
				{ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth},
			}},
			accountRoutes: newSubsiteAccountRouteStore(subsiteForwardAccountRouteTTL),
		}

		ok, _ := forwarder.shouldUseMasterDirect(context.Background(), &subsiteForwardContext{GroupID: 10, Platform: service.PlatformOpenAI})

		require.False(t, ok)
	})
}

type subsiteForwardAccountRepoStub struct {
	service.AccountRepository
	accounts []service.Account
}

func (s *subsiteForwardAccountRepoStub) ListByGroup(context.Context, int64) ([]service.Account, error) {
	accounts := make([]service.Account, len(s.accounts))
	copy(accounts, s.accounts)
	for i := range accounts {
		accounts[i].Status = service.StatusActive
		accounts[i].Schedulable = true
		service.ApplyAccountSubsiteRoutePolicy(&accounts[i])
	}
	return accounts, nil
}
