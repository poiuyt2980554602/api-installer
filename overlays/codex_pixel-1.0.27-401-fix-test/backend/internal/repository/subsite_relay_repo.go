package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *subsiteRepository) recordSubsiteHealthSample(ctx context.Context, heartbeat *service.SubsiteHeartbeat, metadataJSON []byte) error {
	if heartbeat == nil {
		return nil
	}
	healthScore := 100
	if strings.EqualFold(strings.TrimSpace(heartbeat.Status), service.SubsiteStatusUnhealthy) {
		healthScore = 0
	}
	sampleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_, err := r.db.ExecContext(sampleCtx, `
		INSERT INTO subsite_health_samples (
			subsite_id, status, health_score, active_requests, queued_usage, qps,
			cpu_percent, memory_bytes, version, metadata, created_at
		)
		VALUES (
			$1::varchar, $2::varchar, $3::int, $4::int, $5::int, $6::double precision,
			$7::double precision, $8::bigint, $9::varchar, $10::jsonb, $11::timestamptz
		)
	`, heartbeat.SubsiteID, heartbeat.Status, healthScore, heartbeat.ActiveRequests, heartbeat.QueuedUsage,
		heartbeat.QPS, heartbeat.CPUPercent, heartbeat.MemoryBytes, heartbeat.Version, metadataJSON, heartbeat.ReportedAt)
	return err
}

func (r *subsiteRepository) GetForwardAffinity(ctx context.Context, key string) (*service.SubsiteForwardAffinity, error) {
	affinity, err := scanForwardAffinity(r.db.QueryRowContext(ctx, forwardAffinitySelectSQL()+`
		WHERE affinity_key = $1
		  AND deleted_at IS NULL
		  AND (locked = TRUE OR expires_at > NOW())
	`, strings.TrimSpace(key)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSubsiteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get subsite forward affinity: %w", err)
	}
	return affinity, nil
}

func (r *subsiteRepository) UpsertForwardAffinity(ctx context.Context, input service.UpsertSubsiteForwardAffinityInput) (*service.SubsiteForwardAffinity, error) {
	affinity, err := scanForwardAffinity(r.db.QueryRowContext(ctx, `
		INSERT INTO subsite_forward_affinities (
			affinity_key, affinity_type, subsite_id, lease_id, account_id, api_key_id,
			user_id, group_id, model, session_id, source, locked, last_reason,
			last_error, expires_at, hits, last_used_at
		)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, 0), NULLIF($6, 0),
			NULLIF($7, 0), NULLIF($8, 0), $9, $10, $11, $12, $13, $14, $15, 1, NOW())
		ON CONFLICT (affinity_key) DO UPDATE
		SET affinity_type = EXCLUDED.affinity_type,
			subsite_id = CASE WHEN subsite_forward_affinities.locked AND NOT EXCLUDED.locked THEN subsite_forward_affinities.subsite_id ELSE EXCLUDED.subsite_id END,
			lease_id = CASE WHEN subsite_forward_affinities.locked AND NOT EXCLUDED.locked THEN subsite_forward_affinities.lease_id ELSE EXCLUDED.lease_id END,
			account_id = CASE WHEN subsite_forward_affinities.locked AND NOT EXCLUDED.locked THEN subsite_forward_affinities.account_id ELSE EXCLUDED.account_id END,
			api_key_id = COALESCE(EXCLUDED.api_key_id, subsite_forward_affinities.api_key_id),
			user_id = COALESCE(EXCLUDED.user_id, subsite_forward_affinities.user_id),
			group_id = COALESCE(EXCLUDED.group_id, subsite_forward_affinities.group_id),
			model = EXCLUDED.model,
			session_id = EXCLUDED.session_id,
			source = CASE WHEN EXCLUDED.locked THEN EXCLUDED.source ELSE subsite_forward_affinities.source END,
			locked = subsite_forward_affinities.locked OR EXCLUDED.locked,
			hits = subsite_forward_affinities.hits + 1,
			last_reason = EXCLUDED.last_reason,
			last_error = EXCLUDED.last_error,
			expires_at = CASE WHEN subsite_forward_affinities.locked THEN subsite_forward_affinities.expires_at ELSE EXCLUDED.expires_at END,
			last_used_at = NOW(),
			updated_at = NOW(),
			deleted_at = NULL
		RETURNING id, affinity_key, affinity_type, subsite_id, COALESCE(lease_id, ''),
			COALESCE(account_id, 0), COALESCE(api_key_id, 0), COALESCE(user_id, 0),
			COALESCE(group_id, 0), model, session_id, source, locked, hits,
			last_reason, last_error, expires_at, last_used_at, created_at, updated_at, deleted_at
	`,
		input.Key,
		input.Type,
		input.SubsiteID,
		input.LeaseID,
		input.AccountID,
		input.APIKeyID,
		input.UserID,
		input.GroupID,
		input.Model,
		input.SessionID,
		input.Source,
		input.Locked,
		input.LastReason,
		input.LastError,
		input.ExpiresAt,
	))
	if err != nil {
		return nil, fmt.Errorf("upsert subsite forward affinity: %w", err)
	}
	return affinity, nil
}

func (r *subsiteRepository) DeleteForwardAffinity(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE subsite_forward_affinities
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("delete subsite forward affinity: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return service.ErrSubsiteNotFound
	}
	return nil
}

func (r *subsiteRepository) ListForwardAffinities(ctx context.Context, params pagination.PaginationParams, filter service.ListSubsiteForwardAffinitiesFilter) ([]service.SubsiteForwardAffinity, *pagination.PaginationResult, error) {
	where, args := forwardAffinityWhere(filter)
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subsite_forward_affinities WHERE "+where, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count subsite forward affinities: %w", err)
	}
	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.Limit()
	offset := (page - 1) * pageSize
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		%s
		WHERE %s
		ORDER BY locked DESC, last_used_at DESC
		LIMIT $%d OFFSET $%d
	`, forwardAffinitySelectSQL(), where, len(args)+1, len(args)+2), append(args, pageSize, offset)...)
	if err != nil {
		return nil, nil, fmt.Errorf("list subsite forward affinities: %w", err)
	}
	defer rows.Close()
	items := make([]service.SubsiteForwardAffinity, 0)
	for rows.Next() {
		item, err := scanForwardAffinity(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	pages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if pages < 1 {
		pages = 1
	}
	return items, &pagination.PaginationResult{Total: total, Page: page, PageSize: pageSize, Pages: pages}, nil
}

func (r *subsiteRepository) RecordForwardEvent(ctx context.Context, event *service.SubsiteForwardEvent) error {
	if event == nil {
		return service.ErrSubsiteInvalidInput
	}
	metadataJSON, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("marshal subsite forward event metadata: %w", err)
	}
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO subsite_forward_events (
			request_id, affinity_key, subsite_id, attempted_subsite_id, fallback_from,
			lease_id, account_id, api_key_id, user_id, group_id, model, session_id,
			method, path, status_code, latency_ms, request_bytes, response_bytes,
			reason, outcome, error, metadata
		)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, NULLIF($6, ''), NULLIF($7, 0),
			NULLIF($8, 0), NULLIF($9, 0), NULLIF($10, 0), $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21, $22)
		RETURNING id, created_at
	`,
		event.RequestID,
		event.AffinityKey,
		event.SubsiteID,
		event.AttemptedSubsiteID,
		event.FallbackFrom,
		event.LeaseID,
		event.AccountID,
		event.APIKeyID,
		event.UserID,
		event.GroupID,
		event.Model,
		event.SessionID,
		event.Method,
		event.Path,
		event.StatusCode,
		event.LatencyMS,
		event.RequestBytes,
		event.ResponseBytes,
		event.Reason,
		event.Outcome,
		event.Error,
		metadataJSON,
	).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert subsite forward event: %w", err)
	}
	if event.Outcome == "failed" || event.Outcome == "upstream_error" {
		_ = r.upsertCircuitBreaker(ctx, event)
	} else if event.Outcome == "success" || event.Outcome == "fallback" {
		_ = r.clearCircuitBreaker(ctx, "subsite", event.SubsiteID)
		if event.AccountID > 0 {
			_ = r.clearCircuitBreaker(ctx, "account", fmt.Sprint(event.AccountID))
		}
		if strings.TrimSpace(event.LeaseID) != "" {
			_ = r.clearCircuitBreaker(ctx, "lease", event.LeaseID)
		}
	}
	return nil
}

