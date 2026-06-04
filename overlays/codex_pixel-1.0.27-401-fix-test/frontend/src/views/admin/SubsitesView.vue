<template>
  <AppLayout>
    <section class="mb-6 space-y-4">
      <div class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-600 dark:bg-dark-800">
        <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
          <div class="max-w-3xl">
            <div class="flex flex-wrap items-center gap-2">
              <h1 class="text-xl font-semibold text-gray-950 dark:text-white">子站中转总控台</h1>
              <span :class="['badge', relayHealthState.className]">{{ relayHealthState.label }}</span>
            </div>
            <p class="mt-2 text-sm text-gray-600 dark:text-gray-300">{{ relayHealthState.description }}</p>
            <p class="mt-2 text-xs text-gray-500">规则：账号必须审核通过、等级匹配价格分组、公有/私有模式匹配，并且一个账号同一时间只能在一个子站池里。</p>
            <p class="mt-1 text-xs text-gray-500">自动分发：账号审核通过、等级变化、公私切换、分组变化后约 10 秒内重算；系统每 60 秒兜底扫描一次，自动释放不合规旧租约并补充分发。</p>
            <p v-if="distributedAccounts.length > 0" class="mt-2 text-xs font-medium text-emerald-700 dark:text-emerald-300">
              当前已分发：{{ distributedAccountsSummary }}
            </p>
          </div>
          <div class="min-w-full rounded-2xl border border-gray-100 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-700 xl:min-w-[25rem]">
            <p class="text-xs text-gray-500">当前转发模式</p>
            <div class="mt-2 flex gap-2">
              <select v-model="forwardModeForm" class="input min-h-[40px] flex-1 text-sm">
                <option value="local">只用主站账号</option>
                <option value="forward">主站转发到子站</option>
                <option value="direct">只允许子站直连</option>
              </select>
              <button class="btn btn-sm btn-secondary" :disabled="forwardModeSaving || forwardModeForm === (forwardStats?.mode || 'forward')" @click="saveForwardMode">
                保存
              </button>
            </div>
            <div class="mt-3 flex flex-wrap gap-2">
              <button class="btn btn-primary" :disabled="autoDistributeSaving || forwardLoading" @click="runAutoDistribute">
                {{ autoDistributeSaving ? '正在分发...' : '立即自动分发' }}
              </button>
              <button class="btn btn-secondary" :disabled="forwardLoading || autoDistributeSaving" @click="loadForwardConsole">
                刷新诊断
              </button>
            </div>
          </div>
        </div>

        <div class="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-6">
          <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700">
            <p class="text-xs text-gray-500">在线子站</p>
            <p class="mt-2 text-2xl font-semibold text-gray-950 dark:text-white">{{ forwardStats?.online_subsites || 0 }}/{{ forwardStats?.total_subsites || 0 }}</p>
          </div>
          <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700">
            <p class="text-xs text-gray-500">账号总数</p>
            <p class="mt-2 text-2xl font-semibold text-gray-950 dark:text-white">{{ relaySummary.totalAccounts }}</p>
          </div>
          <div class="rounded-2xl bg-emerald-50 p-4 dark:bg-emerald-950/20">
            <p class="text-xs text-emerald-700 dark:text-emerald-300">已进子站</p>
            <p class="mt-2 text-2xl font-semibold text-emerald-700 dark:text-emerald-200">{{ distributedAccountCount }}</p>
          </div>
          <div class="rounded-2xl bg-amber-50 p-4 dark:bg-amber-950/20">
            <p class="text-xs text-amber-700 dark:text-amber-300">可分发未分发</p>
            <p class="mt-2 text-2xl font-semibold text-amber-700 dark:text-amber-200">{{ readyAccountCount }}</p>
          </div>
          <div class="rounded-2xl bg-red-50 p-4 dark:bg-red-950/20">
            <p class="text-xs text-red-700 dark:text-red-300">被规则阻断</p>
            <p class="mt-2 text-2xl font-semibold text-red-700 dark:text-red-200">{{ blockedAccountCount }}</p>
          </div>
          <div class="rounded-2xl bg-sky-50 p-4 dark:bg-sky-950/20">
            <p class="text-xs text-sky-700 dark:text-sky-300">24h 请求 / 缓存</p>
            <p class="mt-2 text-sm font-semibold text-sky-800 dark:text-sky-100">{{ formatCompact(forwardStats?.events_24h || 0) }} / {{ formatPercent(forwardStats?.cache_hit_ratio_24h || 0) }}</p>
            <p class="mt-1 text-xs text-sky-700 dark:text-sky-300">95% 请求延迟 {{ Math.round(forwardStats?.p95_latency_ms_24h || 0) }} 毫秒</p>
          </div>
        </div>

        <div class="mt-4 grid gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <div class="rounded-2xl border border-emerald-100 bg-emerald-50/70 p-4 dark:border-emerald-900/40 dark:bg-emerald-950/20">
            <div class="flex items-center justify-between gap-3">
              <div>
                <h2 class="text-sm font-semibold text-emerald-900 dark:text-emerald-100">已经进入子站的账号</h2>
                <p class="text-xs text-emerald-700 dark:text-emerald-300">这里显示账号现在在哪个子站池里。</p>
              </div>
              <span class="badge badge-success">{{ distributedAccounts.length }}</span>
            </div>
            <div class="mt-3 space-y-2">
              <div v-for="item in distributedAccounts.slice(0, 5)" :key="`distributed-${item.account_id}-${item.lease_id}`" class="rounded-xl bg-white/80 p-3 text-sm text-gray-700 shadow-sm dark:bg-dark-800 dark:text-gray-200">
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <span class="font-medium text-gray-950 dark:text-white">#{{ item.account_id }} {{ item.account_name || '未命名账号' }}</span>
                  <span class="badge badge-success">已进子站</span>
                </div>
                <p class="mt-1 text-xs text-gray-500">
                  子站：{{ item.subsite_name || item.subsite_id }} · 价格池：{{ item.group_name || '-' }} · 到期：{{ formatDate(item.lease_expires_at) }}
                </p>
              </div>
              <p v-if="!distributedAccounts.length" class="text-sm text-emerald-700 dark:text-emerald-300">还没有账号进入子站。请先看右侧阻断原因，或点击“立即自动分发”。</p>
            </div>
          </div>

          <div class="rounded-2xl border border-amber-100 bg-amber-50/70 p-4 dark:border-amber-900/40 dark:bg-amber-950/20">
            <div class="flex items-center justify-between gap-3">
              <div>
                <h2 class="text-sm font-semibold text-amber-900 dark:text-amber-100">还没进入子站的原因</h2>
                <p class="text-xs text-amber-700 dark:text-amber-300">优先显示最影响分发的几个原因。</p>
              </div>
              <span class="badge badge-warning">{{ blockedAccountCount + readyAccountCount }}</span>
            </div>
            <div class="mt-3 space-y-2">
              <div v-for="reason in topAccountBlockReasons" :key="reason.label" class="rounded-xl bg-white/80 p-3 text-sm text-gray-700 shadow-sm dark:bg-dark-800 dark:text-gray-200">
                <div class="flex items-center justify-between gap-3">
                  <span class="font-medium text-gray-950 dark:text-white">{{ reason.label }}</span>
                  <span>{{ reason.value }} 个</span>
                </div>
                <p class="mt-1 text-xs text-gray-500">{{ reason.hint }}</p>
              </div>
              <p v-if="!topAccountBlockReasons.length" class="text-sm text-amber-700 dark:text-amber-300">暂无明显阻断项。若账号还没分发，请点击“立即自动分发”后查看结果。</p>
            </div>
          </div>
        </div>

        <div class="mt-4 grid gap-3 lg:grid-cols-3">
          <div v-for="rule in relayAutomationRules" :key="rule.title" class="rounded-2xl border border-gray-100 bg-gray-50 p-4 text-sm dark:border-dark-600 dark:bg-dark-700">
            <div class="font-semibold text-gray-900 dark:text-white">{{ rule.title }}</div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-300">{{ rule.description }}</p>
          </div>
        </div>

        <div v-if="autoDistributeMessage" class="mt-4 rounded-2xl border border-sky-100 bg-sky-50/70 p-4 text-sm text-sky-900 dark:border-sky-900/40 dark:bg-sky-950/20 dark:text-sky-100">
          <p class="font-semibold">{{ autoDistributeMessage }}</p>
          <div v-if="autoDistributeDetails" class="mt-3 grid gap-3 lg:grid-cols-2">
            <div>
              <p class="text-xs font-semibold">新增租约</p>
              <div class="mt-2 space-y-1 text-xs">
                <p v-for="lease in autoDistributeDetails.created" :key="lease.lease_id">
                  #{{ lease.account_id }} {{ lease.account_name || '' }} → {{ lease.subsite_id }} / {{ lease.group_name || '未命名分组' }}
                </p>
                <p v-if="!autoDistributeDetails.created.length" class="text-sky-700 dark:text-sky-300">没有新增租约。</p>
              </div>
            </div>
            <div>
              <p class="text-xs font-semibold">跳过账号</p>
              <div class="mt-2 space-y-1 text-xs">
                <p v-for="item in autoDistributeDetails.skipped" :key="`${item.account_id}-${item.group_name}-${item.reason}`">
                  #{{ item.account_id }} {{ item.account_name || '' }} / {{ item.group_name || '未命名分组' }}：{{ item.reason }}
                </p>
                <p v-if="!autoDistributeDetails.skipped.length" class="text-sky-700 dark:text-sky-300">没有可展示的跳过项。若仍未分发，请看下方“账号去向明细”，那里会显示待审核、等级未知、分组不匹配等完整原因。</p>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="rounded-2xl border border-indigo-100 bg-indigo-50/60 p-4 shadow-sm dark:border-indigo-900/40 dark:bg-indigo-950/20">
        <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
          <div class="max-w-3xl">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="text-base font-semibold text-indigo-950 dark:text-indigo-100">出口代理池 / 固定出口 IP</h2>
              <span :class="['badge', relayProxySummary.className]">{{ relayProxySummary.label }}</span>
            </div>
            <p class="mt-2 text-sm text-indigo-900/80 dark:text-indigo-100/80">{{ relayProxySummary.description }}</p>
            <p class="mt-2 text-xs text-indigo-800/80 dark:text-indigo-200/80">
              操作路径：先在 IP 管理添加代理并测试通过，然后在下方“账号去向明细”的“固定出口 IP”列给账号选择代理。一个账号建议固定一个代理/IP，避免同一账号访问 IP 频繁变化。
            </p>
            <p class="mt-1 text-xs text-indigo-800/80 dark:text-indigo-200/80">
              自动分发规则：已有绑定会被保留；未绑定账号会按启用代理的账号数量选择负载最低的代理；清除绑定后，该账号不再强制固定出口 IP。
            </p>
          </div>
          <div class="grid min-w-full gap-2 text-sm xl:min-w-[24rem]">
            <div class="grid grid-cols-3 gap-2">
              <div class="rounded-xl bg-white/80 p-3 dark:bg-dark-800">
                <p class="text-xs text-gray-500">启用代理</p>
                <p class="mt-1 text-xl font-semibold text-gray-950 dark:text-white">{{ activeRelayProxies.length }}</p>
              </div>
              <div class="rounded-xl bg-white/80 p-3 dark:bg-dark-800">
                <p class="text-xs text-gray-500">已固定账号</p>
                <p class="mt-1 text-xl font-semibold text-gray-950 dark:text-white">{{ proxyBoundAccountCount }}</p>
              </div>
              <div class="rounded-xl bg-white/80 p-3 dark:bg-dark-800">
                <p class="text-xs text-gray-500">未绑定</p>
                <p class="mt-1 text-xl font-semibold text-gray-950 dark:text-white">{{ proxyMissingAccountCount }}</p>
              </div>
            </div>
            <div class="flex flex-wrap gap-2">
              <RouterLink to="/admin/proxies" class="btn btn-primary btn-sm">打开 IP 管理 / 添加代理</RouterLink>
              <button type="button" class="btn btn-secondary btn-sm" :disabled="relayProxiesLoading" @click="loadRelayProxies">
                {{ relayProxiesLoading ? '刷新中...' : '刷新代理池' }}
              </button>
              <button type="button" class="btn btn-secondary btn-sm" :disabled="autoDistributeSaving || forwardLoading" @click="runAutoDistribute">
                按负载自动补齐绑定
              </button>
            </div>
          </div>
        </div>
        <div class="mt-4 grid gap-2 md:grid-cols-2 xl:grid-cols-4">
          <div v-for="proxy in activeRelayProxies.slice(0, 8)" :key="proxy.id" class="rounded-xl border border-indigo-100 bg-white/80 p-3 text-sm dark:border-indigo-900/40 dark:bg-dark-800">
            <div class="flex items-center justify-between gap-2">
              <span class="truncate font-medium text-gray-950 dark:text-white">{{ proxy.name }}</span>
              <span class="badge badge-gray">{{ proxy.account_count || 0 }} 个账号</span>
            </div>
            <p class="mt-1 truncate text-xs text-gray-500">{{ proxy.protocol }}://{{ proxy.host }}:{{ proxy.port }}</p>
            <p class="mt-1 text-xs text-gray-500">
              {{ proxy.country || proxy.country_code || '未知地区' }}
              <span v-if="proxy.ip_address"> · {{ proxy.ip_address }}</span>
              <span v-if="proxy.quality_grade"> · 质量 {{ proxy.quality_grade }}</span>
            </p>
          </div>
        </div>
        <p v-if="!activeRelayProxies.length" class="mt-3 rounded-xl border border-amber-100 bg-amber-50/70 p-3 text-sm text-amber-800 dark:border-amber-900/40 dark:bg-amber-950/20 dark:text-amber-100">
          当前没有启用代理。请点击“打开 IP 管理 / 添加代理”新增代理，测试通过后再回到这里绑定账号。
        </p>
        <p v-else class="mt-3 text-xs text-indigo-800/80 dark:text-indigo-200/80">{{ proxyBindingMessage }}</p>
      </div>

      <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,0.9fr)]">
        <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800">
          <div class="flex items-start justify-between gap-3">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">当前结论</h2>
              <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ relayHealthState.title }}</p>
            </div>
            <span :class="['badge', relayHealthState.className]">{{ relayHealthState.label }}</span>
          </div>
          <p class="mt-3 text-sm text-gray-600 dark:text-gray-300">{{ relayHealthState.description }}</p>
          <div class="mt-4 grid gap-2 text-xs text-gray-600 dark:text-gray-300 sm:grid-cols-2">
            <span>公有池账号：{{ forwardStats?.automation_summary?.public_pool_accounts || 0 }}</span>
            <span>私有池账号：{{ forwardStats?.automation_summary?.private_pool_accounts || 0 }}</span>
            <span>有效租约：{{ forwardStats?.active_leases || 0 }}</span>
            <span>熔断项：{{ forwardStats?.circuit_open || 0 }}</span>
            <span>24h 成功率：{{ formatPercent(forwardStats?.success_rate_24h || 0) }}</span>
            <span>主站计费成本：{{ formatCost(forwardStats?.forwarded_cost_24h || 0) }}</span>
          </div>
        </div>

        <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">分发流程</h2>
          <p class="text-xs text-gray-500">按真实账号数据展示，每一步卡住都会影响后面。</p>
          <div class="mt-4 space-y-3">
            <div v-for="step in relayFlowSteps" :key="step.label" class="rounded-xl border border-gray-100 p-3 dark:border-dark-700">
              <div class="flex items-center justify-between gap-3">
                <span class="text-sm font-medium text-gray-900 dark:text-white">{{ step.label }}</span>
                <span class="text-lg font-semibold text-gray-900 dark:text-white">{{ step.value }}</span>
              </div>
              <p class="mt-1 text-xs text-gray-500">{{ step.hint }}</p>
            </div>
          </div>
        </div>

        <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">主要阻断原因</h2>
          <p class="text-xs text-gray-500">这里优先解释“审核通过但子站没有”的原因。</p>
          <div class="mt-4 space-y-2">
            <div v-for="reason in relayBlockReasons" :key="reason.label" class="rounded-xl border border-amber-100 bg-amber-50/50 p-3 text-sm text-amber-800 dark:border-amber-900/40 dark:bg-amber-950/20 dark:text-amber-100">
              <div class="flex items-center justify-between gap-3">
                <span class="font-semibold">{{ reason.label }}</span>
                <span>{{ reason.value }}</span>
              </div>
              <p class="mt-1 text-xs">{{ reason.hint }}</p>
            </div>
            <p v-if="!relayBlockReasons.length" class="rounded-xl border border-emerald-100 bg-emerald-50/50 p-3 text-sm text-emerald-800 dark:border-emerald-900/40 dark:bg-emerald-950/20 dark:text-emerald-100">
              暂无明显阻断项。如果账号仍没进入子站，请点击“立即自动分发”并查看返回结果。
            </p>
          </div>
        </div>
      </div>

      <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800">
        <div class="mb-3 flex items-center justify-between gap-3">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">系统自检</h2>
            <p class="text-xs text-gray-500">严重项会阻止子站中转；警告项会影响某些账号池，但不一定影响全部分发。</p>
          </div>
          <div class="flex gap-2 text-xs">
            <span class="badge badge-danger">严重 {{ relaySummary.criticalChecks.length }}</span>
            <span class="badge badge-warning">警告 {{ relaySummary.warningChecks.length }}</span>
          </div>
        </div>
        <div class="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
          <div
            v-for="check in forwardStats?.configuration_checks || []"
            :key="check.code"
            :class="['rounded-xl border p-3 text-sm', check.status === 'ok' ? 'border-emerald-100 bg-emerald-50/50 text-emerald-800 dark:border-emerald-900/40 dark:bg-emerald-950/20 dark:text-emerald-100' : adviceClass(check.severity)]"
          >
            <div class="flex items-center justify-between gap-3">
              <span class="font-semibold">{{ configCheckLabel(check.code) }}</span>
              <span class="text-xs tracking-wide">{{ check.status === 'ok' ? '正常' : severityLabel(check.severity) }}</span>
            </div>
            <p class="mt-1 text-xs">{{ check.message }}</p>
          </div>
        </div>
        <p v-if="!(forwardStats?.configuration_checks || []).length" class="text-sm text-gray-500">暂无配置自检数据。</p>
      </div>

      <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800">
        <div class="mb-3 flex items-center justify-between gap-3">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">价格池和账号池</h2>
            <p class="text-xs text-gray-500">每个分组就是一个价格池。账号等级必须和价格池要求一致，才允许进入子站。</p>
          </div>
          <span class="badge badge-secondary">{{ (forwardStats?.pool_distribution || []).length }}</span>
        </div>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
            <thead class="text-left text-xs uppercase tracking-wide text-gray-500">
              <tr>
                <th class="py-2 pr-4">分组</th>
                <th class="py-2 pr-4">价格池规则</th>
                <th class="py-2 pr-4">账号数量</th>
                <th class="py-2 pr-4">子站分发</th>
                <th class="py-2 pr-4">阻断明细</th>
                <th class="py-2">结论</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="pool in forwardStats?.pool_distribution || []" :key="`${pool.group_id}-${pool.platform}-${pool.scope}-${pool.required_level}`">
                <td class="py-3 pr-4">
                  <div class="font-medium text-gray-900 dark:text-white">{{ pool.group_name || `#${pool.group_id}` }}</div>
                  <div class="text-xs text-gray-500">#{{ pool.group_id }}</div>
                </td>
                <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">
                  平台：{{ platformLabel(pool.platform) }}<br />
                  模式：{{ scopeLabel(pool.scope) }}<br />
                  <span class="text-xs">等级要求：{{ levelLabel(pool.required_level) }}</span>
                </td>
                <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">
                  总数 {{ pool.total_accounts }}<br />
                  <span class="text-emerald-600">可进子站 {{ pool.schedulable_accounts }}</span><br />
                  <span class="text-xs text-sky-600 dark:text-sky-300">主站直连 {{ pool.master_direct_accounts || 0 }} / 仅本地 {{ pool.local_only_accounts || 0 }}</span><br />
                  <span class="text-xs text-indigo-600 dark:text-indigo-300">固定出口 {{ pool.proxy_bound_accounts || 0 }} / 未绑定代理 {{ pool.proxy_missing_accounts || 0 }}</span><br />
                  <span class="text-xs text-gray-500">未分发 {{ pool.unleased_accounts }}</span>
                </td>
                <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">
                  已进子站 {{ pool.active_leases }}<br />
                  <span class="text-xs">覆盖子站 {{ pool.assigned_subsites }}</span>
                </td>
                <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">
                  待审 {{ pool.pending_accounts }} / 暂停 {{ pool.suspended_accounts }}<br />
                  等级未知 {{ pool.unknown_level_accounts }} / 等级不符 {{ pool.level_mismatch_accounts }}<br />
                  <span class="text-xs">限流 {{ pool.rate_limited_accounts }} / 临时不可用 {{ pool.temp_blocked_accounts }} / 过期 {{ pool.expired_accounts }}</span>
                </td>
                <td class="py-3">
                  <span :class="['badge', pool.schedulable_accounts > 0 ? 'badge-success' : 'badge-warning']">
                    {{ pool.schedulable_accounts > 0 ? '可分发' : '不可分发' }}
                  </span>
                  <p v-if="pool.blocked_reason" class="mt-1 max-w-sm text-xs text-amber-600 dark:text-amber-300">{{ pool.blocked_reason }}</p>
                  <p v-else class="mt-1 text-xs text-gray-500">符合规则，可点击“立即自动分发”进入子站。</p>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="!(forwardStats?.pool_distribution || []).length" class="mt-3 text-sm text-gray-500">暂无账号池数据。请先把审核通过的账号绑定到对应分组。</p>
      </div>

      <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">账号去向明细</h2>
            <p class="text-xs text-gray-500">每个账号只展示一个当前结论：已进哪个子站，或者为什么还不能进子站。</p>
          </div>
          <div class="flex flex-wrap gap-2 text-xs">
            <span class="badge badge-success">已分发 {{ distributedAccountCount }}</span>
            <span class="badge badge-warning">待分发 {{ readyAccountCount }}</span>
            <span class="badge badge-primary">主站直连 {{ masterDirectAccountCount }}</span>
            <span class="badge badge-gray">不可分发 {{ blockedAccountCount }}</span>
          </div>
        </div>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
            <thead class="text-left text-xs uppercase tracking-wide text-gray-500">
              <tr>
                <th class="py-2 pr-4">账号</th>
                <th class="py-2 pr-4">账号当前状态</th>
                <th class="py-2 pr-4">目标价格池</th>
                <th class="py-2 pr-4">子站去向</th>
                <th class="py-2 pr-4">固定出口 IP</th>
                <th class="py-2">系统判断</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="item in visibleAccountDistribution" :key="`${item.account_id}-${item.group_id}-${item.lease_id || item.reason_code}`">
                <td class="py-3 pr-4">
                  <div class="font-medium text-gray-900 dark:text-white">{{ item.account_name || `#${item.account_id}` }}</div>
                  <div class="text-xs text-gray-500">#{{ item.account_id }}</div>
                </td>
                <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">
                  平台 {{ platformLabel(item.platform) }} / 等级 {{ levelLabel(item.account_level) }}<br />
                  <span class="text-xs">模式 {{ scopeLabel(item.share_mode) }} · 审核 {{ shareStatusLabel(item.share_status) }} · {{ item.schedulable ? '可调度' : '不可调度' }}</span>
                </td>
                <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">
                  {{ item.group_name || (item.group_id ? `#${item.group_id}` : '未绑定') }}<br />
                  <span class="text-xs">模式 {{ scopeLabel(item.group_scope) }} · 要求等级 {{ levelLabel(item.required_level) }}</span>
                </td>
                <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">
                  <template v-if="item.distributed">
                    {{ item.subsite_name || item.subsite_id }}<br />
                    <span class="text-xs">租约 {{ item.lease_id || '-' }}，到期 {{ formatDate(item.lease_expires_at) }}</span>
                  </template>
                  <template v-else-if="item.route_resolved === 'master_direct'">
                    <span class="font-medium text-sky-700 dark:text-sky-300">主站直连</span><br />
                    <span class="text-xs">外部 API Key / 自定义 Base URL 不进入子站池</span>
                  </template>
                  <template v-else-if="item.route_resolved === 'local_only'">
                    <span class="font-medium text-gray-500">仅主站本地</span><br />
                    <span class="text-xs">当前账号类型不参与子站转发</span>
                  </template>
                  <template v-else>
                    <span class="text-gray-400">未分发</span>
                  </template>
                </td>
                <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">
                  <select
                    :value="item.proxy_id || 0"
                    class="input min-w-[13rem] py-2 text-xs"
                    :disabled="proxyBindingSaving.has(item.account_id) || relayProxiesLoading"
                    @change="handleAccountProxyChange(item.account_id, $event)"
                  >
                    <option :value="0">不固定 / 自动补齐</option>
                    <option v-for="proxy in activeRelayProxies" :key="proxy.id" :value="proxy.id">
                      {{ proxySelectLabel(proxy) }}
                    </option>
                  </select>
                  <p class="mt-1 max-w-sm text-xs" :class="item.proxy_id ? 'text-indigo-600 dark:text-indigo-300' : 'text-amber-600 dark:text-amber-300'">
                    {{ proxyAffinityLabel(item) }}
                  </p>
                  <p v-if="proxyBindingSaving.has(item.account_id)" class="mt-1 text-xs text-gray-500">正在保存绑定...</p>
                  <p v-else-if="!activeRelayProxies.length" class="mt-1 text-xs text-amber-600 dark:text-amber-300">没有启用代理，请先到 IP 管理添加。</p>
                </td>
                <td class="py-3">
                  <span :class="['badge', accountDistributionClass(item)]">{{ accountDistributionLabel(item) }}</span>
                  <span v-if="item.route_resolved" class="ml-1 badge badge-secondary">{{ routePolicyLabel(item.route_resolved) }}</span>
                  <p class="mt-1 max-w-md text-xs text-gray-500 dark:text-gray-300">{{ item.reason }}</p>
                  <p v-if="item.route_reason && item.route_reason !== item.reason" class="mt-1 max-w-md text-xs text-gray-400 dark:text-gray-500">
                    路由规则：{{ item.route_reason }}
                  </p>
                  <p v-if="item.reason_code === 'ACCOUNT_LEVEL_UNKNOWN'" class="mt-1 max-w-md text-xs text-amber-600 dark:text-amber-300">
                    这个账号虽然审核通过，但系统没有识别出 Free/Plus/Pro。为了避免错价计费，暂时不会把它放进 Free 公共池。
                  </p>
                  <p v-if="item.distributed" class="mt-1 max-w-md text-xs text-emerald-600 dark:text-emerald-300">
                    这个账号已经在子站账号池中，不会再分发到第二个子站。
                  </p>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="!visibleAccountDistribution.length" class="mt-3 text-sm text-gray-500">暂无可展示账号。请确认账号已创建并绑定分组。</p>
        <p v-if="accountDistributionOverflow > 0" class="mt-3 text-xs text-gray-500">仅显示前 80 条，还有 {{ accountDistributionOverflow }} 条未显示。</p>
      </div>

      <div v-if="(forwardStats?.recommendations || []).length || (forwardStats?.lease_distribution || []).length" class="grid gap-4 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
        <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800">
          <div class="mb-3 flex items-center justify-between gap-3">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">系统建议</h2>
              <p class="text-xs text-gray-500">根据转发、租约、缓存命中和子站在线情况给出处理建议。</p>
            </div>
            <span class="badge badge-secondary">{{ (forwardStats?.recommendations || []).length }}</span>
          </div>
          <div class="space-y-2">
            <div
              v-for="advice in forwardStats?.recommendations || []"
              :key="advice.code"
              :class="['rounded-xl border p-3 text-sm', adviceClass(advice.severity)]"
            >
              <div class="flex items-center justify-between gap-3">
                <span class="font-semibold">{{ adviceLabel(advice.code) }}</span>
                <span class="text-xs tracking-wide">{{ severityLabel(advice.severity) }}</span>
              </div>
              <p class="mt-1 text-xs">{{ advice.message }}</p>
            </div>
            <p v-if="!(forwardStats?.recommendations || []).length" class="text-sm text-gray-500">当前没有需要处理的建议。</p>
          </div>
        </div>

        <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800">
          <div class="mb-3">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">租约分布</h2>
            <p class="text-xs text-gray-500">按平台、分组、等级统计已进入子站的账号租约。</p>
          </div>
          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
              <thead class="text-left text-xs uppercase tracking-wide text-gray-500">
                <tr>
                  <th class="py-2 pr-4">分组</th>
                  <th class="py-2 pr-4">等级</th>
                  <th class="py-2 pr-4">范围</th>
                  <th class="py-2 pr-4">租约数</th>
                  <th class="py-2">子站数</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="item in forwardStats?.lease_distribution || []" :key="`${item.group_id}-${item.platform}-${item.required_level}`">
                  <td class="py-3 pr-4">
                    <div class="font-medium text-gray-900 dark:text-white">{{ item.group_name || `#${item.group_id}` }}</div>
                    <div class="text-xs text-gray-500">{{ item.platform }}</div>
                  </td>
                  <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">{{ levelLabel(item.required_level) }}</td>
                  <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">{{ scopeLabel(item.scope) }}</td>
                  <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">
                    {{ item.active_leases }}
                    <span v-if="item.expiring_leases_1h > 0" class="ml-1 text-amber-600">/{{ item.expiring_leases_1h }} 即将过期</span>
                  </td>
                  <td class="py-3 text-gray-600 dark:text-gray-300">{{ item.assigned_subsites }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-if="!(forwardStats?.lease_distribution || []).length" class="mt-3 text-sm text-gray-500">当前没有活跃租约。点击“立即自动分发”或等待 API 请求触发分发。</p>
        </div>
      </div>

      <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_26rem]">
        <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800">
          <div class="mb-3 flex items-center justify-between gap-3">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">子站负载面板</h2>
              <p class="text-xs text-gray-500">根据心跳、租约、最近请求和熔断状态选择转发目标。</p>
            </div>
            <button class="btn btn-sm btn-secondary" :disabled="forwardLoading" @click="loadForwardConsole">
              <Icon name="refresh" size="sm" :class="forwardLoading ? 'animate-spin' : ''" />
            </button>
          </div>
          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
              <thead class="text-left text-xs uppercase tracking-wide text-gray-500">
                <tr>
                  <th class="py-2 pr-4">子站</th>
                  <th class="py-2 pr-4">负载</th>
                  <th class="py-2 pr-4">24h 质量</th>
                  <th class="py-2 pr-4">缓存</th>
                  <th class="py-2 pr-4">粘性路由</th>
                  <th class="py-2">健康</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="site in forwardStats?.by_subsite || []" :key="site.subsite_id">
                  <td class="py-3 pr-4">
                    <div class="font-medium text-gray-900 dark:text-white">{{ site.name || site.subsite_id }}</div>
                    <code class="text-xs text-gray-500">{{ site.subsite_id }}</code>
                  </td>
                  <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">
                    <span :class="['badge', loadLevelClass(site.load_level)]">{{ loadLevelLabel(site.load_level) }}</span><br />
                    活跃 {{ site.active_requests }} / 队列 {{ site.queued_usage }}<br />
                    每秒请求 {{ site.qps.toFixed(2) }} / 处理器 {{ site.cpu_percent.toFixed(1) }}%
                  </td>
                  <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">
                    请求 {{ formatCompact(site.events_24h) }}<br />
                    <span :class="site.failures_24h > 0 ? 'text-red-600' : 'text-emerald-600'">成功率 {{ formatPercent(site.success_rate_24h || 0) }}</span><br />
                    95% 请求延迟 {{ Math.round(site.p95_latency_ms_24h || 0) }} 毫秒
                  </td>
                  <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">
                    <span class="font-medium text-emerald-600">{{ formatPercent(site.cache_hit_ratio_24h || 0) }}</span><br />
                    命中缓存 {{ formatCompact(site.cache_read_tokens_24h || 0) }} Token
                  </td>
                  <td class="py-3 pr-4 text-gray-600 dark:text-gray-300">
                    {{ site.affinities }} 条<br />
                    锁定 {{ site.locked_affinities }} 条<br />
                    账号池 {{ site.active_leases || 0 }} 个
                  </td>
                  <td class="py-3">
                    <span :class="['badge', statusClass(site.effective_status || site.status)]">{{ site.health_score }}%</span>
                    <span v-if="site.circuit_open" class="ml-1 badge badge-danger">熔断</span>
                    <div class="mt-1 text-xs text-gray-500">{{ formatDate(site.last_heartbeat_at) }}</div>
                    <div v-if="site.circuit_open" class="mt-1 text-xs text-red-500">{{ site.circuit_reason || '-' }}</div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">手动粘性路由</h2>
          <p class="mt-1 text-xs text-gray-500">用于把某个密钥、账号或会话固定到指定子站。一般不用手动设置，除非要排查路由。</p>
          <div class="mt-4 space-y-3">
            <input v-model.trim="forwardLockForm.affinity_key" class="input" placeholder="路由标识，例如 api-key:123 或 session:abc" />
            <select v-model="forwardLockForm.subsite_id" class="input">
              <option value="">选择子站</option>
              <option v-for="site in subsites" :key="site.subsite_id" :value="site.subsite_id">{{ site.name }} / {{ site.subsite_id }}</option>
            </select>
            <div class="grid grid-cols-2 gap-3">
              <input v-model.trim="forwardLockForm.lease_id" class="input" placeholder="租约 ID，可选" />
              <input v-model.number="forwardLockForm.account_id" type="number" min="0" class="input" placeholder="账号 ID，可选" />
            </div>
            <div class="grid grid-cols-2 gap-3">
              <input v-model.number="forwardLockForm.api_key_id" type="number" min="0" class="input" placeholder="密钥 ID，可选" />
              <input v-model.number="forwardLockForm.ttl_hours" type="number" min="1" class="input" placeholder="有效期小时" />
            </div>
            <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
              <input v-model="forwardLockForm.locked" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" />
              强制锁定，不随负载自动迁移
            </label>
            <button class="btn btn-primary w-full" :disabled="forwardSaving" @click="saveForwardLock">保存粘性路由</button>
          </div>
        </div>
      </div>

      <div class="grid gap-4 xl:grid-cols-3">
        <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">粘性路由</h2>
            <button class="btn btn-sm btn-secondary" @click="loadForwardAffinities">刷新</button>
          </div>
          <div class="space-y-2">
            <div v-for="item in forwardAffinities" :key="item.id" class="rounded-xl border border-gray-100 p-3 text-sm dark:border-dark-700">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="truncate font-medium text-gray-900 dark:text-white">{{ item.model || '-' }} / {{ item.subsite_id }}</div>
                  <code class="block truncate text-xs text-gray-500">{{ item.affinity_key }}</code>
                </div>
                <button class="btn btn-sm btn-secondary" @click="deleteForwardAffinity(item.id)">删除</button>
              </div>
              <div class="mt-2 grid grid-cols-2 gap-2 text-xs text-gray-500">
                <span>命中 {{ item.hits }}</span>
                <span>{{ item.locked ? '锁定' : affinitySourceLabel(item.source) }}</span>
                <span>账号 {{ item.account_id || '-' }}</span>
                <span>租约 {{ item.lease_id || '-' }}</span>
              </div>
            </div>
            <p v-if="!forwardAffinities.length" class="text-sm text-gray-500">暂无粘性路由。</p>
          </div>
        </div>

        <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">熔断</h2>
            <button class="btn btn-sm btn-secondary" @click="loadForwardStats">刷新</button>
          </div>
          <div class="space-y-2">
            <div v-for="breaker in forwardStats?.circuit_breakers || []" :key="breaker.id" class="rounded-xl border border-red-100 bg-red-50/50 p-3 text-sm dark:border-red-900/40 dark:bg-red-950/20">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="truncate font-medium text-red-700 dark:text-red-200">{{ breakerScopeLabel(breaker.scope) }} / {{ breaker.target_id }}</div>
                  <div class="truncate text-xs text-red-500 dark:text-red-300">{{ breaker.subsite_id || '-' }}</div>
                </div>
                <span class="badge badge-danger">{{ breaker.failures }} 次</span>
              </div>
              <div class="mt-2 grid grid-cols-2 gap-2 text-xs text-gray-600 dark:text-gray-300">
                <span>账号 {{ breaker.account_id || '-' }}</span>
                <span>租约 {{ breaker.lease_id || '-' }}</span>
                <span>{{ breaker.reason || '-' }}</span>
                <span>{{ formatDate(breaker.cooldown_until) }}</span>
              </div>
              <p v-if="breaker.last_error" class="mt-2 break-all text-xs text-red-600 dark:text-red-300">{{ breaker.last_error }}</p>
            </div>
            <p v-if="!(forwardStats?.circuit_breakers || []).length" class="text-sm text-gray-500">暂无熔断。</p>
          </div>
        </div>

        <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-600 dark:bg-dark-800">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">最近转发事件</h2>
            <button class="btn btn-sm btn-secondary" @click="loadForwardEvents">刷新</button>
          </div>
          <div class="space-y-2">
            <div v-for="event in forwardEvents" :key="event.id" class="rounded-xl border border-gray-100 p-3 text-sm dark:border-dark-700">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="truncate font-medium text-gray-900 dark:text-white">{{ event.method }} {{ event.path }}</div>
                  <div class="text-xs text-gray-500">{{ event.subsite_id || event.attempted_subsite_id || '-' }} / {{ event.model || '-' }}</div>
                </div>
                <span :class="['badge', event.outcome === 'success' ? 'badge-success' : 'badge-danger']">{{ forwardOutcomeLabel(event.outcome) }}</span>
              </div>
              <div class="mt-2 grid grid-cols-3 gap-2 text-xs text-gray-500">
                <span>{{ event.status_code || '-' }}</span>
                <span>{{ event.latency_ms }} ms</span>
                <span>{{ formatDate(event.created_at) }}</span>
                <span>{{ forwardReasonLabel(event.reason) }}</span>
                <span>账号 {{ event.account_id || '-' }}</span>
                <span>租约 {{ event.lease_id || '-' }}</span>
              </div>
              <div v-if="event.fallback_from" class="mt-2 text-xs text-amber-600 dark:text-amber-300">从 {{ event.fallback_from }} 回退</div>
              <p v-if="event.error" class="mt-2 break-all text-xs text-red-600">{{ event.error }}</p>
              <div v-if="eventRouteSummary(event)" class="mt-2 rounded-lg bg-gray-50 p-2 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                {{ eventRouteSummary(event) }}
              </div>
              <div v-if="eventRouteDetails(event).length" class="mt-2 flex flex-wrap gap-1">
                <span v-for="detail in eventRouteDetails(event)" :key="detail" class="badge badge-secondary">{{ detail }}</span>
              </div>
            </div>
            <p v-if="!forwardEvents.length" class="text-sm text-gray-500">暂无事件。</p>
          </div>
        </div>
      </div>
    </section>

    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="relative w-full sm:w-72">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input v-model="search" type="text" class="input min-h-[44px] pl-10" placeholder="搜索名称、ID 或 URL" @input="handleSearch" />
          </div>
          <select v-model="status" class="input min-h-[44px] w-full sm:w-44" @change="reload">
            <option value="">全部状态</option>
            <option value="pending">待激活</option>
            <option value="active">运行中</option>
            <option value="maintenance">维护中</option>
            <option value="unhealthy">异常</option>
            <option value="disabled">已停用</option>
          </select>
          <div class="ml-auto flex flex-wrap gap-2">
            <button class="btn btn-secondary" :disabled="loading" @click="reload">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-primary" @click="openCreate">
              <Icon name="plus" size="md" class="mr-2" />
              新建子站
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="subsites" :loading="loading">
          <template #cell-name="{ row }">
            <div class="min-w-0">
              <div class="truncate font-medium text-gray-900 dark:text-white">{{ row.name }}</div>
              <code class="text-xs text-gray-500">{{ row.subsite_id }}</code>
            </div>
          </template>

          <template #cell-status="{ value }">
            <span :class="['badge', statusClass(value)]">{{ statusLabel(value) }}</span>
          </template>

          <template #cell-public_url="{ value }">
            <div class="flex min-w-0 items-center gap-2">
              <a v-if="value" :href="value" target="_blank" rel="noreferrer" class="truncate text-sm text-primary-600 hover:underline dark:text-primary-300">
                {{ value }}
              </a>
              <span v-else class="text-sm text-gray-400">-</span>
              <button v-if="value" class="rounded p-1 text-gray-400 hover:text-primary-600 dark:hover:text-primary-300" title="复制 URL" @click.stop="copy(value)">
                <Icon name="copy" size="sm" />
              </button>
            </div>
          </template>

          <template #cell-health_score="{ row }">
            <div class="flex flex-col gap-1">
              <span class="text-sm font-medium text-gray-900 dark:text-white">{{ row.health_score }}%</span>
              <span class="text-xs text-gray-500">{{ formatDate(row.last_heartbeat_at) }}</span>
            </div>
          </template>

          <template #cell-limits="{ row }">
            <div class="text-sm text-gray-700 dark:text-gray-200">
              每秒请求 {{ row.max_qps || '-' }} / 并发 {{ row.max_concurrency || '-' }}
            </div>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex flex-wrap justify-end gap-1.5">
              <button class="btn btn-sm btn-secondary" @click="openLeases(row)">租约</button>
              <button class="btn btn-sm btn-secondary" @click="copyClientConfig(row)">复制配置</button>
              <button v-if="row.status === 'pending' || row.status === 'unhealthy'" class="btn btn-sm btn-secondary" @click="openResetSecret(row)">修复</button>
              <button class="btn btn-sm btn-secondary" @click="openEdit(row)">编辑</button>
              <button v-if="row.status === 'pending' || row.status === 'unhealthy'" class="btn btn-sm btn-primary" @click="changeStatus(row, 'activate')">激活</button>
              <button v-if="row.status === 'active'" class="btn btn-sm btn-secondary" @click="changeStatus(row, 'pause')">暂停</button>
              <button v-if="row.status === 'maintenance'" class="btn btn-sm btn-primary" @click="changeStatus(row, 'resume')">恢复</button>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <BaseDialog :show="showForm" :title="editing ? '编辑子站' : '新建子站'" width="wide" @close="closeForm">
      <form class="space-y-4" @submit.prevent="saveSubsite">
        <div class="grid gap-4 md:grid-cols-2">
          <label class="form-field">
            <span class="form-label">名称</span>
            <input v-model.trim="form.name" class="input" required />
          </label>
          <label class="form-field">
            <span class="form-label">子站 ID</span>
            <input v-model.trim="form.subsite_id" class="input" :disabled="!!editing" placeholder="留空自动生成" />
          </label>
          <label class="form-field md:col-span-2">
            <span class="form-label">公网访问地址</span>
            <input v-model.trim="form.public_url" class="input" placeholder="https://edge.example.com" />
          </label>
          <label class="form-field">
            <span class="form-label">区域</span>
            <input v-model.trim="form.region" class="input" placeholder="us-east-1" />
          </label>
          <label class="form-field">
            <span class="form-label">版本</span>
            <input v-model.trim="form.version" class="input" placeholder="subsite-agent" />
          </label>
          <label class="form-field">
            <span class="form-label">最大 QPS</span>
            <input v-model.number="form.max_qps" type="number" min="0" class="input" />
          </label>
          <label class="form-field">
            <span class="form-label">最大并发</span>
            <input v-model.number="form.max_concurrency" type="number" min="0" class="input" />
          </label>
        </div>
        <label class="form-field">
          <span class="form-label">能力标签</span>
          <input v-model.trim="capabilitiesText" class="input" placeholder="openai,claude,gemini,images,websocket" />
        </label>
        <div class="flex justify-end gap-2 pt-2">
          <button type="button" class="btn btn-secondary" @click="closeForm">取消</button>
          <button type="submit" class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : '保存' }}</button>
        </div>
      </form>
    </BaseDialog>

    <BaseDialog :show="!!createdSecret" title="子站密钥" width="normal" @close="createdSecret = ''">
      <div class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-300">密钥只显示一次，请写入子站镜像服务的环境变量。</p>
        <div class="flex items-center gap-2 rounded border border-gray-200 bg-gray-50 p-3 dark:border-dark-500 dark:bg-dark-700">
          <code class="min-w-0 flex-1 break-all text-sm">{{ createdSecret }}</code>
          <button class="btn btn-secondary" @click="copy(createdSecret)">复制</button>
        </div>
      </div>
    </BaseDialog>

    <BaseDialog :show="!!resetTarget" title="修复子站连通" width="normal" @close="closeResetSecret">
      <div class="space-y-4">
        <div class="rounded border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-600/40 dark:bg-amber-900/20 dark:text-amber-100">
          这会重置 {{ resetTarget?.name }} 的主站认证密钥。旧子站密钥会立即失效，必须把新密钥写入子站环境变量并重启子站后，心跳才会恢复。
        </div>
        <div v-if="resetSecretResult" class="space-y-3">
          <div class="flex items-center gap-2 rounded border border-gray-200 bg-gray-50 p-3 dark:border-dark-500 dark:bg-dark-700">
            <code class="min-w-0 flex-1 break-all text-sm">{{ resetSecretResult.secret }}</code>
            <button class="btn btn-secondary" @click="copy(resetSecretResult.secret)">复制</button>
          </div>
          <div class="rounded border border-gray-200 bg-gray-50 p-3 dark:border-dark-500 dark:bg-dark-700">
            <pre class="whitespace-pre-wrap break-all text-xs text-gray-700 dark:text-gray-200">{{ resetEnvText }}</pre>
          </div>
          <p class="text-xs text-gray-500 dark:text-gray-400">请确认 SUBSITE_MASTER_URL 是子站能够访问到的主站根地址，不要带 /api/v1。</p>
          <div class="flex justify-end gap-2">
            <button class="btn btn-secondary" @click="copy(resetEnvText)">复制环境变量</button>
            <button class="btn btn-primary" @click="closeResetSecret">完成</button>
          </div>
        </div>
        <div v-else class="flex justify-end gap-2">
          <button class="btn btn-secondary" :disabled="resettingSecret" @click="closeResetSecret">取消</button>
          <button class="btn btn-danger" :disabled="resettingSecret" @click="resetSubsiteSecret">
            {{ resettingSecret ? '修复中...' : '重置密钥' }}
          </button>
        </div>
      </div>
    </BaseDialog>

    <BaseDialog :show="!!leaseSubsite" :title="leaseSubsite ? `租约 - ${leaseSubsite.name}` : '租约'" width="full" @close="closeLeases">
      <div class="grid gap-6 xl:grid-cols-[minmax(0,1fr)_24rem]">
        <div class="min-w-0">
          <DataTable :columns="leaseColumns" :data="leases" :loading="leasesLoading">
            <template #header-select>
              <input
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600"
                :checked="allVisibleLeasesSelected"
                :disabled="leases.length === 0"
                @change="toggleAllVisibleLeases"
              />
            </template>
            <template #cell-select="{ row }">
              <input
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600"
                :checked="selectedLeaseIDs.has(row.lease_id)"
                @change="toggleLeaseSelection(row)"
              />
            </template>
            <template #cell-group="{ row }">
              <div class="min-w-0">
                <div class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ leaseGroupLabel(row) }}</div>
                <div class="text-xs text-gray-500">{{ leaseGroupMeta(row) }}</div>
              </div>
            </template>
            <template #cell-account="{ row }">
              <div class="min-w-0">
                <div class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ leaseAccountLabel(row) }}</div>
                <div class="text-xs text-gray-500">{{ row.platform || '-' }}</div>
              </div>
            </template>
            <template #cell-status="{ value }">
              <span :class="['badge', leaseStatusClass(value)]">{{ leaseStatusLabel(value) }}</span>
            </template>
            <template #cell-usage="{ row }">
              <div class="text-sm text-gray-700 dark:text-gray-200">
                请求 {{ row.used_requests }} / {{ row.max_requests || '-' }}
                <br />
                Token {{ row.used_tokens }} / {{ row.max_tokens || '-' }}
              </div>
            </template>
            <template #cell-expires_at="{ value }">
              <span class="text-sm text-gray-600 dark:text-gray-300">{{ formatDate(value) }}</span>
            </template>
            <template #cell-actions="{ row }">
              <div class="flex flex-wrap justify-end gap-1.5">
                <button class="btn btn-sm btn-secondary" @click="renewLease(row)">续租 24h</button>
                <button v-if="row.status === 'active' || row.status === 'renewing'" class="btn btn-sm btn-secondary" @click="drainLease(row)">排空</button>
                <button v-if="row.status !== 'released'" class="btn btn-sm btn-danger" @click="releaseLease(row)">释放</button>
                <button class="btn btn-sm btn-danger" @click="deleteLease(row)">删除</button>
              </div>
            </template>
          </DataTable>
          <Pagination
            v-if="leasePagination.total > 0"
            :page="leasePagination.page"
            :total="leasePagination.total"
            :page-size="leasePagination.page_size"
            :page-size-options="[10, 20]"
            class="mt-3"
            @update:page="handleLeasePageChange"
            @update:pageSize="handleLeasePageSizeChange"
          />
        </div>
        <div class="space-y-4">
        <form class="space-y-4 rounded-lg border border-gray-200 p-4 dark:border-dark-500" @submit.prevent="createLease">
          <div>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">新增租约</h3>
            <p v-if="leaseAccountsLoading" class="mt-1 text-xs text-gray-500 dark:text-gray-400">正在按分组加载主站账号...</p>
          </div>
          <label class="form-field">
            <span class="form-label">分组</span>
            <select v-model.number="leaseForm.group_id" class="input" required @change="handleLeaseGroupChange">
              <option :value="0" disabled>选择分组</option>
              <option v-for="group in leaseGroupOptions" :key="group.id" :value="group.id">{{ group.name }}</option>
            </select>
          </label>
          <label class="form-field">
            <span class="form-label">账号</span>
            <input
              v-model.trim="leaseAccountSearch"
              class="input"
              :disabled="leaseForm.group_id <= 0"
              placeholder="输入账号 ID、名称或平台搜索"
            />
            <select v-if="filteredAccountOptions.length > 0" v-model.number="leaseForm.account_id" class="input" required :disabled="leaseForm.group_id <= 0">
              <option :value="0" disabled>选择账号</option>
              <option v-for="account in filteredAccountOptions" :key="account.id" :value="account.id">{{ account.label }}</option>
            </select>
            <div v-else class="rounded border border-dashed border-gray-300 px-3 py-2 text-sm text-gray-500 dark:border-dark-500 dark:text-gray-400">
              {{ leaseAccountEmptyText }}
            </div>
            <p v-if="leaseBulkSummary" class="text-xs text-gray-500 dark:text-gray-400">{{ leaseBulkSummary }}</p>
          </label>
          <label class="form-field">
            <span class="form-label">有效期小时</span>
            <input v-model.number="leaseForm.ttl_hours" type="number" min="1" class="input" required />
          </label>
          <label class="form-field">
            <span class="form-label">最大请求数</span>
            <input v-model.number="leaseForm.max_requests" type="number" min="0" class="input" />
          </label>
          <label class="form-field">
            <span class="form-label">最大 Token</span>
            <input v-model.number="leaseForm.max_tokens" type="number" min="0" class="input" />
          </label>
          <label class="form-field">
            <span class="form-label">随机数量</span>
            <input
              v-model.number="leaseRandomCount"
              type="number"
              min="1"
              :max="bulkLeaseAccounts.length || 1"
              class="input"
              :disabled="leaseForm.group_id <= 0"
            />
          </label>
          <div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-1 2xl:grid-cols-2">
            <button type="submit" class="btn btn-primary w-full" :disabled="leaseSaving || leaseBulkSaving || leaseForm.account_id <= 0">
              {{ leaseSaving ? '创建中...' : '创建租约' }}
            </button>
            <button
              type="button"
              class="btn btn-secondary w-full"
              :disabled="leaseSaving || leaseBulkSaving || leaseAccountsLoading || bulkLeaseAccounts.length === 0"
              @click="createAllVisibleLeases"
            >
              {{ leaseBulkSaving ? `添加中 ${leaseBulkProgress.done}/${leaseBulkProgress.total}` : `添加全部 ${bulkLeaseAccounts.length} 个账号` }}
            </button>
            <button
              type="button"
              class="btn btn-secondary w-full"
              :disabled="leaseSaving || leaseBulkSaving || leaseAccountsLoading || bulkLeaseAccounts.length === 0"
              @click="createRandomLeases(10)"
            >
              随机 {{ randomTenCount }} 个
            </button>
            <button
              type="button"
              class="btn btn-secondary w-full"
              :disabled="leaseSaving || leaseBulkSaving || leaseAccountsLoading || bulkLeaseAccounts.length === 0"
              @click="createRandomLeases(20)"
            >
              随机 {{ randomTwentyCount }} 个
            </button>
            <button
              type="button"
              class="btn btn-secondary w-full sm:col-span-2 xl:col-span-1 2xl:col-span-2"
              :disabled="leaseSaving || leaseBulkSaving || leaseAccountsLoading || bulkLeaseAccounts.length === 0 || normalizedLeaseRandomCount <= 0"
              @click="createRandomLeases(normalizedLeaseRandomCount)"
            >
              随机自定义 {{ normalizedLeaseRandomCount }} 个
            </button>
          </div>
        </form>
        <form class="space-y-4 rounded-lg border border-gray-200 p-4 dark:border-dark-500" @submit.prevent="updateSelectedLeaseLimits">
          <div>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">批量更改设置</h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">已选择 {{ selectedLeaseIDs.size }} 个租约</p>
          </div>
          <label class="form-field">
            <span class="form-label">最大并发</span>
            <input v-model.number="leaseBulkEditForm.max_concurrency" type="number" min="0" class="input" />
          </label>
          <label class="form-field">
            <span class="form-label">最大请求数</span>
            <input v-model.number="leaseBulkEditForm.max_requests" type="number" min="0" class="input" />
          </label>
          <label class="form-field">
            <span class="form-label">最大 Token</span>
            <input v-model.number="leaseBulkEditForm.max_tokens" type="number" min="0" class="input" />
          </label>
          <button type="submit" class="btn btn-primary w-full" :disabled="leaseBulkUpdating || selectedLeaseIDs.size === 0">
            {{ leaseBulkUpdating ? `保存中 ${leaseBulkProgress.done}/${leaseBulkProgress.total}` : '保存所选租约设置' }}
          </button>
          <div class="grid gap-2 sm:grid-cols-3 xl:grid-cols-1 2xl:grid-cols-3">
            <button type="button" class="btn btn-secondary w-full" :disabled="leaseBulkUpdating || selectedLeaseIDs.size === 0" @click="renewSelectedLeases">
              批量续租
            </button>
            <button type="button" class="btn btn-secondary w-full" :disabled="leaseBulkUpdating || selectedLeaseIDs.size === 0" @click="releaseSelectedLeases">
              批量释放
            </button>
            <button type="button" class="btn btn-danger w-full" :disabled="leaseBulkUpdating || selectedLeaseIDs.size === 0" @click="deleteSelectedLeases">
              批量删除
            </button>
          </div>
        </form>
        </div>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { adminAPI } from '@/api/admin'
