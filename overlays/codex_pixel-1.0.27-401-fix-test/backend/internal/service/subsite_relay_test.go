package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildSubsiteRelayAdvice(t *testing.T) {
	items := BuildSubsiteRelayAdvice(&SubsiteForwardStats{
		TotalSubsites:     1,
		OnlineSubsites:    0,
		Events24h:         20,
		SuccessRate24h:    0.80,
		CacheHitRatio24h:  0.05,
		ActiveLeases:      0,
		ExpiringLeases24h: 2,
	})
	codes := make([]string, 0, len(items))
	for _, item := range items {
		codes = append(codes, item.Code)
	}
	require.Contains(t, codes, "NO_ONLINE_SUBSITE")
	require.Contains(t, codes, "NO_ACTIVE_LEASE")
	require.Contains(t, codes, "LOW_SUCCESS_RATE")
	require.Contains(t, codes, "LOW_CACHE_HIT")
	require.Contains(t, codes, "LEASE_EXPIRING")
}

func TestBuildSubsiteRelayAdviceHealthy(t *testing.T) {
	items := BuildSubsiteRelayAdvice(&SubsiteForwardStats{
		TotalSubsites:    2,
		OnlineSubsites:   2,
		ActiveLeases:     4,
		Events24h:        20,
		SuccessRate24h:   1,
		CacheHitRatio24h: 0.50,
	})
	require.Empty(t, items)
}