func (r *subsiteRepository) ListForwardEvents(ctx context.Context, params pagination.PaginationParams, filter service.ListSubsiteForwardEventsFilter) ([]service.SubsiteForwardEvent, *pagination.PaginationResult, error) {
	where, args := forwardEventWhere(filter)
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subsite_forward_events WHERE "+where, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count subsite forward events: %w", err)
	}
	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.Limit()
	offset := (page - 1) * pageSize
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		%s
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, forwardEventSelectSQL(), where, len(args)+1, len(args)+2), append(args, pageSize, offset)...)
	if err != nil {
		return nil, nil, fmt.Errorf("list subsite forward events: %w", err)
	}
	defer rows.Close()
	items := make([]service.SubsiteForwardEvent, 0)
	for rows.Next() {
		item, err := scanForwardEvent(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	pages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if pages < 1 {
		pages = 1
	}
	return items, &pagination.PaginationResult{Total: total, Page: page, PageSize: pageSize, Pages: pages}, nil
}

func (r *subsiteRepository) ForwardStats(ctx context.Context) (*service.SubsiteForwardStats, error) {
	stats := &service.SubsiteForwardStats{
		BySubsite:           []service.SubsiteForwardSiteStat{},
		CircuitBreakers:     []service.SubsiteCircuitBreaker{},
		LeaseDistribution:   []service.SubsiteRelayLeaseStat{},
		PoolDistribution:    []service.SubsiteRelayPoolStat{},
		AccountDistribution: []service.SubsiteRelayAccountStat{},
		ConfigChecks:        []service.SubsiteRelayConfigCheck{},
	}
	var cacheableInputTokens24h int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM subsite_forward_affinities WHERE deleted_at IS NULL),
			(SELECT COUNT(*) FROM subsite_forward_affinities WHERE deleted_at IS NULL AND locked = TRUE),
			(SELECT COUNT(*) FROM subsite_forward_affinities WHERE deleted_at IS NULL AND (locked = TRUE OR expires_at > NOW())),
			(SELECT COUNT(*) FROM subsite_forward_affinities WHERE deleted_at IS NULL AND COALESCE(account_id, 0) > 0),
			(SELECT COUNT(*) FROM subsite_forward_events WHERE created_at >= NOW() - INTERVAL '24 hours'),
			(SELECT COUNT(*) FROM subsite_forward_events WHERE created_at >= NOW() - INTERVAL '24 hours' AND outcome IN ('failed', 'no_candidate', 'client_error', 'upstream_error')),
			(SELECT COUNT(*) FROM subsite_forward_events WHERE created_at >= NOW() - INTERVAL '24 hours' AND outcome = 'fallback'),
			(SELECT COUNT(*) FROM subsite_circuit_breakers WHERE deleted_at IS NULL AND cooldown_until > NOW()),
			(SELECT COUNT(*) FROM subsites WHERE deleted_at IS NULL),
			(SELECT COUNT(*) FROM subsites WHERE deleted_at IS NULL AND status = 'active' AND last_heartbeat_at >= NOW() - INTERVAL '3 minutes'),
			(SELECT COUNT(*) FROM subsites WHERE deleted_at IS NULL AND status = 'active' AND last_heartbeat_at >= NOW() - INTERVAL '3 minutes' AND health_score < 80),
			(SELECT COUNT(*) FROM subsites WHERE deleted_at IS NULL AND (status IN ('unhealthy', 'pending') OR (status = 'active' AND (last_heartbeat_at IS NULL OR last_heartbeat_at < NOW() - INTERVAL '3 minutes')))),
			(SELECT COUNT(*) FROM account_leases WHERE deleted_at IS NULL AND status IN ('active', 'renewing')),
			(SELECT COUNT(*) FROM account_leases WHERE deleted_at IS NULL AND status IN ('active', 'renewing') AND expires_at > NOW() AND expires_at <= NOW() + INTERVAL '24 hours'),
			COALESCE((SELECT AVG(latency_ms)::float8 FROM subsite_forward_events WHERE created_at >= NOW() - INTERVAL '24 hours' AND latency_ms > 0), 0),
			COALESCE((SELECT (percentile_disc(0.95) WITHIN GROUP (ORDER BY latency_ms))::float8 FROM subsite_forward_events WHERE created_at >= NOW() - INTERVAL '24 hours' AND latency_ms > 0), 0),
			COALESCE((SELECT (percentile_disc(0.99) WITHIN GROUP (ORDER BY latency_ms))::float8 FROM subsite_forward_events WHERE created_at >= NOW() - INTERVAL '24 hours' AND latency_ms > 0), 0),
			COALESCE((SELECT AVG(ul.first_token_ms)::float8 FROM usage_logs ul JOIN quota_reservations qr ON qr.request_id = ul.request_id WHERE ul.created_at >= NOW() - INTERVAL '24 hours' AND COALESCE(ul.first_token_ms, 0) > 0), 0),
			COALESCE((SELECT SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens + ul.image_output_tokens) FROM usage_logs ul JOIN quota_reservations qr ON qr.request_id = ul.request_id WHERE ul.created_at >= NOW() - INTERVAL '24 hours'), 0),
			COALESCE((SELECT SUM(ul.actual_cost) FROM usage_logs ul JOIN quota_reservations qr ON qr.request_id = ul.request_id WHERE ul.created_at >= NOW() - INTERVAL '24 hours'), 0),
			COALESCE((SELECT SUM(ul.cache_read_tokens) FROM usage_logs ul JOIN quota_reservations qr ON qr.request_id = ul.request_id WHERE ul.created_at >= NOW() - INTERVAL '24 hours'), 0),
			COALESCE((SELECT SUM(ul.input_tokens + ul.cache_read_tokens) FROM usage_logs ul JOIN quota_reservations qr ON qr.request_id = ul.request_id WHERE ul.created_at >= NOW() - INTERVAL '24 hours'), 0)
	`).Scan(
		&stats.TotalAffinities,
		&stats.LockedAffinity,
		&stats.ActiveAffinity,
		&stats.AccountAffinity,
		&stats.Events24h,
		&stats.Failures24h,
		&stats.Failovers24h,
		&stats.CircuitOpen,
		&stats.TotalSubsites,
		&stats.OnlineSubsites,
		&stats.DegradedSubsites,
		&stats.OfflineSubsites,
		&stats.ActiveLeases,
		&stats.ExpiringLeases24h,
		&stats.AvgLatencyMS24h,
		&stats.P95LatencyMS24h,
		&stats.P99LatencyMS24h,
		&stats.AvgFirstTokenMS24h,
		&stats.ForwardedTokens24h,
		&stats.ForwardedCost24h,
		&stats.CacheReadTokens24h,
		&cacheableInputTokens24h,
	); err != nil {
		return nil, fmt.Errorf("get subsite forward stats: %w", err)
	}
	if stats.Events24h > 0 {
		stats.SuccessRate24h = float64(stats.Events24h-stats.Failures24h) / float64(stats.Events24h)
	}
	if cacheableInputTokens24h > 0 {
		stats.CacheHitRatio24h = float64(stats.CacheReadTokens24h) / float64(cacheableInputTokens24h)
	}
	bySubsite, err := r.forwardSiteStats(ctx, true)
	if err != nil {
		return nil, err
	}
	stats.BySubsite = bySubsite
	breakers, err := r.ListActiveCircuitBreakers(ctx)
	if err != nil {
		return nil, err
	}
	stats.CircuitBreakers = breakers
	distribution, err := r.ListRelayLeaseDistribution(ctx)
	if err != nil {
		return nil, err
	}
	stats.LeaseDistribution = distribution
	pools, err := r.ListRelayPoolDistribution(ctx)
	if err != nil {
		return nil, err
	}
	stats.PoolDistribution = pools
	accountDistribution, err := r.ListRelayAccountDistribution(ctx)
	if err != nil {
		return nil, err
	}
	stats.AccountDistribution = accountDistribution
	checks, err := r.ListRelayConfigChecks(ctx)
	if err != nil {
		return nil, err
	}
	stats.ConfigChecks = checks
	stats.Automation = buildSubsiteRelayAutomationSummary(stats)
	return stats, nil
}

func (r *subsiteRepository) ForwardRouteStats(ctx context.Context) ([]service.SubsiteForwardSiteStat, error) {
	return r.forwardSiteStats(ctx, false)
}

func (r *subsiteRepository) ListRelayLeaseDistribution(ctx context.Context) ([]service.SubsiteRelayLeaseStat, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT al.group_id, COALESCE(NULLIF(g.name, ''), ''), g.platform,
			COALESCE(NULLIF(g.scope, ''), 'public'), COALESCE(g.required_account_level, ''),
			COUNT(*) AS active_leases,
			COUNT(DISTINCT al.subsite_id) AS assigned_subsites,
			COUNT(*) FILTER (WHERE al.expires_at <= NOW() + INTERVAL '1 hour') AS expiring_leases_1h,
			COUNT(*) FILTER (WHERE al.expires_at <= NOW() + INTERVAL '24 hours') AS expiring_leases_24h
		FROM account_leases al
		JOIN groups g ON g.id = al.group_id AND g.deleted_at IS NULL
		WHERE al.deleted_at IS NULL
		  AND al.status IN ('active', 'renewing')
		  AND al.expires_at > NOW()
		GROUP BY al.group_id, g.name, g.platform, g.scope, g.required_account_level
		ORDER BY g.platform ASC, g.required_account_level ASC, al.group_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list relay lease distribution: %w", err)
	}
	defer rows.Close()
	items := make([]service.SubsiteRelayLeaseStat, 0)
	for rows.Next() {
		var item service.SubsiteRelayLeaseStat
		if err := rows.Scan(
			&item.GroupID,
			&item.GroupName,
			&item.Platform,
			&item.Scope,
			&item.RequiredLevel,
			&item.ActiveLeases,
			&item.AssignedSubsites,
			&item.ExpiringLeases1h,
			&item.ExpiringLeases24h,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *subsiteRepository) ListRelayPoolDistribution(ctx context.Context) ([]service.SubsiteRelayPoolStat, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH effective_leases AS (
			SELECT account_id, group_id, subsite_id
			FROM account_leases
			WHERE deleted_at IS NULL
			  AND status IN ('active', 'renewing', 'draining')
			  AND expires_at > NOW()
		)
		SELECT
			g.id,
			COALESCE(NULLIF(g.name, ''), ''),
			g.platform,
			COALESCE(NULLIF(g.scope, ''), 'public'),
			COALESCE(g.required_account_level, ''),
			COUNT(DISTINCT a.id) AS total_accounts,
			COUNT(DISTINCT a.id) FILTER (
				WHERE COALESCE(
					NULLIF(a.extra->>'subsite_route_policy_resolved', ''),
					CASE
						WHEN a.type IN ('apikey', 'upstream', 'bedrock', 'service_account') THEN 'master_direct'
						WHEN a.type IN ('oauth', 'setup-token') THEN 'subsite_relay'
						ELSE 'local_only'
					END
				) = 'subsite_relay'
			) AS relay_eligible_accounts,
			COUNT(DISTINCT a.id) FILTER (
				WHERE COALESCE(
					NULLIF(a.extra->>'subsite_route_policy_resolved', ''),
					CASE
						WHEN a.type IN ('apikey', 'upstream', 'bedrock', 'service_account') THEN 'master_direct'
						WHEN a.type IN ('oauth', 'setup-token') THEN 'subsite_relay'
						ELSE 'local_only'
					END
				) = 'master_direct'
			) AS master_direct_accounts,
			COUNT(DISTINCT a.id) FILTER (
				WHERE COALESCE(
					NULLIF(a.extra->>'subsite_route_policy_resolved', ''),
					CASE
						WHEN a.type IN ('apikey', 'upstream', 'bedrock', 'service_account') THEN 'master_direct'
						WHEN a.type IN ('oauth', 'setup-token') THEN 'subsite_relay'
						ELSE 'local_only'
					END
				) = 'local_only'
			) AS local_only_accounts,
			COUNT(DISTINCT a.id) FILTER (
				WHERE a.status = 'active'
				  AND a.schedulable = TRUE
				  AND COALESCE(
					NULLIF(a.extra->>'subsite_route_policy_resolved', ''),
					CASE
						WHEN a.type IN ('apikey', 'upstream', 'bedrock', 'service_account') THEN 'master_direct'
						WHEN a.type IN ('oauth', 'setup-token') THEN 'subsite_relay'
						ELSE 'local_only'
					END
				  ) = 'subsite_relay'
				  AND (a.auto_pause_on_expired = FALSE OR a.expires_at IS NULL OR a.expires_at > NOW())
				  AND (a.overload_until IS NULL OR a.overload_until <= NOW())
				  AND (a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW())
				  AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW())
				  AND (
					(COALESCE(g.scope, 'public') = 'user_private'
					  AND a.owner_user_id IS NOT NULL
					  AND g.owner_user_id IS NOT NULL
					  AND a.owner_user_id = g.owner_user_id
					  AND COALESCE(a.share_mode, 'private') = 'private')
					OR
					(COALESCE(g.scope, 'public') <> 'user_private'
					  AND (
						a.owner_user_id IS NULL
						OR (COALESCE(a.share_mode, 'private') = 'public' AND COALESCE(a.share_status, 'approved') = 'approved')
					  ))
				  )
				  AND (
					g.platform <> 'openai'
					OR COALESCE(g.required_account_level, '') = ''
					OR CASE WHEN COALESCE(a.account_level, 'unknown') = 'team' THEN 'plus' ELSE COALESCE(a.account_level, 'unknown') END =
					   CASE WHEN g.required_account_level = 'team' THEN 'plus' ELSE g.required_account_level END
				  )
			) AS schedulable_accounts,
			COUNT(DISTINCT a.id) FILTER (
				WHERE a.status <> 'active'
				   OR a.schedulable = FALSE
				   OR (a.auto_pause_on_expired = TRUE AND a.expires_at IS NOT NULL AND a.expires_at <= NOW())
				   OR (a.overload_until IS NOT NULL AND a.overload_until > NOW())
				   OR (a.rate_limit_reset_at IS NOT NULL AND a.rate_limit_reset_at > NOW())
				   OR (a.temp_unschedulable_until IS NOT NULL AND a.temp_unschedulable_until > NOW())
			) AS unschedulable_accounts,
			COUNT(DISTINCT a.id) FILTER (WHERE COALESCE(a.share_mode, 'private') = 'public' AND COALESCE(a.share_status, 'approved') = 'pending') AS pending_accounts,
			COUNT(DISTINCT a.id) FILTER (WHERE COALESCE(a.share_mode, 'private') = 'public' AND COALESCE(a.share_status, 'approved') = 'suspended') AS suspended_accounts,
			COUNT(DISTINCT a.id) FILTER (WHERE a.rate_limit_reset_at IS NOT NULL AND a.rate_limit_reset_at > NOW()) AS rate_limited_accounts,
			COUNT(DISTINCT a.id) FILTER (
				WHERE (a.overload_until IS NOT NULL AND a.overload_until > NOW())
				   OR (a.temp_unschedulable_until IS NOT NULL AND a.temp_unschedulable_until > NOW())
			) AS temp_blocked_accounts,
			COUNT(DISTINCT a.id) FILTER (WHERE a.auto_pause_on_expired = TRUE AND a.expires_at IS NOT NULL AND a.expires_at <= NOW()) AS expired_accounts,
			COUNT(DISTINCT a.id) FILTER (WHERE g.platform = 'openai' AND COALESCE(a.account_level, 'unknown') = 'unknown') AS unknown_level_accounts,
			COUNT(DISTINCT a.id) FILTER (
				WHERE g.platform = 'openai'
				  AND COALESCE(g.required_account_level, '') <> ''
				  AND COALESCE(a.account_level, 'unknown') <> 'unknown'
				  AND CASE WHEN COALESCE(a.account_level, 'unknown') = 'team' THEN 'plus' ELSE COALESCE(a.account_level, 'unknown') END <>
					  CASE WHEN g.required_account_level = 'team' THEN 'plus' ELSE g.required_account_level END
			) AS level_mismatch_accounts,
			COUNT(DISTINCT a.id) FILTER (
				WHERE a.proxy_id IS NOT NULL
				  AND a.status = 'active'
				  AND a.schedulable = TRUE
				  AND COALESCE(
					NULLIF(a.extra->>'subsite_route_policy_resolved', ''),
					CASE
						WHEN a.type IN ('apikey', 'upstream', 'bedrock', 'service_account') THEN 'master_direct'
						WHEN a.type IN ('oauth', 'setup-token') THEN 'subsite_relay'
						ELSE 'local_only'
					END
				  ) = 'subsite_relay'
			) AS proxy_bound_accounts,
			COUNT(DISTINCT a.id) FILTER (
				WHERE a.proxy_id IS NULL
				  AND a.status = 'active'
				  AND a.schedulable = TRUE
				  AND COALESCE(
					NULLIF(a.extra->>'subsite_route_policy_resolved', ''),
					CASE
						WHEN a.type IN ('apikey', 'upstream', 'bedrock', 'service_account') THEN 'master_direct'
						WHEN a.type IN ('oauth', 'setup-token') THEN 'subsite_relay'
						ELSE 'local_only'
					END
				  ) = 'subsite_relay'
			) AS proxy_missing_accounts,
			COUNT(DISTINCT el.account_id) AS leased_accounts,
			GREATEST(
				COUNT(DISTINCT a.id) FILTER (
					WHERE a.status = 'active'
					  AND a.schedulable = TRUE
					  AND COALESCE(
						NULLIF(a.extra->>'subsite_route_policy_resolved', ''),
						CASE
							WHEN a.type IN ('apikey', 'upstream', 'bedrock', 'service_account') THEN 'master_direct'
							WHEN a.type IN ('oauth', 'setup-token') THEN 'subsite_relay'
							ELSE 'local_only'
						END
					  ) = 'subsite_relay'
					  AND (a.auto_pause_on_expired = FALSE OR a.expires_at IS NULL OR a.expires_at > NOW())
					  AND (a.overload_until IS NULL OR a.overload_until <= NOW())
					  AND (a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW())
					  AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW())
					  AND (
						(COALESCE(g.scope, 'public') = 'user_private'
						  AND a.owner_user_id IS NOT NULL
						  AND g.owner_user_id IS NOT NULL
						  AND a.owner_user_id = g.owner_user_id
						  AND COALESCE(a.share_mode, 'private') = 'private')
						OR
						(COALESCE(g.scope, 'public') <> 'user_private'
						  AND (
							a.owner_user_id IS NULL
							OR (COALESCE(a.share_mode, 'private') = 'public' AND COALESCE(a.share_status, 'approved') = 'approved')
						  ))
					  )
					  AND (
						g.platform <> 'openai'
						OR COALESCE(g.required_account_level, '') = ''
						OR CASE WHEN COALESCE(a.account_level, 'unknown') = 'team' THEN 'plus' ELSE COALESCE(a.account_level, 'unknown') END =
						   CASE WHEN g.required_account_level = 'team' THEN 'plus' ELSE g.required_account_level END
					  )
				) - COUNT(DISTINCT el.account_id),
				0
			) AS unleased_accounts,
			COUNT(DISTINCT el.subsite_id) AS assigned_subsites,
			COUNT(el.account_id) AS active_leases
		FROM groups g
		LEFT JOIN account_groups ag ON ag.group_id = g.id
		LEFT JOIN accounts a ON a.id = ag.account_id AND a.deleted_at IS NULL
		LEFT JOIN effective_leases el ON el.account_id = a.id AND el.group_id = g.id
		WHERE g.deleted_at IS NULL
		  AND g.status = 'active'
		GROUP BY g.id, g.name, g.platform, g.scope, g.required_account_level
		HAVING COUNT(DISTINCT a.id) > 0 OR COUNT(el.account_id) > 0
		ORDER BY g.platform ASC, COALESCE(NULLIF(g.scope, ''), 'public') ASC, COALESCE(g.required_account_level, '') ASC, g.id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list relay pool distribution: %w", err)
	}
	defer rows.Close()
	items := make([]service.SubsiteRelayPoolStat, 0)
	for rows.Next() {
		var item service.SubsiteRelayPoolStat
		if err := rows.Scan(
			&item.GroupID,
			&item.GroupName,
			&item.Platform,
			&item.Scope,
			&item.RequiredLevel,
			&item.TotalAccounts,
			&item.RelayEligibleAccounts,
			&item.MasterDirectAccounts,
			&item.LocalOnlyAccounts,
			&item.SchedulableAccounts,
			&item.UnschedulableAccounts,
			&item.PendingAccounts,
			&item.SuspendedAccounts,
			&item.RateLimitedAccounts,
			&item.TempBlockedAccounts,
			&item.ExpiredAccounts,
			&item.UnknownLevelAccounts,
			&item.LevelMismatchAccounts,
			&item.ProxyBoundAccounts,
			&item.ProxyMissingAccounts,
			&item.LeasedAccounts,
			&item.UnleasedAccounts,
			&item.AssignedSubsites,
			&item.ActiveLeases,
		); err != nil {
			return nil, err
		}
		item.BlockedReason = relayPoolBlockedReason(item)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *subsiteRepository) ListRelayAccountDistribution(ctx context.Context) ([]service.SubsiteRelayAccountStat, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH effective_leases AS (
			SELECT DISTINCT ON (al.account_id)
				al.account_id, al.group_id, al.subsite_id, al.lease_id, al.status, al.expires_at,
				COALESCE(s.name, '') AS subsite_name
			FROM account_leases al
			LEFT JOIN subsites s ON s.subsite_id = al.subsite_id AND s.deleted_at IS NULL
			WHERE al.deleted_at IS NULL
			  AND al.status IN ('active', 'renewing', 'draining')
			  AND al.expires_at > NOW()
			ORDER BY al.account_id, al.assigned_at DESC
		)
		SELECT
			a.id,
			COALESCE(NULLIF(a.name, ''), ''),
			a.platform,
			COALESCE(a.account_level, 'unknown'),
			COALESCE(a.share_mode, 'private'),
			COALESCE(a.share_status, 'approved'),
			a.status,
			a.schedulable,
			COALESCE(g.id, 0),
			COALESCE(NULLIF(g.name, ''), ''),
			COALESCE(NULLIF(g.scope, ''), 'public'),
			COALESCE(g.required_account_level, ''),
			COALESCE(NULLIF(a.extra->>'subsite_route_policy', ''), 'auto') AS route_policy,
			COALESCE(
				NULLIF(a.extra->>'subsite_route_policy_resolved', ''),
				CASE
					WHEN a.type IN ('apikey', 'upstream', 'bedrock', 'service_account') THEN 'master_direct'
					WHEN a.type IN ('oauth', 'setup-token') THEN 'subsite_relay'
					ELSE 'local_only'
				END
			) AS route_resolved,
			COALESCE(NULLIF(a.extra->>'subsite_route_policy_reason', ''), '') AS route_reason,
			COALESCE(a.proxy_id, 0),
			COALESCE(NULLIF(p.name, ''), ''),
			COALESCE(NULLIF(p.protocol, ''), ''),
			COALESCE(NULLIF(p.host, ''), ''),
			COALESCE(p.port, 0),
			el.lease_id IS NOT NULL AS distributed,
			(
				el.lease_id IS NULL
				AND g.id IS NOT NULL
				AND g.deleted_at IS NULL
				AND g.status = 'active'
				AND COALESCE(
					NULLIF(a.extra->>'subsite_route_policy_resolved', ''),
					CASE
						WHEN a.type IN ('apikey', 'upstream', 'bedrock', 'service_account') THEN 'master_direct'
						WHEN a.type IN ('oauth', 'setup-token') THEN 'subsite_relay'
						ELSE 'local_only'
					END
				) = 'subsite_relay'
				AND a.status = 'active'
				AND a.schedulable = TRUE
				AND g.platform = a.platform
				AND (a.auto_pause_on_expired = FALSE OR a.expires_at IS NULL OR a.expires_at > NOW())
				AND (a.overload_until IS NULL OR a.overload_until <= NOW())
				AND (a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= NOW())
				AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW())
				AND (
					(COALESCE(g.scope, 'public') = 'user_private'
					  AND a.owner_user_id IS NOT NULL
					  AND g.owner_user_id IS NOT NULL
					  AND a.owner_user_id = g.owner_user_id
					  AND COALESCE(a.share_mode, 'private') = 'private')
					OR
					(COALESCE(g.scope, 'public') <> 'user_private'
					  AND (
						a.owner_user_id IS NULL
						OR (COALESCE(a.share_mode, 'private') = 'public' AND COALESCE(a.share_status, 'approved') = 'approved')
					  ))
				)
				AND (
					g.platform <> 'openai'
					OR COALESCE(g.required_account_level, '') = ''
					OR (
						COALESCE(a.account_level, 'unknown') <> 'unknown'
						AND CASE WHEN COALESCE(a.account_level, 'unknown') = 'team' THEN 'plus' ELSE COALESCE(a.account_level, 'unknown') END =
							CASE WHEN g.required_account_level = 'team' THEN 'plus' ELSE g.required_account_level END
					)
				)
			) AS distributable,
			COALESCE(el.subsite_id, ''),
			COALESCE(el.subsite_name, ''),
			COALESCE(el.lease_id, ''),
			COALESCE(el.status, ''),
			CASE
				WHEN el.lease_id IS NOT NULL THEN 'DISTRIBUTED'
				WHEN COALESCE(
					NULLIF(a.extra->>'subsite_route_policy_resolved', ''),
					CASE
						WHEN a.type IN ('apikey', 'upstream', 'bedrock', 'service_account') THEN 'master_direct'
						WHEN a.type IN ('oauth', 'setup-token') THEN 'subsite_relay'
						ELSE 'local_only'
					END
				) = 'master_direct' THEN 'MASTER_DIRECT'
				WHEN COALESCE(
					NULLIF(a.extra->>'subsite_route_policy_resolved', ''),
					CASE
						WHEN a.type IN ('apikey', 'upstream', 'bedrock', 'service_account') THEN 'master_direct'
						WHEN a.type IN ('oauth', 'setup-token') THEN 'subsite_relay'
						ELSE 'local_only'
					END
				) = 'local_only' THEN 'LOCAL_ONLY'
				WHEN g.id IS NULL THEN 'GROUP_NOT_BOUND'
				WHEN g.deleted_at IS NOT NULL OR g.status <> 'active' THEN 'GROUP_INACTIVE'
				WHEN a.status <> 'active' THEN 'ACCOUNT_INACTIVE'
				WHEN a.schedulable = FALSE THEN 'ACCOUNT_UNSCHEDULABLE'
				WHEN a.auto_pause_on_expired = TRUE AND a.expires_at IS NOT NULL AND a.expires_at <= NOW() THEN 'ACCOUNT_EXPIRED'
				WHEN a.overload_until IS NOT NULL AND a.overload_until > NOW() THEN 'ACCOUNT_OVERLOADED'
				WHEN a.rate_limit_reset_at IS NOT NULL AND a.rate_limit_reset_at > NOW() THEN 'ACCOUNT_RATE_LIMITED'
				WHEN a.temp_unschedulable_until IS NOT NULL AND a.temp_unschedulable_until > NOW() THEN 'ACCOUNT_TEMP_BLOCKED'
				WHEN g.platform <> a.platform THEN 'PLATFORM_MISMATCH'
				WHEN COALESCE(g.scope, 'public') = 'user_private'
					AND (a.owner_user_id IS NULL OR g.owner_user_id IS NULL OR a.owner_user_id <> g.owner_user_id) THEN 'PRIVATE_OWNER_MISMATCH'
				WHEN COALESCE(g.scope, 'public') = 'user_private'
					AND COALESCE(a.share_mode, 'private') <> 'private' THEN 'PRIVATE_SHARE_MODE_MISMATCH'
				WHEN COALESCE(g.scope, 'public') <> 'user_private'
					AND a.owner_user_id IS NOT NULL
					AND (COALESCE(a.share_mode, 'private') <> 'public' OR COALESCE(a.share_status, 'approved') <> 'approved') THEN 'PUBLIC_SHARE_NOT_APPROVED'
				WHEN g.platform = 'openai'
					AND COALESCE(g.required_account_level, '') <> ''
					AND COALESCE(a.account_level, 'unknown') = 'unknown' THEN 'ACCOUNT_LEVEL_UNKNOWN'
				WHEN g.platform = 'openai'
					AND COALESCE(g.required_account_level, '') <> ''
					AND CASE WHEN COALESCE(a.account_level, 'unknown') = 'team' THEN 'plus' ELSE COALESCE(a.account_level, 'unknown') END <>
						CASE WHEN g.required_account_level = 'team' THEN 'plus' ELSE g.required_account_level END THEN 'ACCOUNT_LEVEL_MISMATCH'
				ELSE 'READY_TO_DISTRIBUTE'
			END AS reason_code,
			a.updated_at,
			el.expires_at
		FROM accounts a
		LEFT JOIN account_groups ag ON ag.account_id = a.id
		LEFT JOIN groups g ON g.id = ag.group_id
		LEFT JOIN proxies p ON p.id = a.proxy_id
		LEFT JOIN effective_leases el ON el.account_id = a.id AND (g.id IS NULL OR el.group_id = g.id)
		WHERE a.deleted_at IS NULL
		  AND (
			ag.group_id IS NOT NULL
			OR a.owner_user_id IS NOT NULL
			OR el.lease_id IS NOT NULL
		  )
		ORDER BY
			(el.lease_id IS NOT NULL) DESC,
			a.updated_at DESC,
			a.id DESC,
			COALESCE(g.id, 0) ASC
		LIMIT 300
	`)
	if err != nil {
		return nil, fmt.Errorf("list relay account distribution: %w", err)
	}
	defer rows.Close()
	items := make([]service.SubsiteRelayAccountStat, 0)
	for rows.Next() {
		var item service.SubsiteRelayAccountStat
		var leaseExpiresAt sql.NullTime
		if err := rows.Scan(
			&item.AccountID,
			&item.AccountName,
			&item.Platform,
			&item.AccountLevel,
			&item.ShareMode,
			&item.ShareStatus,
			&item.Status,
			&item.Schedulable,
			&item.GroupID,
			&item.GroupName,
			&item.GroupScope,
			&item.RequiredLevel,
			&item.RoutePolicy,
			&item.RouteResolved,
			&item.RouteReason,
			&item.ProxyID,
			&item.ProxyName,
			&item.ProxyProtocol,
			&item.ProxyHost,
			&item.ProxyPort,
			&item.Distributed,
			&item.Distributable,
			&item.SubsiteID,
			&item.SubsiteName,
			&item.LeaseID,
			&item.LeaseStatus,
			&item.ReasonCode,
			&item.UpdatedAt,
			&leaseExpiresAt,
		); err != nil {
			return nil, err
		}
		if leaseExpiresAt.Valid {
			item.LeaseExpiresAt = &leaseExpiresAt.Time
		}
		item.Reason = relayAccountDistributionReason(item.ReasonCode)
		if item.RouteReason != "" && (item.ReasonCode == "MASTER_DIRECT" || item.ReasonCode == "LOCAL_ONLY") {
			item.Reason = item.RouteReason
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *subsiteRepository) ListRelayConfigChecks(ctx context.Context) ([]service.SubsiteRelayConfigCheck, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH metrics AS (
			SELECT
				(SELECT COUNT(*) FROM account_share_policies
				 WHERE deleted_at IS NULL
				   AND enabled = TRUE
				   AND effective_at <= NOW()
				   AND scope_type = 'global'
				   AND owner_share_ratio > 0) AS global_share_policy,
				(SELECT COUNT(*) FROM subsites
				 WHERE deleted_at IS NULL
				   AND status = 'active'
				   AND last_heartbeat_at >= NOW() - INTERVAL '3 minutes') AS online_subsites,
				(SELECT COUNT(*) FROM groups
				 WHERE deleted_at IS NULL
				   AND status = 'active'
				   AND platform = 'openai'
				   AND COALESCE(scope, 'public') = 'public'
				   AND required_account_level = 'free') AS public_openai_free_group,
				(SELECT COUNT(*) FROM groups
				 WHERE deleted_at IS NULL
				   AND status = 'active'
				   AND platform = 'openai'
				   AND COALESCE(scope, 'public') = 'public'
				   AND required_account_level = 'plus') AS public_openai_plus_group,
				(SELECT COUNT(*) FROM groups
				 WHERE deleted_at IS NULL
				   AND status = 'active'
				   AND platform = 'openai'
				   AND COALESCE(scope, 'public') = 'public'
				   AND required_account_level = 'pro') AS public_openai_pro_group,
				(SELECT COUNT(*)
				 FROM (
					SELECT account_id
					FROM account_leases
					WHERE deleted_at IS NULL
					  AND status IN ('active', 'renewing', 'draining')
					  AND expires_at > NOW()
					GROUP BY account_id
					HAVING COUNT(*) > 1
				 ) dup) AS duplicate_effective_lease,
				(SELECT COUNT(*)
				 FROM accounts
				 WHERE deleted_at IS NULL
				   AND share_mode = 'public'
				   AND share_status = 'pending'
				   AND COALESCE(NULLIF(error_message, ''), '') = '') AS pending_without_reason,
				(SELECT COUNT(*)
				 FROM account_leases al
				 JOIN accounts a ON a.id = al.account_id
				 JOIN groups g ON g.id = al.group_id
				 WHERE al.deleted_at IS NULL
				   AND al.status IN ('active', 'renewing', 'draining')
				   AND al.expires_at > NOW()
				   AND g.platform = 'openai'
				   AND COALESCE(g.required_account_level, '') <> ''
				   AND CASE WHEN COALESCE(a.account_level, 'unknown') = 'team' THEN 'plus' ELSE COALESCE(a.account_level, 'unknown') END <>
					   CASE WHEN g.required_account_level = 'team' THEN 'plus' ELSE g.required_account_level END) AS level_mismatch_lease,
				(SELECT COUNT(*)
				 FROM account_leases al
				 JOIN accounts a ON a.id = al.account_id
				 JOIN groups g ON g.id = al.group_id
				 WHERE al.deleted_at IS NULL
				   AND al.status IN ('active', 'renewing', 'draining')
				   AND al.expires_at > NOW()
				   AND (
					(COALESCE(g.scope, 'public') = 'user_private'
					  AND (a.owner_user_id IS NULL OR g.owner_user_id IS NULL OR a.owner_user_id <> g.owner_user_id OR COALESCE(a.share_mode, 'private') <> 'private'))
					OR
					(COALESCE(g.scope, 'public') <> 'user_private'
					  AND a.owner_user_id IS NOT NULL
					  AND (COALESCE(a.share_mode, 'private') <> 'public' OR COALESCE(a.share_status, 'approved') <> 'approved'))
				   )) AS share_mode_mismatch_lease
		)
		SELECT code, ok, severity, message
		FROM metrics m
		CROSS JOIN LATERAL (
			VALUES
				('GLOBAL_SHARE_POLICY', m.global_share_policy > 0, 'warning', '没有启用全局账号分润策略。新提交的公有账号可能会一直停在待审核，请先在分润策略里配置全局策略。'),
				('ONLINE_SUBSITE', m.online_subsites > 0, 'critical', '最近 3 分钟没有在线子站心跳。主站无法选择健康子站，也就无法分发账号。'),
				('PUBLIC_OPENAI_FREE_GROUP', m.public_openai_free_group > 0, 'warning', '没有启用的 OpenAI Free 公共分组。Free 公有账号无法进入正确价格池。'),
				('PUBLIC_OPENAI_PLUS_GROUP', m.public_openai_plus_group > 0, 'warning', '没有启用的 OpenAI Plus 公共分组。Plus 公有账号无法进入正确价格池。'),
				('PUBLIC_OPENAI_PRO_GROUP', m.public_openai_pro_group > 0, 'warning', '没有启用的 OpenAI Pro 公共分组。Pro 公有账号无法进入正确价格池。'),
				('DUPLICATE_EFFECTIVE_LEASE', m.duplicate_effective_lease = 0, 'critical', '存在一个账号同时拥有多个有效租约。必须保证一个账号同一时间只在一个子站池里。'),
				('PENDING_WITHOUT_REASON', m.pending_without_reason = 0, 'info', '有公有账号待审核但没有失败原因。请重新校验账号，让系统写入具体阻断原因。'),
				('LEVEL_MISMATCH_LEASE', m.level_mismatch_lease = 0, 'critical', '存在 OpenAI 租约等级与分组要求不一致。请释放这些无效租约后重新分发。'),
				('SHARE_MODE_MISMATCH_LEASE', m.share_mode_mismatch_lease = 0, 'critical', '存在租约与公有/私有模式冲突。请释放这些无效租约后重新分发。')
		) AS checks(code, ok, severity, message)
	`)
	if err != nil {
		return nil, fmt.Errorf("list relay config checks: %w", err)
	}
	defer rows.Close()
	items := make([]service.SubsiteRelayConfigCheck, 0)
	for rows.Next() {
		var item service.SubsiteRelayConfigCheck
		var ok bool
		if err := rows.Scan(&item.Code, &ok, &item.Severity, &item.Message); err != nil {
			return nil, err
		}
		if ok {
			item.Status = "ok"
			item.Severity = "info"
			item.Message = relayConfigCheckOKMessage(item.Code)
		} else {
			item.Status = "failed"
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *subsiteRepository) forwardSiteStats(ctx context.Context, includeUsage bool) ([]service.SubsiteForwardSiteStat, error) {
	usageJoin := ""
	usageSelect := `
		0::bigint AS forwarded_tokens,
		0::float8 AS forwarded_cost,
		0::bigint AS cache_read_tokens,
		0::bigint AS cacheable_input_tokens,
		0::float8 AS avg_first_token_ms`
	if includeUsage {
		usageJoin = `
		usage_stats AS (
			SELECT qr.subsite_id,
				COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens + ul.image_output_tokens), 0) AS forwarded_tokens,
				COALESCE(SUM(ul.actual_cost), 0)::float8 AS forwarded_cost,
				COALESCE(SUM(ul.cache_read_tokens), 0) AS cache_read_tokens,
				COALESCE(SUM(ul.input_tokens + ul.cache_read_tokens), 0) AS cacheable_input_tokens,
				COALESCE(AVG(ul.first_token_ms) FILTER (WHERE ul.first_token_ms > 0), 0)::float8 AS avg_first_token_ms
			FROM usage_logs ul
			JOIN quota_reservations qr ON qr.request_id = ul.request_id
			WHERE ul.created_at >= NOW() - INTERVAL '24 hours'
			GROUP BY qr.subsite_id
		),`
		usageSelect = `
			COALESCE(us.forwarded_tokens, 0),
			COALESCE(us.forwarded_cost, 0),
			COALESCE(us.cache_read_tokens, 0),
			COALESCE(us.cacheable_input_tokens, 0),
			COALESCE(us.avg_first_token_ms, 0)`
	}
	usageLeftJoin := ""
	if includeUsage {
		usageLeftJoin = "LEFT JOIN usage_stats us ON us.subsite_id = s.subsite_id"
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		WITH latest_hb AS (
			SELECT DISTINCT ON (subsite_id)
				subsite_id, active_requests, queued_usage, qps, cpu_percent, memory_bytes
			FROM subsite_heartbeats
			ORDER BY subsite_id, created_at DESC
		),
		event_stats AS (
			SELECT subsite_id,
				COUNT(*) AS events_24h,
				COUNT(*) FILTER (WHERE outcome IN ('failed', 'no_candidate', 'client_error', 'upstream_error')) AS failures_24h,
				COALESCE(AVG(latency_ms) FILTER (WHERE latency_ms > 0), 0)::float8 AS avg_latency_ms,
				COALESCE(percentile_disc(0.95) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE latency_ms > 0), 0)::float8 AS p95_latency_ms,
				COALESCE(percentile_disc(0.99) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE latency_ms > 0), 0)::float8 AS p99_latency_ms
			FROM subsite_forward_events
			WHERE created_at >= NOW() - INTERVAL '24 hours'
			GROUP BY subsite_id
		),
		%s
		affinity_stats AS (
			SELECT subsite_id,
				COUNT(*) AS affinities,
				COUNT(*) FILTER (WHERE locked = TRUE) AS locked_affinities
			FROM subsite_forward_affinities
			WHERE deleted_at IS NULL
			GROUP BY subsite_id
		),
		circuit_stats AS (
			SELECT DISTINCT ON (subsite_id)
				subsite_id, reason, cooldown_until
			FROM subsite_circuit_breakers
			WHERE deleted_at IS NULL
			  AND cooldown_until > NOW()
			  AND scope = 'subsite'
			ORDER BY subsite_id, updated_at DESC
		),
		lease_stats AS (
			SELECT subsite_id,
				COUNT(*) FILTER (WHERE status IN ('active', 'renewing')) AS active_leases,
				COUNT(*) FILTER (WHERE status IN ('active', 'renewing') AND expires_at > NOW() AND expires_at <= NOW() + INTERVAL '24 hours') AS expiring_leases_24h
			FROM account_leases
			WHERE deleted_at IS NULL
			GROUP BY subsite_id
		)
		SELECT s.subsite_id, s.name, s.status, s.health_score, s.last_heartbeat_at,
			COALESCE(h.active_requests, 0), COALESCE(h.queued_usage, 0),
			COALESCE(h.qps, 0), COALESCE(h.cpu_percent, 0), COALESCE(h.memory_bytes, 0),
			COALESCE(ls.active_leases, 0), COALESCE(ls.expiring_leases_24h, 0),
			COALESCE(es.events_24h, 0), COALESCE(es.failures_24h, 0), COALESCE(es.avg_latency_ms, 0),
			COALESCE(es.p95_latency_ms, 0), COALESCE(es.p99_latency_ms, 0),
			%s,
			COALESCE(afs.affinities, 0), COALESCE(afs.locked_affinities, 0),
			cs.subsite_id IS NOT NULL, COALESCE(cs.reason, ''), cs.cooldown_until
		FROM subsites s
		LEFT JOIN latest_hb h ON h.subsite_id = s.subsite_id
		LEFT JOIN event_stats es ON es.subsite_id = s.subsite_id
		%s
		LEFT JOIN affinity_stats afs ON afs.subsite_id = s.subsite_id
		LEFT JOIN circuit_stats cs ON cs.subsite_id = s.subsite_id
		LEFT JOIN lease_stats ls ON ls.subsite_id = s.subsite_id
		WHERE s.deleted_at IS NULL
		ORDER BY s.status = 'active' DESC, s.health_score DESC, s.name ASC
	`, usageJoin, usageSelect, usageLeftJoin))
	if err != nil {
		return nil, fmt.Errorf("list subsite forward site stats: %w", err)
	}
	defer rows.Close()
	items := make([]service.SubsiteForwardSiteStat, 0)
	now := time.Now()
	for rows.Next() {
		var item service.SubsiteForwardSiteStat
		var cacheableInputTokens int64
		if err := rows.Scan(
			&item.SubsiteID,
			&item.Name,
			&item.Status,
			&item.HealthScore,
			&item.LastHeartbeatAt,
			&item.ActiveRequests,
			&item.QueuedUsage,
			&item.QPS,
			&item.CPUPercent,
			&item.MemoryBytes,
			&item.ActiveLeases,
			&item.ExpiringLeases24h,
			&item.Events24h,
			&item.Failures24h,
			&item.AvgLatencyMS24h,
			&item.P95LatencyMS24h,
			&item.P99LatencyMS24h,
			&item.ForwardedTokens24h,
			&item.ForwardedCost24h,
			&item.CacheReadTokens24h,
			&cacheableInputTokens,
			&item.AvgFirstTokenMS24h,
			&item.Affinities,
			&item.LockedAffinity,
			&item.CircuitOpen,
			&item.CircuitReason,
			&item.CooldownUntil,
		); err != nil {
			return nil, err
		}
		if item.Events24h > 0 {
			item.SuccessRate24h = float64(item.Events24h-item.Failures24h) / float64(item.Events24h)
		}
		if cacheableInputTokens > 0 {
			item.CacheHitRatio24h = float64(item.CacheReadTokens24h) / float64(cacheableInputTokens)
		}
		item.EffectiveStatus = service.SubsiteEffectiveStatusForStats(item, now)
		item.LoadLevel = service.SubsiteLoadLevelForStats(item)
		items = append(items, item)
	}
	return items, rows.Err()
}