import type {
  AccountLease,
  ResetSubsiteSecretResult,
  Subsite,
  SubsiteForwardAffinity,
  SubsiteForwardEvent,
  SubsiteForwardMode,
  SubsiteForwardStats
} from '@/api/admin'
import type { Account, AdminGroup, Proxy } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import { useClipboard } from '@/composables/useClipboard'

const subsites = ref<Subsite[]>([])
const forwardStats = ref<SubsiteForwardStats | null>(null)
const forwardAffinities = ref<SubsiteForwardAffinity[]>([])
const forwardEvents = ref<SubsiteForwardEvent[]>([])
const relayProxies = ref<Proxy[]>([])
const leases = ref<AccountLease[]>([])
const leaseAccounts = ref<Account[]>([])
const leaseGroups = ref<AdminGroup[]>([])
const activeLeaseAccountIds = ref<Set<number>>(new Set())
const loading = ref(false)
const leasesLoading = ref(false)
const saving = ref(false)
const leaseSaving = ref(false)
const leaseBulkSaving = ref(false)
const leaseBulkUpdating = ref(false)
const leaseAccountsLoading = ref(false)
const resettingSecret = ref(false)
const forwardLoading = ref(false)
const forwardSaving = ref(false)
const forwardModeSaving = ref(false)
const autoDistributeSaving = ref(false)
const relayProxiesLoading = ref(false)
const proxyBindingSaving = ref<Set<number>>(new Set())
const autoDistributeMessage = ref('')
const forwardModeForm = ref<SubsiteForwardMode>('forward')
const search = ref('')
const status = ref('')
const leaseAccountSearch = ref('')
const leaseBulkSummary = ref('')
const leaseRandomCount = ref(10)
const searchTimer = ref<number | undefined>()
const showForm = ref(false)
const editing = ref<Subsite | null>(null)
const createdSecret = ref('')
const leaseSubsite = ref<Subsite | null>(null)
const resetTarget = ref<Subsite | null>(null)
const resetSecretResult = ref<ResetSubsiteSecretResult | null>(null)
const capabilitiesText = ref('openai,claude,gemini,images,websocket')
const { copyToClipboard } = useClipboard()

