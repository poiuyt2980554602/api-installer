package service

import (
	"context"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"time"
)

var defaultSubsiteRelayReconciler subsiteRelayReconciler

// RegisterSubsiteRelayReconciler connects account lifecycle changes to relay lease reconciliation.
func RegisterSubsiteRelayReconciler(reconciler subsiteRelayReconciler) {
	defaultSubsiteRelayReconciler = reconciler
}

func TriggerSubsiteRelayReconcile(reason string) {
	if defaultSubsiteRelayReconciler == nil {
		return
	}
	defaultSubsiteRelayReconciler.TriggerSubsiteRelayReconcile(reason)
}

func ShouldReconcileSubsiteRelayForAccountChange(before, after *Account) bool {
	if after == nil {
		return false
	}
	if before == nil {
		return after.IsActive() && after.Schedulable
	}
	if before.ID != after.ID {
		return true
	}
	if before.Platform != after.Platform ||
		before.Type != after.Type ||
		before.Status != after.Status ||
		before.Schedulable != after.Schedulable ||
		NormalizeAccountShareMode(before.ShareMode) != NormalizeAccountShareMode(after.ShareMode) ||
		NormalizeAccountShareStatus(before.ShareStatus) != NormalizeAccountShareStatus(after.ShareStatus) ||
		NormalizeAccountLevel(before.AccountLevel) != NormalizeAccountLevel(after.AccountLevel) ||
		before.ErrorMessage != after.ErrorMessage ||
		!sameOptionalInt64(before.OwnerUserID, after.OwnerUserID) ||
		!sameOptionalTime(before.ExpiresAt, after.ExpiresAt) ||
		before.AutoPauseOnExpired != after.AutoPauseOnExpired ||
		!sameOptionalTime(before.RateLimitResetAt, after.RateLimitResetAt) ||
		!sameOptionalTime(before.OverloadUntil, after.OverloadUntil) ||
		!sameOptionalTime(before.TempUnschedulableUntil, after.TempUnschedulableUntil) {
		return true
	}
	return !reflect.DeepEqual(normalizeInt64Set(before.GroupIDs), normalizeInt64Set(after.GroupIDs))
}

func (s *AccountLeaseService) StartSubsiteRelayReconciler() {
	if s == nil || s.reconcileCh == nil || s.reconcileStopCh == nil {
		return
	}
	s.reconcileStartOnce.Do(func() {
		s.reconcileWG.Add(1)
		go func() {
			defer s.reconcileWG.Done()
			var timer *time.Timer
			var timerC <-chan time.Time
			pendingReason := ""
			for {
				select {
				case reason := <-s.reconcileCh:
					if strings.TrimSpace(reason) != "" {
						pendingReason = strings.TrimSpace(reason)
					}
					if timer == nil {
						timer = time.NewTimer(DefaultSubsiteRelayReconcileDebounce)
						timerC = timer.C
					} else {
						if !timer.Stop() {
							select {
							case <-timer.C:
							default:
							}
						}
						timer.Reset(DefaultSubsiteRelayReconcileDebounce)
					}
				case <-timerC:
					s.runRelayReconcile(pendingReason)
					pendingReason = ""
					timerC = nil
					timer = nil
				case <-s.reconcileStopCh:
					if timer != nil {
						timer.Stop()
					}
					return
				}
			}
		}()
	})
}

func (s *AccountLeaseService) StopSubsiteRelayReconciler() {
	if s == nil || s.reconcileStopCh == nil {
		return
	}
	s.reconcileStopOnce.Do(func() {
		close(s.reconcileStopCh)
	})
	s.reconcileWG.Wait()
}

func (s *AccountLeaseService) TriggerSubsiteRelayReconcile(reason string) {
	if s == nil || s.reconcileCh == nil {
		return
	}
	select {
	case s.reconcileCh <- strings.TrimSpace(reason):
	default:
	}
}

func (s *AccountLeaseService) runRelayReconcile(reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultSubsiteMaintenanceRunTimeout)
	defer cancel()
	result, err := s.AutoDistributeRelayAccounts(ctx)
	if err != nil {
		slog.Warn("subsite_relay.reconcile_failed", "reason", reason, "error", err)
		return
	}
	if result.CreatedCount > 0 || result.ReleasedInvalidLeases > 0 {
		slog.Info("subsite_relay.reconciled",
			"reason", reason,
			"created_leases", result.CreatedCount,
			"released_invalid_leases", result.ReleasedInvalidLeases,
			"skipped_accounts", result.SkippedCount,
			"online_subsites", result.OnlineSubsites,
		)
	}
}

func sameOptionalInt64(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameOptionalTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func normalizeInt64Set(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