func buildSubsiteRelayAutomationSummary(stats *service.SubsiteForwardStats) service.SubsiteRelayAutomationSummary {
	if stats == nil {
		return service.SubsiteRelayAutomationSummary{}
	}
	summary := service.SubsiteRelayAutomationSummary{
		ConfigOK:       true,
		OnlineSubsites: stats.OnlineSubsites,
		LeasedAccounts: stats.ActiveLeases,
	}
	for _, check := range stats.ConfigChecks {
		if check.Status != "ok" && check.Severity == "critical" {
			summary.ConfigOK = false
		}
	}
	for _, pool := range stats.PoolDistribution {
		if pool.Scope == service.GroupScopeUserPrivate {
			summary.PrivatePoolAccounts += pool.TotalAccounts
		} else {
			summary.PublicPoolAccounts += pool.TotalAccounts
		}
		summary.SchedulableAccounts += pool.SchedulableAccounts
		summary.UnleasedAccounts += pool.UnleasedAccounts
		summary.PendingAccounts += pool.PendingAccounts
		summary.BlockedAccounts += pool.UnschedulableAccounts + pool.SuspendedAccounts + pool.LevelMismatchAccounts
	}
	summary.Ready = summary.ConfigOK && summary.OnlineSubsites > 0 && summary.SchedulableAccounts > 0
	return summary
}