const resetEnvText = computed(() => {
  if (!resetTarget.value || !resetSecretResult.value) return ''
  return [
    `SUBSITE_ID=${resetTarget.value.subsite_id}`,
    `SUBSITE_PUBLIC_URL=${resetTarget.value.public_url}`,
    `SUBSITE_MASTER_URL=${window.location.origin}`,
    `SUBSITE_MASTER_SECRET=${resetSecretResult.value.secret}`
  ].join('\n')
})

const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const leasePagination = reactive({ page: 1, page_size: 10, total: 0 })
const form = reactive({
  subsite_id: '',
  name: '',
  public_url: '',
  region: '',
  version: '',
  max_qps: 0,
  max_concurrency: 0
})
const leaseForm = reactive({
  group_id: 0,
  account_id: 0,
  ttl_hours: 24,
  max_requests: 0,
  max_tokens: 0
})
const leaseBulkEditForm = reactive({
  max_concurrency: 0,
  max_requests: 0,
  max_tokens: 0
})
const forwardLockForm = reactive({
  affinity_key: '',
  subsite_id: '',
  lease_id: '',
  account_id: 0,
  api_key_id: 0,
  ttl_hours: 168,
  locked: true
})
const selectedLeaseIDs = ref<Set<string>>(new Set())

const leaseGroupOptions = computed(() => leaseGroups.value
  .filter((group) => group.status === 'active')
  .map((group) => ({
    id: group.id,
    name: group.name
  })))

