-- Backfill persisted Subsite Relay route policy decisions for existing accounts.
UPDATE accounts
SET extra = COALESCE(extra, '{}'::jsonb)
	|| jsonb_build_object(
		'subsite_route_policy', COALESCE(NULLIF(extra->>'subsite_route_policy', ''), 'auto'),
		'subsite_route_policy_resolved',
			CASE
				WHEN COALESCE(NULLIF(extra->>'subsite_route_policy', ''), 'auto') IN ('master_direct', 'subsite_relay', 'local_only')
					AND COALESCE(NULLIF(extra->>'subsite_route_policy', ''), 'auto') <> 'auto'
					THEN extra->>'subsite_route_policy'
				WHEN type IN ('apikey', 'upstream', 'bedrock', 'service_account')
					THEN 'master_direct'
				WHEN COALESCE(credentials->>'base_url', credentials->>'api_base_url', extra->>'custom_base_url', extra->>'upstream_url', extra->>'upstream_base_url', extra->>'upstream_endpoint', '') <> ''
					THEN 'master_direct'
				WHEN type IN ('oauth', 'setup-token')
					THEN 'subsite_relay'
				ELSE 'local_only'
			END,
		'subsite_route_policy_reason',
			CASE
				WHEN type IN ('apikey', 'upstream', 'bedrock', 'service_account')
					THEN '外部 API Key / 上游透传账号由主站直连，不进入子站池'
				WHEN COALESCE(credentials->>'base_url', credentials->>'api_base_url', extra->>'custom_base_url', extra->>'upstream_url', extra->>'upstream_base_url', extra->>'upstream_endpoint', '') <> ''
					THEN '账号配置了自定义 Base URL / 上游地址，由主站直连'
				WHEN type IN ('oauth', 'setup-token')
					THEN 'OAuth / Setup Token 账号允许进入子站池'
				ELSE '账号类型暂不支持子站转发'
			END,
		'subsite_route_policy_updated_at', to_char(NOW() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	)
WHERE deleted_at IS NULL;