func relayPoolBlockedReason(item service.SubsiteRelayPoolStat) string {
	if item.TotalAccounts == 0 {
		return "这个分组没有绑定账号"
	}
	if item.SchedulableAccounts > 0 {
		return ""
	}
	if item.RelayEligibleAccounts == 0 && item.MasterDirectAccounts > 0 {
		return "这个分组都是主站直连账号，不会分发到子站"
	}
	if item.RelayEligibleAccounts == 0 && item.LocalOnlyAccounts > 0 {
		return "这个分组都是仅主站本地账号，不会分发到子站"
	}
	if item.PendingAccounts > 0 {
		return "公有账号仍在待审核，或分润策略没有配置"
	}
	if item.SuspendedAccounts > 0 {
		return "公有账号已被暂停共享"
	}
	if item.UnknownLevelAccounts > 0 && item.RequiredLevel != "" {
		return "OpenAI 账号等级未知，不能进入有等级要求的分组"
	}
	if item.LevelMismatchAccounts > 0 {
		return "OpenAI 账号等级与分组要求不一致"
	}
	if item.RateLimitedAccounts > 0 {
		return "账号处于限流恢复期"
	}
	if item.TempBlockedAccounts > 0 {
		return "账号临时不可调度或过载冷却中"
	}
	if item.ExpiredAccounts > 0 {
		return "账号已过期"
	}
	if item.UnschedulableAccounts > 0 {
		return "账号未启用或被手动设为不可调度"
	}
	return "没有账号符合子站转发规则"
}