const filteredLeaseAccounts = computed(() => {
  if (leaseForm.group_id <= 0) return []
  const query = leaseAccountSearch.value.trim().toLowerCase()
  return leaseAccounts.value.filter((account) => {
    if (activeLeaseAccountIDs.value.has(account.id)) return false
    if (!query) return true
    return accountSearchText(account).includes(query)
  })
})

const filteredAccountOptions = computed(() => filteredLeaseAccounts.value.map((account) => ({
  id: account.id,
  label: accountOptionLabel(account)
})))

const activeLeaseAccountIDs = computed(() => new Set(
  activeLeaseAccountIds.value
))

const accountDistribution = computed(() => forwardStats.value?.account_distribution || [])
const accountDistributionByAccount = computed(() => {
  const priority = (item: typeof accountDistribution.value[number]) => {
    if (item.distributed) return 0
    if (item.distributable) return 1
    if (item.route_resolved === 'master_direct') return 2
    if (item.route_resolved === 'local_only') return 3
    if (item.reason_code === 'ACCOUNT_LEVEL_UNKNOWN') return 4
    if (item.reason_code === 'ACCOUNT_LEVEL_MISMATCH') return 5
    if (item.group_id > 0) return 6
    return 7
  }
  const byID = new Map<number, typeof accountDistribution.value[number]>()
  for (const item of accountDistribution.value) {
    const current = byID.get(item.account_id)
    if (!current || priority(item) < priority(current)) {
      byID.set(item.account_id, item)
    }
  }
  return [...byID.values()]
})
const visibleAccountDistribution = computed(() => accountDistributionByAccount.value.slice(0, 80))
const uniqueAccountIDs = computed(() => new Set(accountDistribution.value.map((item) => item.account_id)))
const distributedAccounts = computed(() => accountDistributionByAccount.value.filter((item) => item.distributed))
const distributedAccountsSummary = computed(() => {
  const visible = distributedAccounts.value.slice(0, 3).map((item) => `#${item.account_id} 到 ${item.subsite_name || item.subsite_id}`)
  const suffix = distributedAccounts.value.length > visible.length ? `，还有 ${distributedAccounts.value.length - visible.length} 个` : ''
  return `${visible.join('；')}${suffix}`
})
const distributedAccountCount = computed(() => distributedAccounts.value.length)
const readyAccountCount = computed(() => accountDistributionByAccount.value.filter((item) => !item.distributed && item.distributable).length)
const masterDirectAccountCount = computed(() => accountDistributionByAccount.value.filter((item) => !item.distributed && item.route_resolved === 'master_direct').length)
const localOnlyAccountCount = computed(() => accountDistributionByAccount.value.filter((item) => !item.distributed && item.route_resolved === 'local_only').length)
const blockedAccountCount = computed(() => accountDistributionByAccount.value.filter((item) => !item.distributed && !item.distributable && !['master_direct', 'local_only'].includes(item.route_resolved || '')).length)
const accountDistributionOverflow = computed(() => Math.max(accountDistributionByAccount.value.length - visibleAccountDistribution.value.length, 0))
const activeRelayProxies = computed(() => relayProxies.value.filter((proxy) => proxy.status === 'active'))
const proxyBoundAccountCount = computed(() => accountDistributionByAccount.value.filter((item) => !!item.proxy_id).length)
const proxyMissingAccountCount = computed(() => accountDistributionByAccount.value.filter((item) => (
  !item.proxy_id && !['master_direct', 'local_only'].includes(item.route_resolved || '')
)).length)
const relayProxySummary = computed(() => {
  const active = activeRelayProxies.value.length
  if (active <= 0) {
    return {
      label: '没有可用代理',
      className: 'badge-warning',
      description: '还没有启用的代理。请先进入 IP 管理添加并测试代理；账号未绑定代理时，子站请求不会强制固定出口 IP。'
    }
  }
  if (proxyMissingAccountCount.value > 0) {
    return {
      label: '部分未绑定',
      className: 'badge-warning',
      description: `当前有 ${proxyMissingAccountCount.value} 个账号没有固定出口 IP。可以手动绑定，也可以让自动分发按代理负载补齐未绑定账号。`
    }
  }
  return {
    label: '固定出口正常',
    className: 'badge-success',
    description: '当前可分发账号都有固定出口 IP。已有绑定会被保留，账号不会因为自动分发而频繁换 IP。'
  }
})
const proxyBindingMessage = computed(() => {
  if (activeRelayProxies.value.length <= 0) return '先到 IP 管理添加代理。添加后回到这里，在账号行里选择固定出口 IP。'
  if (proxyBoundAccountCount.value <= 0) return '还没有账号绑定代理。可以在账号行里手动选择，也可以点击“立即自动分发”让系统按代理负载自动补齐。'
  return `已绑定 ${proxyBoundAccountCount.value} 个账号；未绑定 ${proxyMissingAccountCount.value} 个账号。`
})
const relaySummary = computed(() => {
  const stats = forwardStats.value
  const totalAccounts = uniqueAccountIDs.value.size
  const unknownLevel = accountDistributionByAccount.value.filter((item) => item.reason_code === 'ACCOUNT_LEVEL_UNKNOWN').length
  const levelMismatch = accountDistributionByAccount.value.filter((item) => item.reason_code === 'ACCOUNT_LEVEL_MISMATCH').length
  const shareModeMismatch = accountDistributionByAccount.value.filter((item) => [
    'PRIVATE_SHARE_MODE_MISMATCH',
    'PUBLIC_SHARE_NOT_APPROVED',
    'PRIVATE_OWNER_MISMATCH'
  ].includes(item.reason_code)).length
  const groupBlocked = accountDistributionByAccount.value.filter((item) => [
    'GROUP_NOT_BOUND',
    'GROUP_INACTIVE',
    'PLATFORM_MISMATCH'
  ].includes(item.reason_code)).length
  const temporarilyBlocked = accountDistributionByAccount.value.filter((item) => [
    'ACCOUNT_RATE_LIMITED',
    'ACCOUNT_TEMP_BLOCKED',
    'ACCOUNT_OVERLOADED',
    'ACCOUNT_EXPIRED',
    'ACCOUNT_INACTIVE',
    'ACCOUNT_UNSCHEDULABLE'
  ].includes(item.reason_code)).length
  const masterDirect = masterDirectAccountCount.value
  const localOnly = localOnlyAccountCount.value
  const criticalChecks = stats?.configuration_checks?.filter((check) => check.status !== 'ok' && check.severity === 'critical') || []
  const warningChecks = stats?.configuration_checks?.filter((check) => check.status !== 'ok' && check.severity !== 'critical') || []
  return {
    totalAccounts,
    unknownLevel,
    levelMismatch,
    shareModeMismatch,
    groupBlocked,
    temporarilyBlocked,
    masterDirect,
    localOnly,
    criticalChecks,
    warningChecks
  }
})
const relayHealthState = computed(() => {
  const stats = forwardStats.value
  if (!stats) return { label: '加载中', className: 'badge-gray', title: '正在读取子站数据', description: '请稍等，系统正在加载子站、账号池和租约诊断。' }
  if (relaySummary.value.criticalChecks.length > 0) {
    return { label: '严重阻断', className: 'badge-danger', title: '子站转发当前不可用', description: relaySummary.value.criticalChecks[0]?.message || '存在严重配置问题，需要先处理。' }
  }
  if ((stats.online_subsites || 0) <= 0) {
    return { label: '无在线子站', className: 'badge-danger', title: '没有可接收账号的子站', description: '请先启动子站 Agent，确认心跳正常。' }
  }
  if (distributedAccountCount.value > 0) {
    return { label: '已分发', className: 'badge-success', title: '已有账号进入子站', description: `当前 ${distributedAccountCount.value} 条账号租约正在子站池中。` }
  }
  if (readyAccountCount.value > 0) {
    return { label: '可分发', className: 'badge-warning', title: '有账号可进入子站', description: `还有 ${readyAccountCount.value} 个账号符合规则，点击“立即自动分发”即可创建租约。` }
  }
  if (masterDirectAccountCount.value > 0) {
    return { label: '主站直连', className: 'badge-primary', title: '当前账号池由主站直接调用', description: `有 ${masterDirectAccountCount.value} 个外部 API Key / 自定义 Base URL 账号不会进入子站池，请直接通过主站网关计费调用。` }
  }
  if (relaySummary.value.unknownLevel > 0) {
    return { label: '等级未知', className: 'badge-warning', title: '账号审核通过，但等级未知', description: '账号已经通过审核，但 Free/Plus/Pro 没识别出来。严格价格池不会把 unknown 当作 Free，需要重新校验等级或修复等级识别。' }
  }
  if (blockedAccountCount.value > 0) {
    return { label: '账号被阻断', className: 'badge-warning', title: '账号没有满足分发规则', description: '请查看下方“账号去向明细”的状态说明，处理等级、分组、公私模式或限流问题。' }
  }
  return { label: '暂无账号', className: 'badge-gray', title: '没有可展示的子站账号', description: '请先上传账号，并确认账号已审核通过且绑定到对应分组。' }
})
const relayFlowSteps = computed(() => [
  { label: '账号审核通过', value: accountDistribution.value.filter((item) => item.status === 'active' && item.share_status === 'approved').length, hint: '账号状态 active，共享审核 approved' },
  { label: '进入正确分组', value: accountDistribution.value.filter((item) => item.group_id > 0).length, hint: '公有账号进公共价格池，私有账号进用户私有池' },
  { label: '符合分发规则', value: readyAccountCount.value + distributedAccountCount.value, hint: '等级、模式、限流、过期状态都满足' },
  { label: '已进入子站', value: distributedAccountCount.value, hint: '有有效租约，子站可取用该账号' },
  { label: '主站直连', value: masterDirectAccountCount.value, hint: '外部 API Key / 自定义 Base URL 直接由主站调用，不分发到子站' }
])
const relayAutomationRules = [
  { title: '审核通过后自动进池', description: '账号检测通过并满足等级、分组、公私模式后，系统会在约 10 秒内尝试分发到负载最低的在线子站。' },
  { title: '公私切换会迁移租约', description: '公有改私有、私有改公有、等级变化或分组变化时，旧租约会先被释放，再按新规则重新进入正确账号池。' },
  { title: '每 60 秒自动兜底', description: '后台会定时清理离线子站、限流账号、过期账号和不合规租约，并补充分发漏掉的账号。' }
]
const relayBlockReasons = computed(() => [
  { label: '主站直连账号', value: relaySummary.value.masterDirect, hint: '这些账号按规则不进子站池，不属于分发失败。' },
  { label: '仅主站本地', value: relaySummary.value.localOnly, hint: '这些账号类型不参与子站转发。' },
  { label: '账号等级未知', value: relaySummary.value.unknownLevel, hint: '审核通过但未识别 Free/Plus/Pro，不能进有等级要求的价格池。' },
  { label: '等级不匹配', value: relaySummary.value.levelMismatch, hint: '账号等级和分组要求不同，比如 Free 账号不能进 Plus 分组。' },
  { label: '公私模式不匹配', value: relaySummary.value.shareModeMismatch, hint: '账号被设为公有/私有后，只能进入对应池子。' },
  { label: '分组未绑定或异常', value: relaySummary.value.groupBlocked, hint: '账号没有绑定分组，或分组未启用、平台不一致。' },
  { label: '账号临时不可用', value: relaySummary.value.temporarilyBlocked, hint: '账号被限流、过载、过期、停用或不可调度。' }
].filter((item) => item.value > 0))
const topAccountBlockReasons = computed(() => {
  if (readyAccountCount.value > 0) {
    return [
      { label: '可分发但还没创建租约', value: readyAccountCount.value, hint: '这些账号已经满足规则，点击“立即自动分发”后会进入在线子站。' },
      ...relayBlockReasons.value
    ].slice(0, 4)
  }
  return relayBlockReasons.value.slice(0, 4)
})
const autoDistributeDetails = ref<{
  created: Array<{ lease_id: string; account_id: number; account_name?: string | null; subsite_id: string; group_name?: string | null }>
  skipped: Array<{ account_id: number; account_name: string; group_name: string; reason: string }>
} | null>(null)