func relayConfigCheckOKMessage(code string) string {
	switch code {
	case "GLOBAL_SHARE_POLICY":
		return "全局账号分润策略已配置。"
	case "ONLINE_SUBSITE":
		return "至少有一个启用子站在线。"
	case "PUBLIC_OPENAI_FREE_GROUP":
		return "OpenAI Free 公共分组已配置。"
	case "PUBLIC_OPENAI_PLUS_GROUP":
		return "OpenAI Plus 公共分组已配置。"
	case "PUBLIC_OPENAI_PRO_GROUP":
		return "OpenAI Pro 公共分组已配置。"
	case "DUPLICATE_EFFECTIVE_LEASE":
		return "每个账号最多只有一个有效租约。"
	case "PENDING_WITHOUT_REASON":
		return "待审核公有账号已有诊断原因。"
	case "LEVEL_MISMATCH_LEASE":
		return "有效 OpenAI 租约与分组等级匹配。"
	case "SHARE_MODE_MISMATCH_LEASE":
		return "有效租约符合公有/私有共享规则。"
	default:
		return "检查通过。"
	}
}

func relayAccountDistributionReason(code string) string {
	switch code {
	case "DISTRIBUTED":
		return "已分发到子站。"
	case "MASTER_DIRECT":
		return "主站直连账号，不进入子站池。"
	case "LOCAL_ONLY":
		return "仅主站本地账号，不进入子站池。"
	case "READY_TO_DISTRIBUTE":
		return "符合分发条件，点击“立即自动分发”后会进入在线子站。"
	case "GROUP_NOT_BOUND":
		return "账号还没有绑定到可转发分组。"
	case "GROUP_INACTIVE":
		return "分组未启用。"
	case "ACCOUNT_INACTIVE":
		return "账号不是启用状态。"
	case "ACCOUNT_UNSCHEDULABLE":
		return "账号被设置为不可调度。"
	case "ACCOUNT_EXPIRED":
		return "账号已过期。"
	case "ACCOUNT_OVERLOADED":
		return "账号处于过载冷却中。"
	case "ACCOUNT_RATE_LIMITED":
		return "账号处于限流恢复期。"
	case "ACCOUNT_TEMP_BLOCKED":
		return "账号临时不可调度。"
	case "PLATFORM_MISMATCH":
		return "账号平台和分组平台不一致。"
	case "PRIVATE_OWNER_MISMATCH":
		return "私有账号所有者和私有分组不一致。"
	case "PRIVATE_SHARE_MODE_MISMATCH":
		return "私有分组只能分发私有模式账号。"
	case "PUBLIC_SHARE_NOT_APPROVED":
		return "公有账号没有审核通过，或共享状态不是已通过。"
	case "ACCOUNT_LEVEL_UNKNOWN":
		return "账号等级未知，不能自动进入有等级要求的分组。"
	case "ACCOUNT_LEVEL_MISMATCH":
		return "账号等级不符合当前分组要求。"
	default:
		return "未命中分发规则。"
	}
}

func (r *subsiteRepository) ListActiveCircuitBreakers(ctx context.Context) ([]service.SubsiteCircuitBreaker, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, scope, target_id, COALESCE(subsite_id, ''), COALESCE(account_id, 0),
			COALESCE(lease_id, ''), reason, failures, cooldown_until, last_error, created_at, updated_at
		FROM subsite_circuit_breakers
		WHERE deleted_at IS NULL
		  AND cooldown_until > NOW()
		ORDER BY cooldown_until DESC, updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list active subsite circuit breakers: %w", err)
	}
	defer rows.Close()
	items := make([]service.SubsiteCircuitBreaker, 0)
	for rows.Next() {
		var item service.SubsiteCircuitBreaker
		if err := rows.Scan(
			&item.ID,
			&item.Scope,
			&item.TargetID,
			&item.SubsiteID,
			&item.AccountID,
			&item.LeaseID,
			&item.Reason,
			&item.Failures,
			&item.CooldownUntil,
			&item.LastError,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *subsiteRepository) upsertCircuitBreaker(ctx context.Context, event *service.SubsiteForwardEvent) error {
	if event == nil {
		return nil
	}
	scope := "subsite"
	targetID := strings.TrimSpace(event.AttemptedSubsiteID)
	if targetID == "" {
		targetID = strings.TrimSpace(event.SubsiteID)
	}
	accountID := int64(0)
	leaseID := ""
	if event.AccountID > 0 {
		scope = "account"
		targetID = fmt.Sprint(event.AccountID)
		accountID = event.AccountID
	}
	if strings.TrimSpace(event.LeaseID) != "" {
		scope = "lease"
		targetID = strings.TrimSpace(event.LeaseID)
		leaseID = targetID
		accountID = event.AccountID
	}
	if targetID == "" {
		return nil
	}
	reason := strings.TrimSpace(event.Reason)
	if reason == "" {
		reason = event.Outcome
	}
	cooldown := time.Now().Add(2 * time.Minute)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO subsite_circuit_breakers (
			scope, target_id, subsite_id, account_id, lease_id, reason,
			failures, cooldown_until, last_error
		)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, 0), NULLIF($5, ''), $6, 1, $7, $8)
		ON CONFLICT (scope, target_id) DO UPDATE
		SET subsite_id = COALESCE(EXCLUDED.subsite_id, subsite_circuit_breakers.subsite_id),
			account_id = COALESCE(EXCLUDED.account_id, subsite_circuit_breakers.account_id),
			lease_id = COALESCE(EXCLUDED.lease_id, subsite_circuit_breakers.lease_id),
			reason = EXCLUDED.reason,
			failures = subsite_circuit_breakers.failures + 1,
			cooldown_until = GREATEST(subsite_circuit_breakers.cooldown_until, EXCLUDED.cooldown_until),
			last_error = EXCLUDED.last_error,
			updated_at = NOW(),
			deleted_at = NULL
	`, scope, targetID, event.SubsiteID, accountID, leaseID, reason, cooldown, event.Error)
	if err != nil {
		return fmt.Errorf("upsert subsite circuit breaker: %w", err)
	}
	return nil
}

func (r *subsiteRepository) clearCircuitBreaker(ctx context.Context, scope, targetID string) error {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE subsite_circuit_breakers
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE scope = $1 AND target_id = $2 AND deleted_at IS NULL
	`, scope, targetID)
	if err != nil {
		return fmt.Errorf("clear subsite circuit breaker: %w", err)
	}
	return nil
}