const bulkLeaseAccounts = computed(() => filteredLeaseAccounts.value)
const allVisibleLeasesSelected = computed(() => leases.value.length > 0 && leases.value.every((lease) => selectedLeaseIDs.value.has(lease.lease_id)))
const randomTenCount = computed(() => Math.min(10, bulkLeaseAccounts.value.length))
const randomTwentyCount = computed(() => Math.min(20, bulkLeaseAccounts.value.length))
const normalizedLeaseRandomCount = computed(() => Math.min(
  Math.max(Number.isFinite(leaseRandomCount.value) ? Math.floor(leaseRandomCount.value) : 0, 0),
  bulkLeaseAccounts.value.length
))

const leaseBulkProgress = reactive({
  done: 0,
  total: 0
})

const leaseAccountEmptyText = computed(() => {
  if (leaseForm.group_id <= 0) return '请先选择分组'
  if (leaseAccountSearch.value.trim()) return '没有匹配当前搜索的可用账号'
  return '当前分组下没有可用活跃账号'
})

const columns = computed<Column[]>(() => [
  { key: 'name', label: '子站' },
  { key: 'status', label: '状态' },
  { key: 'public_url', label: '入口 URL' },
  { key: 'region', label: '区域' },
  { key: 'health_score', label: '健康' },
  { key: 'limits', label: '限制' },
  { key: 'actions', label: '操作', class: 'text-right' }
])