func (r *subsiteRepository) CleanupForwardState(ctx context.Context, now time.Time) (*service.SubsiteForwardCleanupResult, error) {
	if now.IsZero() {
		now = time.Now()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin relay cleanup tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result := &service.SubsiteForwardCleanupResult{}
	expiredAffinities, err := tx.ExecContext(ctx, `
		UPDATE subsite_forward_affinities
		SET deleted_at = $1, updated_at = $1
		WHERE deleted_at IS NULL
		  AND locked = FALSE
		  AND expires_at <= $1
	`, now)
	if err != nil {
		return nil, fmt.Errorf("cleanup expired relay affinities: %w", err)
	}
	result.ExpiredAffinities, _ = expiredAffinities.RowsAffected()

	expiredBreakers, err := tx.ExecContext(ctx, `
		UPDATE subsite_circuit_breakers
		SET deleted_at = $1, updated_at = $1
		WHERE deleted_at IS NULL
		  AND cooldown_until <= $1
	`, now)
	if err != nil {
		return nil, fmt.Errorf("cleanup expired relay circuit breakers: %w", err)
	}
	result.ExpiredBreakers, _ = expiredBreakers.RowsAffected()

	deletedEvents, err := tx.ExecContext(ctx, `
		DELETE FROM subsite_forward_events
		WHERE created_at < $1::timestamptz - INTERVAL '14 days'
	`, now)
	if err != nil {
		return nil, fmt.Errorf("cleanup relay forward events: %w", err)
	}
	result.DeletedEvents, _ = deletedEvents.RowsAffected()

	deletedSamples, err := tx.ExecContext(ctx, `
		DELETE FROM subsite_health_samples
		WHERE created_at < $1::timestamptz - INTERVAL '7 days'
	`, now)
	if err != nil {
		return nil, fmt.Errorf("cleanup relay health samples: %w", err)
	}
	result.DeletedSamples, _ = deletedSamples.RowsAffected()

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit relay cleanup tx: %w", err)
	}
	return result, nil
}