const leaseColumns = computed<Column[]>(() => [
  { key: 'select', label: '' },
  { key: 'lease_id', label: '租约 ID' },
  { key: 'group', label: '分组' },
  { key: 'account', label: '账号' },
  { key: 'status', label: '状态' },
  { key: 'usage', label: '用量' },
  { key: 'expires_at', label: '过期时间' },
  { key: 'actions', label: '操作', class: 'text-right' }
])

async function reload(): Promise<void> {
  loading.value = true
  try {
    const result = await adminAPI.subsites.list(pagination.page, pagination.page_size, {
      search: search.value || undefined,
      status: status.value || undefined
    })
    subsites.value = result.items
    pagination.total = result.total
    if (!forwardLockForm.subsite_id && result.items.length > 0) {
      forwardLockForm.subsite_id = result.items[0].subsite_id
    }
  } finally {
    loading.value = false
  }
}

async function loadForwardConsole(): Promise<void> {
  forwardLoading.value = true
  try {
    await Promise.all([
      loadForwardStats(),
      loadForwardAffinities(),
      loadForwardEvents(),
      loadRelayProxies()
    ])
  } finally {
    forwardLoading.value = false
  }
}

async function loadForwardStats(): Promise<void> {
  forwardStats.value = await adminAPI.subsites.forwardStats()
  forwardModeForm.value = forwardStats.value.mode || 'forward'
}

async function loadForwardAffinities(): Promise<void> {
  const result = await adminAPI.subsites.listForwardAffinities(1, 8)
  forwardAffinities.value = result.items
}

async function loadForwardEvents(): Promise<void> {
  const result = await adminAPI.subsites.listForwardEvents(1, 8)
  forwardEvents.value = result.items
}

async function loadRelayProxies(): Promise<void> {
  relayProxiesLoading.value = true
  try {
    relayProxies.value = await adminAPI.proxies.getAllWithCount()
  } finally {
    relayProxiesLoading.value = false
  }
}

async function saveForwardMode(): Promise<void> {
  forwardModeSaving.value = true
  try {
    const result = await adminAPI.subsites.updateForwardMode(forwardModeForm.value)
    forwardModeForm.value = result.mode
    await loadForwardStats()
  } finally {
    forwardModeSaving.value = false
  }
}

async function runAutoDistribute(): Promise<void> {
  autoDistributeSaving.value = true
  autoDistributeMessage.value = ''
  autoDistributeDetails.value = null
  try {
    const result = await adminAPI.subsites.autoDistribute()
    autoDistributeMessage.value = `自动分发完成：新增租约 ${result.created_count || 0} 个，跳过 ${result.skipped_count || 0} 个，释放无效租约 ${result.released_invalid_leases || 0} 个，在线子站 ${result.online_subsites || 0} 个。`
    autoDistributeDetails.value = {
      created: (result.created_leases || []).slice(0, 12).map((lease) => ({
        lease_id: lease.lease_id,
        account_id: lease.account_id,
        account_name: lease.account_name,
        subsite_id: lease.subsite_id,
        group_name: lease.group_name
      })),
      skipped: (result.skipped_accounts || []).slice(0, 12).map((item) => ({
        account_id: item.account_id,
        account_name: item.account_name,
        group_name: item.group_name,
        reason: item.reason || distributionReasonLabel(item.reason_code)
      }))
    }
    await loadForwardConsole()
  } finally {
    autoDistributeSaving.value = false
  }
}

function handleAccountProxyChange(accountID: number, event: Event): void {
  const target = event.target as HTMLSelectElement | null
  if (!target) return
  void updateAccountProxyBinding(accountID, target.value)
}

async function updateAccountProxyBinding(accountID: number, rawProxyID: number | string): Promise<void> {
  const proxyID = Number(rawProxyID) || 0
  const nextSaving = new Set(proxyBindingSaving.value)
  nextSaving.add(accountID)
  proxyBindingSaving.value = nextSaving
  try {
    await adminAPI.accounts.bulkUpdate([accountID], { proxy_id: proxyID })
    await loadForwardStats()
    await loadRelayProxies()
  } finally {
    const doneSaving = new Set(proxyBindingSaving.value)
    doneSaving.delete(accountID)
    proxyBindingSaving.value = doneSaving
  }
}

async function saveForwardLock(): Promise<void> {
  if (!forwardLockForm.affinity_key || !forwardLockForm.subsite_id) return
  forwardSaving.value = true
  try {
    await adminAPI.subsites.upsertForwardAffinity({
      affinity_key: forwardLockForm.affinity_key,
      affinity_type: forwardLockForm.account_id > 0 ? 'account' : 'manual',
      subsite_id: forwardLockForm.subsite_id,
      lease_id: forwardLockForm.lease_id || undefined,
      account_id: forwardLockForm.account_id || undefined,
      api_key_id: forwardLockForm.api_key_id || undefined,
      ttl_seconds: Math.max(1, normalizeNonNegativeNumber(forwardLockForm.ttl_hours || 168)) * 3600,
      locked: forwardLockForm.locked
    })
    forwardLockForm.affinity_key = ''
    forwardLockForm.lease_id = ''
    forwardLockForm.account_id = 0
    forwardLockForm.api_key_id = 0
    await loadForwardConsole()
  } finally {
    forwardSaving.value = false
  }
}

async function deleteForwardAffinity(id: number): Promise<void> {
  const confirmed = window.confirm('确认删除这条粘性路由？')
  if (!confirmed) return
  await adminAPI.subsites.deleteForwardAffinity(id)
  await loadForwardConsole()
}

function handleSearch(): void {
  window.clearTimeout(searchTimer.value)
  searchTimer.value = window.setTimeout(() => {
    pagination.page = 1
    void reload()
  }, 300)
}

function handlePageChange(page: number): void {
  pagination.page = page
  void reload()
}

function handlePageSizeChange(pageSize: number): void {
  pagination.page_size = pageSize
  pagination.page = 1
  void reload()
}

function openCreate(): void {
  editing.value = null
  Object.assign(form, { subsite_id: '', name: '', public_url: '', region: '', version: '', max_qps: 0, max_concurrency: 0 })
  capabilitiesText.value = 'openai,claude,gemini,images,websocket'
  showForm.value = true
}

function openEdit(row: Subsite): void {
  editing.value = row
  Object.assign(form, {
    subsite_id: row.subsite_id,
    name: row.name,
    public_url: row.public_url,
    region: row.region,
    version: row.version,
    max_qps: row.max_qps,
    max_concurrency: row.max_concurrency
  })
  capabilitiesText.value = (row.capabilities || []).join(',')
  showForm.value = true
}

function closeForm(): void {
  showForm.value = false
  editing.value = null
}

async function saveSubsite(): Promise<void> {
  saving.value = true
  const payload = {
    ...form,
    capabilities: capabilitiesText.value.split(',').map((item) => item.trim()).filter(Boolean)
  }
  try {
    if (editing.value) {
      await adminAPI.subsites.update(editing.value.subsite_id, payload)
    } else {
      const result = await adminAPI.subsites.create(payload)
      createdSecret.value = result.secret
    }
    closeForm()
    await reload()
  } finally {
    saving.value = false
  }
}

async function changeStatus(row: Subsite, action: 'activate' | 'pause' | 'resume'): Promise<void> {
  if (action === 'activate') await adminAPI.subsites.activate(row.subsite_id)
  if (action === 'pause') await adminAPI.subsites.pause(row.subsite_id)
  if (action === 'resume') await adminAPI.subsites.resume(row.subsite_id)
  await reload()
}

function openResetSecret(row: Subsite): void {
  resetTarget.value = row
  resetSecretResult.value = null
}

function closeResetSecret(): void {
  if (resettingSecret.value) return
  resetTarget.value = null
  resetSecretResult.value = null
}

async function resetSubsiteSecret(): Promise<void> {
  if (!resetTarget.value) return
  resettingSecret.value = true
  try {
    resetSecretResult.value = await adminAPI.subsites.resetSecret(resetTarget.value.subsite_id)
    await reload()
  } finally {
    resettingSecret.value = false
  }
}

async function openLeases(row: Subsite): Promise<void> {
  leaseSubsite.value = row
  resetLeaseForm()
  leasePagination.page = 1
  await Promise.all([loadLeases(), loadLeaseGroups()])
}

function closeLeases(): void {
  leaseSubsite.value = null
  leases.value = []
  leaseAccounts.value = []
  selectedLeaseIDs.value = new Set()
  activeLeaseAccountIds.value = new Set()
  leasePagination.page = 1
  leasePagination.total = 0
  resetLeaseForm()
}

async function loadLeases(): Promise<void> {
  if (!leaseSubsite.value) return
  leasesLoading.value = true
  try {
    const result = await adminAPI.subsites.listLeases(
      leaseSubsite.value.subsite_id,
      leasePagination.page,
      leasePagination.page_size
    )
    leases.value = result.items
    leasePagination.total = result.total
    selectedLeaseIDs.value = new Set([...selectedLeaseIDs.value].filter((leaseID) => result.items.some((item) => item.lease_id === leaseID)))
    if (result.items.length === 0 && result.total > 0 && leasePagination.page > 1) {
      leasePagination.page = Math.max(1, Math.ceil(result.total / leasePagination.page_size))
      await loadLeases()
      return
    }
    await refreshActiveLeaseAccountIds()
  } finally {
    leasesLoading.value = false
  }
}

async function refreshActiveLeaseAccountIds(): Promise<void> {
  if (!leaseSubsite.value) return
  const accountIds = await adminAPI.subsites.listLeaseActiveAccountIds(leaseSubsite.value.subsite_id)
  activeLeaseAccountIds.value = new Set(accountIds)
}

function handleLeasePageChange(page: number): void {
  leasePagination.page = page
  void loadLeases()
}

function handleLeasePageSizeChange(pageSize: number): void {
  leasePagination.page_size = pageSize
  leasePagination.page = 1
  void loadLeases()
}

async function loadLeaseAccounts(): Promise<void> {
  if (leaseForm.group_id <= 0) {
    leaseAccounts.value = []
    return
  }
  leaseAccountsLoading.value = true
  try {
    const pageSize = 200
    const items: Account[] = []
    let page = 1
    let total = 0
    do {
      const result = await adminAPI.accounts.list(page, pageSize, {
        status: 'active',
        group: String(leaseForm.group_id),
        search: '',
        sort_by: 'id',
        sort_order: 'desc'
      })
      items.push(...result.items)
      total = result.total
      page += 1
    } while (items.length < total)
    leaseAccounts.value = items
  } finally {
    leaseAccountsLoading.value = false
  }
}

async function loadLeaseGroups(): Promise<void> {
  leaseGroups.value = await adminAPI.groups.getAll(undefined, 'all')
}

function handleLeaseGroupChange(): void {
  leaseForm.account_id = 0
  leaseAccountSearch.value = ''
  leaseAccounts.value = []
  leaseBulkSummary.value = ''
  void loadLeaseAccounts()
}

function resetLeaseForm(): void {
  Object.assign(leaseForm, {
    group_id: 0,
    account_id: 0,
    ttl_hours: 24,
    max_requests: 0,
    max_tokens: 0
  })
  leaseAccountSearch.value = ''
  leaseBulkSummary.value = ''
  leaseBulkProgress.done = 0
  leaseBulkProgress.total = 0
  leaseRandomCount.value = 10
  resetLeaseBulkEditForm()
}

function resetLeaseBulkEditForm(): void {
  Object.assign(leaseBulkEditForm, {
    max_concurrency: 0,
    max_requests: 0,
    max_tokens: 0
  })
}

function toggleLeaseSelection(row: AccountLease): void {
  const next = new Set(selectedLeaseIDs.value)
  if (next.has(row.lease_id)) {
    next.delete(row.lease_id)
  } else {
    next.add(row.lease_id)
  }
  selectedLeaseIDs.value = next
}

function toggleAllVisibleLeases(): void {
  if (allVisibleLeasesSelected.value) {
    selectedLeaseIDs.value = new Set()
    return
  }
  selectedLeaseIDs.value = new Set(leases.value.map((lease) => lease.lease_id))
}

async function createLease(): Promise<void> {
  if (!leaseSubsite.value) return
  leaseSaving.value = true
  try {
    await adminAPI.subsites.createLease(leaseSubsite.value.subsite_id, buildLeasePayload(leaseForm.account_id))
    resetLeaseForm()
    await loadLeases()
  } finally {
    leaseSaving.value = false
  }
}

async function updateSelectedLeaseLimits(): Promise<void> {
  if (!leaseSubsite.value || selectedLeaseIDs.value.size === 0) return
  const leaseIDs = [...selectedLeaseIDs.value]
  const confirmed = window.confirm(`确认更新 ${leaseIDs.length} 个租约的请求数和最大 Token 设置？`)
  if (!confirmed) return

  leaseBulkUpdating.value = true
  leaseBulkSummary.value = ''
  leaseBulkProgress.done = 0
  leaseBulkProgress.total = leaseIDs.length
  let success = 0
  let failed = 0
  try {
    for (const leaseID of leaseIDs) {
      try {
        await adminAPI.subsites.updateLease(leaseSubsite.value.subsite_id, leaseID, {
          max_concurrency: normalizeNonNegativeNumber(leaseBulkEditForm.max_concurrency),
          max_requests: normalizeNonNegativeNumber(leaseBulkEditForm.max_requests),
          max_tokens: normalizeNonNegativeNumber(leaseBulkEditForm.max_tokens)
        })
        success += 1
      } catch {
        failed += 1
      } finally {
        leaseBulkProgress.done += 1
      }
    }
    leaseBulkSummary.value = `批量保存完成：成功 ${success} 个，失败 ${failed} 个`
    selectedLeaseIDs.value = new Set()
    await loadLeases()
  } finally {
    leaseBulkUpdating.value = false
  }
}

async function renewSelectedLeases(): Promise<void> {
  await runSelectedLeaseAction('续租 24 小时', async (leaseID) => {
    if (!leaseSubsite.value) return
    await adminAPI.subsites.renewLease(leaseSubsite.value.subsite_id, leaseID, { ttl_seconds: 24 * 3600 })
  })
}

async function releaseSelectedLeases(): Promise<void> {
  await runSelectedLeaseAction('释放', async (leaseID) => {
    if (!leaseSubsite.value) return
    await adminAPI.subsites.releaseLease(leaseSubsite.value.subsite_id, leaseID)
  })
}

async function deleteSelectedLeases(): Promise<void> {
  await runSelectedLeaseAction('删除', async (leaseID) => {
    if (!leaseSubsite.value) return
    await adminAPI.subsites.deleteLease(leaseSubsite.value.subsite_id, leaseID)
  })
}

async function runSelectedLeaseAction(label: string, action: (leaseID: string) => Promise<void>): Promise<void> {
  if (!leaseSubsite.value || selectedLeaseIDs.value.size === 0) return
  const leaseIDs = [...selectedLeaseIDs.value]
  const confirmed = window.confirm(`确认${label} ${leaseIDs.length} 个租约？`)
  if (!confirmed) return

  leaseBulkUpdating.value = true
  leaseBulkSummary.value = ''
  leaseBulkProgress.done = 0
  leaseBulkProgress.total = leaseIDs.length
  let success = 0
  let failed = 0
  try {
    for (const leaseID of leaseIDs) {
      try {
        await action(leaseID)
        success += 1
      } catch {
        failed += 1
      } finally {
        leaseBulkProgress.done += 1
      }
    }
    leaseBulkSummary.value = `批量${label}完成：成功 ${success} 个，失败 ${failed} 个`
    selectedLeaseIDs.value = new Set()
    await loadLeases()
  } finally {
    leaseBulkUpdating.value = false
  }
}

async function createAllVisibleLeases(): Promise<void> {
  if (!leaseSubsite.value || bulkLeaseAccounts.value.length === 0) return
  const accounts = [...bulkLeaseAccounts.value]
  await createBulkLeases(accounts, `确认给当前筛选出的 ${accounts.length} 个账号全部创建租约？`)
}

async function createRandomLeases(count: number): Promise<void> {
  if (!leaseSubsite.value || bulkLeaseAccounts.value.length === 0) return
  const normalizedCount = Math.min(Math.max(Math.floor(count), 0), bulkLeaseAccounts.value.length)
  if (normalizedCount <= 0) return
  const accounts = shuffleAccounts(bulkLeaseAccounts.value).slice(0, normalizedCount)
  await createBulkLeases(accounts, `确认随机选择 ${accounts.length} 个可用账号创建租约？`)
}

async function createBulkLeases(accounts: Account[], confirmMessage: string): Promise<void> {
  if (!leaseSubsite.value || accounts.length === 0) return
  const confirmed = window.confirm(confirmMessage)
  if (!confirmed) return

  leaseBulkSaving.value = true
  leaseBulkSummary.value = ''
  leaseBulkProgress.done = 0
  leaseBulkProgress.total = accounts.length
  let success = 0
  let failed = 0
  try {
    for (const account of accounts) {
      try {
        await adminAPI.subsites.createLease(leaseSubsite.value.subsite_id, buildLeasePayload(account.id))
        success += 1
      } catch {
        failed += 1
      } finally {
        leaseBulkProgress.done += 1
      }
    }
    leaseBulkSummary.value = `批量添加完成：成功 ${success} 个，失败 ${failed} 个`
    leaseForm.account_id = 0
    leasePagination.page = 1
    await loadLeases()
  } finally {
    leaseBulkSaving.value = false
  }
}

function shuffleAccounts(accounts: Account[]): Account[] {
  const shuffled = [...accounts]
  for (let index = shuffled.length - 1; index > 0; index -= 1) {
    const swapIndex = Math.floor(Math.random() * (index + 1))
    const current = shuffled[index]
    shuffled[index] = shuffled[swapIndex]
    shuffled[swapIndex] = current
  }
  return shuffled
}

function buildLeasePayload(accountID: number) {
  return {
    group_id: leaseForm.group_id,
    account_id: accountID,
    ttl_seconds: Math.max(1, leaseForm.ttl_hours) * 3600,
    max_requests: leaseForm.max_requests || 0,
    max_tokens: leaseForm.max_tokens || 0
  }
}

function normalizeNonNegativeNumber(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.floor(value))
}

function accountOptionLabel(account: Account): string {
  const parts = [`#${account.id}`, account.name || account.platform || 'account']
  if (account.account_level && account.account_level !== 'unknown') parts.push(account.account_level)
  if (!account.schedulable) parts.push('不可调度')
  return parts.join(' · ')
}

function accountSearchText(account: Account): string {
  return [
    account.id,
    account.name,
    account.platform,
    account.account_level,
    account.type,
    account.status
  ].filter((item) => item !== undefined && item !== null).join(' ').toLowerCase()
}

async function renewLease(row: AccountLease): Promise<void> {
  if (!leaseSubsite.value) return
  await adminAPI.subsites.renewLease(leaseSubsite.value.subsite_id, row.lease_id, { ttl_seconds: 24 * 3600 })
  await loadLeases()
}

async function drainLease(row: AccountLease): Promise<void> {
  if (!leaseSubsite.value) return
  await adminAPI.subsites.drainLease(leaseSubsite.value.subsite_id, row.lease_id)
  await loadLeases()
}

async function releaseLease(row: AccountLease): Promise<void> {
  if (!leaseSubsite.value) return
  await adminAPI.subsites.releaseLease(leaseSubsite.value.subsite_id, row.lease_id)
  selectedLeaseIDs.value = new Set([...selectedLeaseIDs.value].filter((leaseID) => leaseID !== row.lease_id))
  await loadLeases()
}

async function deleteLease(row: AccountLease): Promise<void> {
  if (!leaseSubsite.value) return
  const confirmed = window.confirm(`确认删除租约 ${row.lease_id}？该操作不可撤销。`)
  if (!confirmed) return
  await adminAPI.subsites.deleteLease(leaseSubsite.value.subsite_id, row.lease_id)
  selectedLeaseIDs.value = new Set([...selectedLeaseIDs.value].filter((leaseID) => leaseID !== row.lease_id))
  await loadLeases()
}