func forwardAffinitySelectSQL() string {
	return `
		SELECT id, affinity_key, affinity_type, subsite_id, COALESCE(lease_id, ''),
			COALESCE(account_id, 0), COALESCE(api_key_id, 0), COALESCE(user_id, 0),
			COALESCE(group_id, 0), model, session_id, source, locked, hits,
			last_reason, last_error, expires_at, last_used_at, created_at, updated_at, deleted_at
		FROM subsite_forward_affinities
	`
}

func scanForwardAffinity(row subsiteRowScanner) (*service.SubsiteForwardAffinity, error) {
	item := &service.SubsiteForwardAffinity{}
	err := row.Scan(
		&item.ID, &item.Key, &item.Type, &item.SubsiteID, &item.LeaseID,
		&item.AccountID, &item.APIKeyID, &item.UserID, &item.GroupID,
		&item.Model, &item.SessionID, &item.Source, &item.Locked, &item.Hits,
		&item.LastReason, &item.LastError, &item.ExpiresAt, &item.LastUsedAt,
		&item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func forwardAffinityWhere(filter service.ListSubsiteForwardAffinitiesFilter) (string, []any) {
	where := []string{"deleted_at IS NULL"}
	args := []any{}
	if strings.TrimSpace(filter.SubsiteID) != "" {
		args = append(args, strings.TrimSpace(filter.SubsiteID))
		where = append(where, fmt.Sprintf("subsite_id = $%d", len(args)))
	}
	if filter.APIKeyID > 0 {
		args = append(args, filter.APIKeyID)
		where = append(where, fmt.Sprintf("api_key_id = $%d", len(args)))
	}
	if filter.AccountID > 0 {
		args = append(args, filter.AccountID)
		where = append(where, fmt.Sprintf("account_id = $%d", len(args)))
	}
	if filter.Locked != nil {
		args = append(args, *filter.Locked)
		where = append(where, fmt.Sprintf("locked = $%d", len(args)))
	}
	if strings.TrimSpace(filter.Search) != "" {
		args = append(args, "%"+escapeLike(strings.TrimSpace(filter.Search))+"%")
		where = append(where, fmt.Sprintf("(affinity_key ILIKE $%d OR model ILIKE $%d OR session_id ILIKE $%d)", len(args), len(args), len(args)))
	}
	return strings.Join(where, " AND "), args
}

func forwardEventSelectSQL() string {
	return `
		SELECT id, request_id, affinity_key, COALESCE(subsite_id, ''), attempted_subsite_id,
			fallback_from, COALESCE(lease_id, ''), COALESCE(account_id, 0),
			COALESCE(api_key_id, 0), COALESCE(user_id, 0), COALESCE(group_id, 0),
			model, session_id, method, path, status_code, latency_ms, request_bytes,
			response_bytes, reason, outcome, error, metadata, created_at
		FROM subsite_forward_events
	`
}

func scanForwardEvent(row subsiteRowScanner) (*service.SubsiteForwardEvent, error) {
	item := &service.SubsiteForwardEvent{}
	var metadataRaw []byte
	err := row.Scan(
		&item.ID, &item.RequestID, &item.AffinityKey, &item.SubsiteID, &item.AttemptedSubsiteID,
		&item.FallbackFrom, &item.LeaseID, &item.AccountID, &item.APIKeyID,
		&item.UserID, &item.GroupID, &item.Model, &item.SessionID, &item.Method,
		&item.Path, &item.StatusCode, &item.LatencyMS, &item.RequestBytes,
		&item.ResponseBytes, &item.Reason, &item.Outcome, &item.Error,
		&metadataRaw, &item.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	item.Metadata = decodeJSONMap(metadataRaw)
	return item, nil
}

func forwardEventWhere(filter service.ListSubsiteForwardEventsFilter) (string, []any) {
	where := []string{"TRUE"}
	args := []any{}
	if strings.TrimSpace(filter.SubsiteID) != "" {
		args = append(args, strings.TrimSpace(filter.SubsiteID))
		where = append(where, fmt.Sprintf("subsite_id = $%d", len(args)))
	}
	if strings.TrimSpace(filter.Outcome) != "" {
		args = append(args, strings.TrimSpace(filter.Outcome))
		where = append(where, fmt.Sprintf("outcome = $%d", len(args)))
	}
	if strings.TrimSpace(filter.Search) != "" {
		args = append(args, "%"+escapeLike(strings.TrimSpace(filter.Search))+"%")
		where = append(where, fmt.Sprintf("(request_id ILIKE $%d OR affinity_key ILIKE $%d OR model ILIKE $%d OR error ILIKE $%d)", len(args), len(args), len(args), len(args)))
	}
	return strings.Join(where, " AND "), args
}