async function copy(value: string): Promise<void> {
  await copyToClipboard(value)
}

async function copyClientConfig(row: Subsite): Promise<void> {
  const config = {
    base_url: row.public_url,
    endpoints: [
      '/v1/messages',
      '/v1/responses',
      '/v1/chat/completions',
      '/v1beta/models/*'
    ]
  }
  await copyToClipboard(JSON.stringify(config, null, 2))
}

function statusLabel(value: string): string {
  return ({ pending: '待激活', active: '运行中', maintenance: '维护中', unhealthy: '异常', disabled: '已停用' } as Record<string, string>)[value] || value
}

function statusClass(value: string): string {
  return ({ active: 'badge-success', pending: 'badge-warning', maintenance: 'badge-warning', unhealthy: 'badge-danger', disabled: 'badge-gray' } as Record<string, string>)[value] || 'badge-gray'
}

function leaseStatusLabel(value: string): string {
  return ({ active: '可用', renewing: '续租中', draining: '排空中', released: '已释放', expired: '已过期', revoked: '已撤销' } as Record<string, string>)[value] || value
}

function leaseStatusClass(value: string): string {
  return ({ active: 'badge-success', renewing: 'badge-primary', draining: 'badge-warning', released: 'badge-gray', expired: 'badge-gray', revoked: 'badge-danger' } as Record<string, string>)[value] || 'badge-gray'
}

function leaseGroupLabel(row: AccountLease): string {
  return row.group_name || (row.group_id ? `#${row.group_id}` : '-')
}

function leaseGroupMeta(row: AccountLease): string {
  return row.group_id ? `分组 ID：${row.group_id}` : '未绑定分组'
}

function leaseAccountLabel(row: AccountLease): string {
  return row.account_name || `#${row.account_id}`
}

function formatDate(value?: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function formatCompact(value: number): string {
  return new Intl.NumberFormat(undefined, {
    notation: 'compact',
    maximumFractionDigits: 1
  }).format(value || 0)
}

function formatPercent(value: number): string {
  const normalized = value > 1 ? value / 100 : value
  return `${(normalized * 100).toFixed(1)}%`
}

function formatCost(value: number): string {
  return `$${(value || 0).toFixed(4)}`
}

function platformLabel(value?: string): string {
  return ({
    openai: 'OpenAI',
    anthropic: 'Claude',
    gemini: 'Gemini',
    antigravity: 'Antigravity'
  } as Record<string, string>)[value || ''] || (value || '-')
}

function levelLabel(value?: string): string {
  return ({
    free: 'Free',
    plus: 'Plus',
    pro: 'Pro',
    team: 'Team',
    unknown: '未知'
  } as Record<string, string>)[value || ''] || '不限'
}

function scopeLabel(value?: string): string {
  return ({
    public: '公有',
    private: '私有',
    user_private: '用户私有'
  } as Record<string, string>)[value || ''] || (value || '-')
}

function shareStatusLabel(value?: string): string {
  return ({
    approved: '审核通过',
    pending: '待审核',
    suspended: '已暂停'
  } as Record<string, string>)[value || ''] || (value || '-')
}

function configCheckLabel(code: string): string {
  return ({
    GLOBAL_SHARE_POLICY: '全局分润策略',
    ONLINE_SUBSITE: '在线子站',
    PUBLIC_OPENAI_FREE_GROUP: 'OpenAI Free 公共分组',
    PUBLIC_OPENAI_PLUS_GROUP: 'OpenAI Plus 公共分组',
    PUBLIC_OPENAI_PRO_GROUP: 'OpenAI Pro 公共分组',
    DUPLICATE_EFFECTIVE_LEASE: '重复有效租约',
    PENDING_WITHOUT_REASON: '待审账号原因',
    LEVEL_MISMATCH_LEASE: '租约等级匹配',
    SHARE_MODE_MISMATCH_LEASE: '公私模式匹配'
  } as Record<string, string>)[code] || code
}

function adviceLabel(code: string): string {
  if (code.startsWith('POOL_NOT_SCHEDULABLE')) return '账号池不可分发'
  if (code.startsWith('POOL_HAS_UNLEASED')) return '存在未分发账号'
  if (code.startsWith('POOL_HAS_UNKNOWN_LEVEL')) return '账号等级未知'
  return ({
    NO_SUBSITE: '没有子站',
    NO_ONLINE_SUBSITE: '没有在线子站',
    NO_ACTIVE_LEASE: '没有活跃租约',
    LOW_SUCCESS_RATE: '转发成功率偏低',
    LOW_CACHE_HIT: '缓存命中率偏低',
    LEASE_EXPIRING: '租约即将过期',
    NO_RELAY_POOL_ACCOUNT: '没有账号池账号'
  } as Record<string, string>)[code] || configCheckLabel(code)
}

function severityLabel(value?: string): string {
  return ({
    critical: '严重',
    warning: '警告',
    info: '提示'
  } as Record<string, string>)[value || ''] || (value || '-')
}

function accountDistributionLabel(item: { distributed: boolean; distributable: boolean; route_resolved?: string }): string {
  if (item.distributed) return '已分发'
  if (item.distributable) return '可分发'
  if (item.route_resolved === 'master_direct') return '主站直连'
  if (item.route_resolved === 'local_only') return '仅主站本地'
  return '不可分发'
}

function accountDistributionClass(item: { distributed: boolean; distributable: boolean; route_resolved?: string }): string {
  if (item.distributed) return 'badge-success'
  if (item.distributable) return 'badge-warning'
  if (item.route_resolved === 'master_direct') return 'badge-primary'
  if (item.route_resolved === 'local_only') return 'badge-secondary'
  return 'badge-gray'
}

function routePolicyLabel(value?: string): string {
  return ({
    auto: '自动判断',
    subsite_relay: '可分发到子站',
    master_direct: '主站直连',
    local_only: '仅主站本地'
  } as Record<string, string>)[value || ''] || (value || '-')
}

function proxyAffinityLabel(item: { proxy_id?: number; proxy_name?: string; proxy_protocol?: string; proxy_host?: string; proxy_port?: number }): string {
  if (!item.proxy_id) return '未绑定代理，当前不会强制固定出口 IP'
  const name = item.proxy_name || `代理 #${item.proxy_id}`
  const endpoint = item.proxy_host ? `${item.proxy_protocol || 'proxy'}://${item.proxy_host}${item.proxy_port ? `:${item.proxy_port}` : ''}` : ''
  return endpoint ? `${name}（${endpoint}）` : name
}

function proxySelectLabel(proxy: Proxy): string {
  const region = proxy.country || proxy.country_code || proxy.city || ''
  const ip = proxy.ip_address ? ` / ${proxy.ip_address}` : ''
  const load = proxy.account_count !== undefined ? ` / ${proxy.account_count} 个账号` : ''
  const suffix = `${region}${ip}${load}`
  return suffix ? `${proxy.name}（${suffix}）` : proxy.name
}

function distributionReasonLabel(code?: string): string {
  return ({
    DISTRIBUTED: '已分发到子站',
    MASTER_DIRECT: '主站直连账号',
    LOCAL_ONLY: '仅主站本地',
    READY_TO_DISTRIBUTE: '符合分发条件',
    GROUP_NOT_BOUND: '账号未绑定分组',
    GROUP_INACTIVE: '分组未启用',
    ACCOUNT_INACTIVE: '账号未启用',
    ACCOUNT_UNSCHEDULABLE: '账号不可调度',
    ACCOUNT_EXPIRED: '账号已过期',
    ACCOUNT_OVERLOADED: '账号过载冷却中',
    ACCOUNT_RATE_LIMITED: '账号限流恢复中',
    ACCOUNT_TEMP_BLOCKED: '账号临时不可用',
    PLATFORM_MISMATCH: '平台不匹配',
    PRIVATE_OWNER_MISMATCH: '私有账号所有者不匹配',
    PRIVATE_SHARE_MODE_MISMATCH: '私有分组只能使用私有账号',
    PUBLIC_SHARE_NOT_APPROVED: '公有账号未审核通过',
    ACCOUNT_LEVEL_UNKNOWN: '账号等级未知',
    ACCOUNT_LEVEL_MISMATCH: '账号等级不匹配',
    NO_ONLINE_SUBSITE: '没有在线子站',
    ALREADY_DISTRIBUTED: '账号已经在一个子站池中'
  } as Record<string, string>)[code || ''] || (code || '未命中分发规则')
}

function loadLevelLabel(value?: string): string {
  return ({
    idle: '空闲',
    warm: '正常',
    busy: '繁忙',
    low: '空闲',
    medium: '中等',
    high: '繁忙',
    critical: '高压',
    offline: '离线',
    blocked: '阻断'
  } as Record<string, string>)[value || ''] || (value || '-')
}

function loadLevelClass(value?: string): string {
  return ({
    idle: 'badge-success',
    warm: 'badge-primary',
    busy: 'badge-warning',
    low: 'badge-success',
    medium: 'badge-primary',
    high: 'badge-warning',
    critical: 'badge-danger',
    offline: 'badge-gray',
    blocked: 'badge-danger'
  } as Record<string, string>)[value || ''] || 'badge-gray'
}

function affinitySourceLabel(value?: string): string {
  return ({
    auto: '自动生成',
    manual: '手动设置',
    fallback: '故障回退',
    imported: '导入'
  } as Record<string, string>)[value || ''] || (value || '-')
}

function breakerScopeLabel(value?: string): string {
  return ({
    subsite: '子站',
    account: '账号',
    lease: '租约'
  } as Record<string, string>)[value || ''] || (value || '-')
}

function forwardOutcomeLabel(value?: string): string {
  return ({
    success: '成功',
    failed: '失败',
    no_candidate: '无可用候选',
    fallback: '已回退',
    client_error: '客户端错误',
    upstream_error: '上游错误'
  } as Record<string, string>)[value || ''] || (value || '-')
}

function forwardReasonLabel(value?: string): string {
  if (!value) return '-'
  return ({
    affinity_hit: '命中粘性路由',
    affinity_miss: '未命中粘性路由',
    selected_best_subsite: '选择最佳子站',
    no_available_subsite: '没有可用子站',
    no_active_lease: '没有有效租约',
    circuit_open: '熔断中',
    upstream_error: '上游错误',
    client_error: '客户端错误',
    fallback: '回退'
  } as Record<string, string>)[value] || value
}

function adviceClass(value?: string): string {
  return ({
    critical: 'border-red-200 bg-red-50 text-red-700 dark:border-red-900/50 dark:bg-red-950/20 dark:text-red-200',
    warning: 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900/50 dark:bg-amber-950/20 dark:text-amber-200',
    info: 'border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900/50 dark:bg-sky-950/20 dark:text-sky-200'
  } as Record<string, string>)[value || ''] || 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200'
}

function eventRouteMetadata(event: SubsiteForwardEvent): Record<string, unknown> | null {
  const route = event.metadata?.route
  if (!route || typeof route !== 'object' || Array.isArray(route)) return null
  return route as Record<string, unknown>
}

function routeNumber(route: Record<string, unknown>, key: string): number {
  const value = route[key]
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : 0
  }
  return 0
}

function routeText(route: Record<string, unknown>, key: string): string {
  const value = route[key]
  if (typeof value === 'string') return value
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  if (typeof value === 'boolean') return value ? 'true' : 'false'
  return ''
}

function routeList(route: Record<string, unknown>, key: string): string[] {
  const value = route[key]
  if (!Array.isArray(value)) return []
  return value
    .map((item) => String(item || '').trim())
    .filter(Boolean)
}

function compactRouteList(label: string, values: string[]): string {
  if (!values.length) return ''
  const visible = values.slice(0, 3).join(', ')
  const suffix = values.length > 3 ? ` +${values.length - 3}` : ''
  return `${label}: ${visible}${suffix}`
}

function eventRouteSummary(event: SubsiteForwardEvent): string {
  const route = eventRouteMetadata(event)
  if (!route) return ''
  const total = routeNumber(route, 'total_subsites')
  const candidates = routeNumber(route, 'candidate_subsites')
  const selected = routeText(route, 'selected_subsite_id') || event.subsite_id || event.attempted_subsite_id || '-'
  const reason = routeText(route, 'selected_reason') || event.reason || '-'
  return `候选 ${candidates}/${total}，选择 ${selected}，原因 ${reason}`
}

function eventRouteDetails(event: SubsiteForwardEvent): string[] {
  const route = eventRouteMetadata(event)
  if (!route) return []
  return [
    compactRouteList('容量满', routeList(route, 'capacity_limited_subsite_ids')),
    compactRouteList('能力不匹配', routeList(route, 'capability_mismatch_subsite_ids')),
    compactRouteList('熔断子站', routeList(route, 'circuit_blocked_subsite_ids')),
    compactRouteList('重试排除', routeList(route, 'retry_excluded_subsite_ids')),
    compactRouteList('不可用', routeList(route, 'ineligible_subsite_ids')),
    compactRouteList('熔断账号', routeList(route, 'circuit_blocked_account_ids')),
    compactRouteList('熔断租约', routeList(route, 'circuit_blocked_lease_ids'))
  ].filter(Boolean)
}

onMounted(() => {
  void reload()
  void loadForwardConsole()
})
</script>
