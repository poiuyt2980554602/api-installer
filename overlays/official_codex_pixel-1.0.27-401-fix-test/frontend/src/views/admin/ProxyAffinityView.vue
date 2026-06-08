<template>
  <AppLayout>
    <div class="space-y-6">
      <section class="rounded-2xl border border-slate-200 bg-gradient-to-br from-slate-950 via-slate-900 to-cyan-950 p-6 text-white shadow-sm dark:border-dark-700">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <p class="text-sm font-medium text-cyan-200">Proxy Affinity</p>
            <h1 class="mt-1 text-2xl font-semibold">代理亲和调度</h1>
            <p class="mt-2 max-w-3xl text-sm leading-6 text-slate-300">
              把账号稳定绑定到固定代理 IP。新账号按负载自动分配；已绑定账号默认不迁移，避免同一个账号频繁换 IP。
            </p>
          </div>
          <div class="flex flex-wrap gap-2">
            <button type="button" class="btn border-white/20 bg-white/10 text-white hover:bg-white/20" :disabled="loading" @click="loadAll">
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              刷新
            </button>
            <button type="button" class="btn border-white/20 bg-white/10 text-white hover:bg-white/20" :disabled="assigning" @click="runAssign(true)">
              预览分配
            </button>
            <button type="button" class="btn border-white/20 bg-white/10 text-white hover:bg-white/20" :disabled="assigning" @click="runPrebind(false)">
              预绑定待校验账号
            </button>
            <button type="button" class="btn bg-cyan-500 text-white hover:bg-cyan-400" :disabled="assigning" @click="runAssign(false)">
              执行分配
            </button>
          </div>
        </div>
      </section>

      <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-6">
        <div v-for="item in statCards" :key="item.label" class="rounded-xl border border-gray-100 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</p>
          <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ item.value }}</p>
          <p v-if="item.hint" class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ item.hint }}</p>
        </div>
      </div>

      <div class="grid gap-6 xl:grid-cols-[minmax(360px,440px)_1fr]">
        <aside class="space-y-6">
          <section class="card p-5">
            <div class="mb-5 flex items-start justify-between gap-3">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">自动分配规则</h2>
                <p class="mt-1 text-sm leading-6 text-gray-500 dark:text-gray-400">
                  自动任务只处理未绑定代理的账号。临时限流、短期额度保护不会释放已有绑定。
                </p>
              </div>
              <span class="rounded-full px-2.5 py-1 text-xs font-medium" :class="settings.enabled ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'">
                {{ settings.enabled ? '已启用' : '未启用' }}
              </span>
            </div>

            <div class="space-y-4">
              <label class="flex items-start justify-between gap-4 rounded-lg border border-gray-100 p-3 dark:border-dark-700">
                <span>
                  <span class="block text-sm font-medium text-gray-900 dark:text-white">启用自动分配</span>
                  <span class="mt-1 block text-xs leading-5 text-gray-500 dark:text-gray-400">
                    开启后后台按扫描周期自动分配。也可以在页面手动执行。
                  </span>
                </span>
                <input v-model="settings.enabled" type="checkbox" class="mt-1" />
              </label>

              <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <label class="form-field">
                  <span class="form-label">分配策略</span>
                  <select v-model="settings.strategy" class="input">
                    <option value="weighted_least_loaded">按权重最小负载</option>
                    <option value="least_loaded">账号数最少优先</option>
                  </select>
                </label>
                <label class="form-field">
                  <span class="form-label">单代理账号上限</span>
                  <input v-model.number="settings.max_accounts_per_proxy" type="number" min="0" class="input" />
                  <span class="mt-1 text-xs text-gray-500">0 表示不限制。</span>
                </label>
                <label class="form-field">
                  <span class="form-label">每批最多处理</span>
                  <input v-model.number="settings.batch_size" type="number" min="1" max="1000" class="input" />
                </label>
                <label class="form-field">
                  <span class="form-label">自动扫描周期（分钟）</span>
                  <input v-model.number="settings.scan_interval_minutes" type="number" min="1" max="1440" class="input" />
                </label>
                <label class="form-field">
                  <span class="form-label">本次执行数量</span>
                  <input v-model.number="assignLimit" type="number" min="1" max="1000" class="input" />
                </label>
                <label class="form-field">
                  <span class="form-label">保留事件数量</span>
                  <input v-model.number="settings.max_stored_events" type="number" min="20" max="500" class="input" />
                </label>
              </div>

              <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
                <label v-for="item in switchItems" :key="item.key" class="flex items-center gap-2 rounded-lg bg-gray-50 px-3 py-2 text-sm text-gray-700 dark:bg-dark-700/60 dark:text-gray-200">
                  <input v-model="settings[item.key]" type="checkbox" />
                  <span>{{ item.label }}</span>
                </label>
              </div>

              <div>
                <p class="mb-2 text-sm font-medium text-gray-900 dark:text-white">参与平台</p>
                <div class="grid grid-cols-2 gap-2">
                  <label v-for="platform in platformOptions" :key="platform.value" class="flex items-center gap-2 rounded-lg bg-gray-50 px-3 py-2 text-sm dark:bg-dark-700/60">
                    <input type="checkbox" :checked="settings.platforms.includes(platform.value)" @change="togglePlatform(platform.value)" />
                    <span>{{ platform.label }}</span>
                  </label>
                </div>
              </div>

              <div class="rounded-lg border border-cyan-200 bg-cyan-50 p-3 text-xs leading-5 text-cyan-900 dark:border-cyan-900/60 dark:bg-cyan-900/20 dark:text-cyan-200">
                稳定性规则：账号一旦绑定代理，不会因为负载变化自动迁移；代理失效或账号长期不合规时才按开关释放。短期限流、过载、5 小时额度保护不会释放绑定。
              </div>

              <div class="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm dark:border-amber-900/60 dark:bg-amber-900/20">
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <h3 class="font-semibold text-amber-950 dark:text-amber-100">校验前预绑定</h3>
                    <p class="mt-1 text-xs leading-5 text-amber-900 dark:text-amber-200">
                      用户上传账号后先绑定代理，再用该代理检测账号，避免检测阶段走主站 IP。关闭代理亲和模块时此能力不会生效。
                    </p>
                  </div>
                  <input v-model="settings.pre_validation_enabled" type="checkbox" class="mt-1" />
                </div>
                <div class="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
                  <label class="flex items-center gap-2 rounded-lg bg-white/70 px-3 py-2 text-xs text-amber-950 dark:bg-dark-700/60 dark:text-amber-100">
                    <input v-model="settings.enforce_validation_proxy" type="checkbox" />
                    检测必须走绑定代理
                  </label>
                  <label class="flex items-center gap-2 rounded-lg bg-white/70 px-3 py-2 text-xs text-amber-950 dark:bg-dark-700/60 dark:text-amber-100">
                    <input v-model="settings.include_pending_accounts" type="checkbox" />
                    公有待审核账号也预绑定
                  </label>
                  <label class="flex items-center gap-2 rounded-lg bg-white/70 px-3 py-2 text-xs text-amber-950 dark:bg-dark-700/60 dark:text-amber-100">
                    <input v-model="settings.release_on_validation_failure" type="checkbox" />
                    校验明确失败后释放代理
                  </label>
                  <label class="form-field">
                    <span class="form-label text-amber-950 dark:text-amber-100">无可用代理时</span>
                    <select v-model="settings.fallback_when_no_proxy" class="input">
                      <option value="wait">等待代理，不直连</option>
                      <option value="reject">拒绝检测</option>
                      <option value="direct">允许直连检测</option>
                    </select>
                  </label>
                </div>
              </div>

              <div class="flex justify-end gap-2 pt-2">
                <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadAll">取消修改</button>
                <button type="button" class="btn btn-primary" :disabled="saving" @click="saveSettings">
                  {{ saving ? '保存中...' : '保存规则' }}
                </button>
              </div>
            </div>
          </section>

          <section class="card p-5">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">手动绑定/释放</h2>
            <p class="mt-1 text-sm leading-6 text-gray-500 dark:text-gray-400">
              用于少量账号的人工干预。批量处理建议先用“预览分配”。
            </p>
            <div class="mt-4 space-y-3">
              <label class="form-field">
                <span class="form-label">账号 ID</span>
                <input v-model.number="manualAccountId" type="number" min="1" class="input" placeholder="例如 123" />
              </label>
              <label class="form-field">
                <span class="form-label">代理 ID</span>
                <input v-model.number="manualProxyId" type="number" min="1" class="input" placeholder="绑定时填写" />
              </label>
              <label class="form-field">
                <span class="form-label">操作原因</span>
                <input v-model.trim="manualReason" type="text" class="input" placeholder="可选，写入最近事件" />
              </label>
              <label class="inline-flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
                <input v-model="manualDryRun" type="checkbox" />
                只预览，不写入数据库
              </label>
              <div class="flex flex-wrap gap-2">
                <button type="button" class="btn btn-secondary" :disabled="manualWorking" @click="manualBind">绑定到代理</button>
                <button type="button" class="btn btn-secondary" :disabled="manualWorking" @click="manualRelease">释放绑定</button>
              </div>
            </div>
          </section>
        </aside>

        <main class="space-y-6">
          <section class="card overflow-hidden">
            <div class="flex flex-col gap-2 border-b border-gray-100 px-6 py-4 dark:border-dark-700 md:flex-row md:items-center md:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">代理负载与策略</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  暂停分配不会影响已绑定账号，只是不再把新账号分配到该代理。
                </p>
              </div>
              <span class="text-sm text-gray-500 dark:text-gray-400">
                平均 {{ formatNumber(overview?.average_load ?? 0) }} 个账号/代理
              </span>
            </div>
            <div class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
                <thead class="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                  <tr>
                    <th class="px-5 py-3">代理</th>
                    <th class="px-5 py-3">IP/地址</th>
                    <th class="px-5 py-3">绑定账号</th>
                    <th class="px-5 py-3">负载</th>
                    <th class="px-5 py-3">权重</th>
                    <th class="px-5 py-3">状态</th>
                    <th class="px-5 py-3">操作</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
                  <tr v-if="loading">
                    <td colspan="7" class="px-5 py-10 text-center text-gray-500">加载中...</td>
                  </tr>
                  <tr v-else-if="proxyLoads.length === 0">
                    <td colspan="7" class="px-5 py-10 text-center text-gray-500">还没有可用代理，请先在代理管理中添加并启用代理。</td>
                  </tr>
                  <tr v-for="proxy in proxyLoads" v-else :key="proxy.proxy_id" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                    <td class="px-5 py-4">
                      <div class="font-medium text-gray-900 dark:text-white">{{ proxy.name || `代理 #${proxy.proxy_id}` }}</div>
                      <div class="text-xs text-gray-500">ID {{ proxy.proxy_id }} · {{ proxy.protocol }}</div>
                    </td>
                    <td class="px-5 py-4">
                      <code class="rounded bg-gray-100 px-2 py-1 text-xs dark:bg-dark-700">{{ proxy.host }}:{{ proxy.port }}</code>
                      <div v-if="proxy.country || proxy.ip_address" class="mt-1 text-xs text-gray-500">
                        {{ proxy.country || '-' }} {{ proxy.ip_address ? `· ${proxy.ip_address}` : '' }}
                      </div>
                    </td>
                    <td class="px-5 py-4 text-gray-700 dark:text-gray-200">
                      {{ proxy.account_count }}
                      <span v-if="proxy.max_accounts > 0" class="text-gray-400">/ {{ proxy.max_accounts }}</span>
                    </td>
                    <td class="px-5 py-4">
                      <div class="h-2 w-32 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                        <div class="h-full rounded-full" :class="loadBarClass(proxy)" :style="{ width: `${loadPercent(proxy)}%` }"></div>
                      </div>
                      <div class="mt-1 text-xs text-gray-500">
                        {{ proxy.max_accounts > 0 ? `${loadPercent(proxy).toFixed(0)}%` : `有效负载 ${formatNumber(proxy.effective_load)}` }}
                      </div>
                    </td>
                    <td class="px-5 py-4">
                      <input class="input h-9 w-20" type="number" min="1" max="100" :value="proxy.weight || 1" @input="setProxyWeight(proxy.proxy_id, $event)" />
                    </td>
                    <td class="px-5 py-4">
                      <span class="badge" :class="proxy.assignable ? 'badge-success' : 'badge-warning'">
                        {{ proxy.assignable ? '可分配' : '不可分配' }}
                      </span>
                      <div v-if="proxy.reason" class="mt-1 text-xs text-gray-500">{{ proxy.reason }}</div>
                      <div v-if="proxy.quality_grade" class="mt-1 text-xs text-gray-500">质量 {{ proxy.quality_grade }}</div>
                    </td>
                    <td class="px-5 py-4">
                      <button type="button" class="btn btn-secondary h-8 px-3 text-xs" @click="toggleProxyPaused(proxy.proxy_id)">
                        {{ isProxyPaused(proxy.proxy_id) ? '恢复分配' : '暂停分配' }}
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <section class="card overflow-hidden">
            <div class="flex flex-col gap-2 border-b border-gray-100 px-6 py-4 dark:border-dark-700 md:flex-row md:items-center md:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">账号绑定状态</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  查看每个已绑定账号当前固定在哪个代理 IP 上。
                </p>
              </div>
              <span class="text-sm text-gray-500">已绑定 {{ boundAccounts.length }} 个账号</span>
            </div>
            <div class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
                <thead class="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                  <tr>
                    <th class="px-5 py-3">账号</th>
                    <th class="px-5 py-3">类型</th>
                    <th class="px-5 py-3">当前代理</th>
                    <th class="px-5 py-3">绑定信息</th>
                    <th class="px-5 py-3">健康状态</th>
                    <th class="px-5 py-3">操作</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
                  <tr v-if="boundAccounts.length === 0">
                    <td colspan="6" class="px-5 py-10 text-center text-gray-500">暂时没有已绑定代理的账号。</td>
                  </tr>
                  <tr v-for="account in boundAccounts" v-else :key="account.account_id" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                    <td class="px-5 py-4">
                      <div class="font-medium text-gray-900 dark:text-white">{{ account.account_name || `账号 #${account.account_id}` }}</div>
                      <div class="text-xs text-gray-500">ID {{ account.account_id }} · {{ ownerLabel(account.owner_user_id) }}</div>
                    </td>
                    <td class="px-5 py-4 text-gray-700 dark:text-gray-200">
                      {{ platformLabel(account.platform) }} / {{ account.type }}
                      <div class="text-xs text-gray-500">{{ shareLabel(account.share_mode, account.share_status) }} · {{ levelLabel(account.account_level) }}</div>
                    </td>
                    <td class="px-5 py-4 text-gray-700 dark:text-gray-200">
                      <div>{{ account.proxy_name || `代理 #${account.proxy_id}` }}</div>
                      <code v-if="account.proxy_host" class="mt-1 inline-block rounded bg-gray-100 px-2 py-1 text-xs dark:bg-dark-700">{{ account.proxy_host }}:{{ account.proxy_port }}</code>
                    </td>
                    <td class="px-5 py-4 text-xs text-gray-500">
                      <div>来源：{{ sourceLabel(account.assigned_by) }}</div>
                      <div>阶段：{{ phaseLabel(account.phase) }}</div>
                      <div>时间：{{ formatDate(account.assigned_at) }}</div>
                      <div v-if="account.last_test_at">最近校验：{{ formatDate(account.last_test_at) }}</div>
                      <div v-if="account.assign_reason">原因：{{ account.assign_reason }}</div>
                      <div v-if="account.last_test_error" class="text-amber-600 dark:text-amber-300">校验信息：{{ account.last_test_error }}</div>
                    </td>
                    <td class="px-5 py-4">
                      <span class="badge" :class="account.health_status === 'healthy' ? 'badge-success' : 'badge-warning'">
                        {{ healthLabel(account.health_status) }}
                      </span>
                      <div v-if="account.health_reason" class="mt-1 text-xs text-gray-500">{{ account.health_reason }}</div>
                    </td>
                    <td class="px-5 py-4">
                      <button type="button" class="btn btn-secondary h-8 px-3 text-xs" :disabled="manualWorking" @click="releaseFromRow(account.account_id)">
                        释放
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <div class="grid gap-6 xl:grid-cols-2">
            <section class="card overflow-hidden">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">待处理账号</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">包含可分配账号和当前被规则跳过的账号。</p>
              </div>
              <div class="max-h-[460px] overflow-auto">
                <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
                  <thead class="sticky top-0 bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                    <tr>
                      <th class="px-5 py-3">账号</th>
                      <th class="px-5 py-3">状态</th>
                      <th class="px-5 py-3">原因</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
                    <tr v-if="pendingAccounts.length === 0">
                      <td colspan="3" class="px-5 py-10 text-center text-gray-500">没有待处理账号。</td>
                    </tr>
                    <tr v-for="account in pendingAccounts" v-else :key="account.account_id" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                      <td class="px-5 py-4">
                        <div class="font-medium text-gray-900 dark:text-white">{{ account.account_name || `账号 #${account.account_id}` }}</div>
                        <div class="text-xs text-gray-500">ID {{ account.account_id }} · {{ platformLabel(account.platform) }}</div>
                      </td>
                      <td class="px-5 py-4 text-xs text-gray-500">
                        {{ shareLabel(account.share_mode, account.share_status) }} · {{ levelLabel(account.account_level) }}
                        <div v-if="account.phase">阶段：{{ phaseLabel(account.phase) }}</div>
                        <div v-if="account.last_test_at">最近校验：{{ formatDate(account.last_test_at) }}</div>
                      </td>
                      <td class="px-5 py-4 text-gray-600 dark:text-gray-300">
                        <div>{{ account.reason }}</div>
                        <div v-if="account.last_test_error" class="mt-1 text-xs text-amber-600 dark:text-amber-300">{{ account.last_test_error }}</div>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </section>

            <section class="card overflow-hidden">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">最近事件</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">记录自动/手动分配、释放、失败和跳过。</p>
              </div>
              <div class="max-h-[460px] overflow-auto">
                <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
                  <thead class="sticky top-0 bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                    <tr>
                      <th class="px-5 py-3">时间</th>
                      <th class="px-5 py-3">操作</th>
                      <th class="px-5 py-3">账号/代理</th>
                      <th class="px-5 py-3">原因</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
                    <tr v-if="recentEvents.length === 0">
                      <td colspan="4" class="px-5 py-10 text-center text-gray-500">暂无事件。</td>
                    </tr>
                    <tr v-for="event in recentEvents" v-else :key="event.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                      <td class="px-5 py-4 text-xs text-gray-500">{{ formatDate(event.occurred_at) }}</td>
                      <td class="px-5 py-4">
                        <span class="badge" :class="assignmentClass(event.action)">{{ assignmentLabel(event.action, event.dry_run) }}</span>
                        <div class="mt-1 text-xs text-gray-500">{{ sourceLabel(event.source) }}</div>
                      </td>
                      <td class="px-5 py-4 text-gray-700 dark:text-gray-200">
                        <div>{{ event.account_name || (event.account_id ? `账号 #${event.account_id}` : '-') }}</div>
                        <div class="text-xs text-gray-500">{{ event.proxy_name || (event.proxy_id ? `代理 #${event.proxy_id}` : '-') }}</div>
                      </td>
                      <td class="px-5 py-4 text-gray-600 dark:text-gray-300">{{ event.reason || '-' }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </section>
          </div>

          <section class="card overflow-hidden">
            <div class="flex flex-col gap-2 border-b border-gray-100 px-6 py-4 dark:border-dark-700 md:flex-row md:items-center md:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">本次执行结果</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">预览不会写入数据库；执行分配会写入账号代理绑定。</p>
              </div>
              <span v-if="lastResult" class="text-sm text-gray-500">
                扫描 {{ lastResult.scanned }}，分配 {{ lastResult.assigned }}，释放 {{ lastResult.released || 0 }}，跳过 {{ lastResult.skipped }}
              </span>
            </div>
            <div class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
                <thead class="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                  <tr>
                    <th class="px-5 py-3">账号</th>
                    <th class="px-5 py-3">类型</th>
                    <th class="px-5 py-3">目标代理</th>
                    <th class="px-5 py-3">结果</th>
                    <th class="px-5 py-3">原因</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
                  <tr v-if="!lastResult">
                    <td colspan="5" class="px-5 py-10 text-center text-gray-500">还没有执行过分配。可以先点击“预览分配”。</td>
                  </tr>
                  <tr v-else-if="lastResult.assignments.length === 0">
                    <td colspan="5" class="px-5 py-10 text-center text-gray-500">没有需要处理的账号。</td>
                  </tr>
                  <tr v-for="(row, index) in lastResult.assignments" v-else :key="`${row.candidate.account_id}-${row.action}-${row.proxy_id || 0}-${index}`" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                    <td class="px-5 py-4">
                      <div class="font-medium text-gray-900 dark:text-white">{{ row.candidate.account_name || `账号 #${row.candidate.account_id}` }}</div>
                      <div class="text-xs text-gray-500">ID {{ row.candidate.account_id }} · {{ ownerLabel(row.candidate.owner_user_id) }}</div>
                    </td>
                    <td class="px-5 py-4 text-gray-700 dark:text-gray-200">
                      {{ platformLabel(row.candidate.platform) }} / {{ row.candidate.type }}
                      <div class="text-xs text-gray-500">{{ shareLabel(row.candidate.share_mode, row.candidate.share_status) }} · {{ levelLabel(row.candidate.account_level) }}</div>
                    </td>
                    <td class="px-5 py-4 text-gray-700 dark:text-gray-200">
                      <span v-if="row.proxy_id">{{ row.proxy_name || `代理 #${row.proxy_id}` }}</span>
                      <span v-else class="text-gray-400">-</span>
                    </td>
                    <td class="px-5 py-4">
                      <span class="badge" :class="assignmentClass(row.action)">
                        {{ assignmentLabel(row.action, row.dry_run) }}
                      </span>
                    </td>
                    <td class="px-5 py-4 text-gray-600 dark:text-gray-300">{{ row.reason || '-' }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>
        </main>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type {
  ProxyAffinityAccountBinding,
  ProxyAffinityAssignResult,
  ProxyAffinityEvent,
  ProxyAffinityOverview,
  ProxyAffinityPendingAccount,
  ProxyAffinityProxyLoad,
  ProxyAffinitySettings
} from '@/api/admin/proxyAffinity'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'

const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
const assigning = ref(false)
const manualWorking = ref(false)
const overview = ref<ProxyAffinityOverview | null>(null)
const lastResult = ref<ProxyAffinityAssignResult | null>(null)
const assignLimit = ref(100)
const manualAccountId = ref<number | null>(null)
const manualProxyId = ref<number | null>(null)
const manualReason = ref('')
const manualDryRun = ref(false)

const settings = reactive<ProxyAffinitySettings>({
  enabled: false,
  user_owned_enabled: true,
  admin_accounts_enabled: true,
  private_accounts_enabled: true,
  public_accounts_enabled: true,
  only_approved_public_accounts: true,
  include_api_key_accounts: true,
  include_oauth_accounts: true,
  max_accounts_per_proxy: 0,
  batch_size: 100,
  scan_interval_minutes: 5,
  platforms: ['openai', 'anthropic', 'gemini', 'antigravity'],
  allow_reassign_when_proxy_down: false,
  release_when_account_inactive: false,
  strategy: 'weighted_least_loaded',
  max_stored_events: 200,
  paused_proxy_ids: [],
  proxy_weights: {},
  pre_validation_enabled: true,
  enforce_validation_proxy: true,
  include_pending_accounts: true,
  release_on_validation_failure: true,
  retry_with_new_proxy_on_failure: false,
  max_pre_validation_retries: 1,
  fallback_when_no_proxy: 'wait'
})

const switchItems: Array<{ key: keyof ProxyAffinitySettings; label: string }> = [
  { key: 'user_owned_enabled', label: '用户上传账号参与' },
  { key: 'admin_accounts_enabled', label: '管理员账号参与' },
  { key: 'private_accounts_enabled', label: '私有账号参与' },
  { key: 'public_accounts_enabled', label: '公有账号参与' },
  { key: 'only_approved_public_accounts', label: '仅审核通过公有账号' },
  { key: 'include_oauth_accounts', label: 'OAuth 账号参与' },
  { key: 'include_api_key_accounts', label: 'API Key 账号参与' },
  { key: 'allow_reassign_when_proxy_down', label: '代理不可用时释放重分配' },
  { key: 'release_when_account_inactive', label: '账号长期异常时释放绑定' }
]

const platformOptions = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' }
]

const statCards = computed(() => [
  { label: '代理总数', value: overview.value?.total_proxies ?? 0, hint: `可分配 ${overview.value?.available_proxies ?? 0}` },
  { label: '已绑定账号', value: overview.value?.bound_accounts ?? 0, hint: '已有固定代理的账号' },
  { label: '校验前绑定', value: overview.value?.pre_validation_accounts ?? 0, hint: '已绑定代理，等待检测结果' },
  { label: '等待代理', value: overview.value?.waiting_proxy_accounts ?? 0, hint: '没有可用代理时暂挂' },
  { label: '校验失败', value: overview.value?.validation_failed_accounts ?? 0, hint: '需要处理账号或代理' },
  { label: '上次运行', value: formatShortDate(overview.value?.last_run_at), hint: '自动任务最近执行时间' }
])

const proxyLoads = computed<ProxyAffinityProxyLoad[]>(() => overview.value?.proxy_loads ?? [])
const boundAccounts = computed<ProxyAffinityAccountBinding[]>(() => overview.value?.bound_account_details ?? [])
const pendingAccounts = computed<ProxyAffinityPendingAccount[]>(() => overview.value?.pending_accounts ?? [])
const recentEvents = computed<ProxyAffinityEvent[]>(() => overview.value?.recent_events ?? [])

function applySettings(next: ProxyAffinitySettings): void {
  Object.assign(settings, {
    ...next,
    platforms: Array.isArray(next.platforms) ? [...next.platforms] : [],
    paused_proxy_ids: Array.isArray(next.paused_proxy_ids) ? [...next.paused_proxy_ids] : [],
    proxy_weights: { ...(next.proxy_weights || {}) },
    strategy: next.strategy || 'weighted_least_loaded',
    max_stored_events: next.max_stored_events || 200,
    pre_validation_enabled: next.pre_validation_enabled ?? true,
    enforce_validation_proxy: next.enforce_validation_proxy ?? true,
    include_pending_accounts: next.include_pending_accounts ?? true,
    release_on_validation_failure: next.release_on_validation_failure ?? true,
    retry_with_new_proxy_on_failure: next.retry_with_new_proxy_on_failure ?? false,
    max_pre_validation_retries: next.max_pre_validation_retries ?? 1,
    fallback_when_no_proxy: next.fallback_when_no_proxy || 'wait'
  })
  assignLimit.value = next.batch_size || 100
}

async function loadAll(): Promise<void> {
  loading.value = true
  try {
    const data = await adminAPI.proxyAffinity.getOverview()
    overview.value = data
    applySettings(data.settings)
  } catch (error: any) {
    appStore.showError(error?.message || '加载代理亲和调度失败')
  } finally {
    loading.value = false
  }
}

async function saveSettings(): Promise<void> {
  saving.value = true
  try {
    const saved = await adminAPI.proxyAffinity.updateSettings({
      ...settings,
      platforms: [...settings.platforms],
      paused_proxy_ids: [...settings.paused_proxy_ids],
      proxy_weights: { ...settings.proxy_weights }
    })
    applySettings(saved)
    appStore.showSuccess('代理亲和调度规则已保存')
    await loadAll()
  } catch (error: any) {
    appStore.showError(error?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function runAssign(dryRun: boolean): Promise<void> {
  assigning.value = true
  try {
    lastResult.value = await adminAPI.proxyAffinity.assign({
      dry_run: dryRun,
      limit: assignLimit.value || settings.batch_size,
      platforms: [...settings.platforms]
    })
    appStore.showSuccess(dryRun ? '预览完成，未写入数据库' : `执行完成：分配 ${lastResult.value.assigned} 个，释放 ${lastResult.value.released || 0} 个`)
    if (!dryRun) {
      await loadAll()
    }
  } catch (error: any) {
    appStore.showError(error?.message || '分配失败')
  } finally {
    assigning.value = false
  }
}

async function runPrebind(dryRun: boolean): Promise<void> {
  assigning.value = true
  try {
    lastResult.value = await adminAPI.proxyAffinity.prebind({
      dry_run: dryRun,
      limit: assignLimit.value || settings.batch_size,
      platforms: [...settings.platforms]
    })
    appStore.showSuccess(dryRun ? '预绑定预览完成，未写入数据库' : `预绑定完成：绑定 ${lastResult.value.assigned} 个，跳过 ${lastResult.value.skipped} 个`)
    if (!dryRun) {
      await loadAll()
    }
  } catch (error: any) {
    appStore.showError(error?.message || '预绑定失败')
  } finally {
    assigning.value = false
  }
}

async function manualBind(): Promise<void> {
  if (!manualAccountId.value || !manualProxyId.value) {
    appStore.showError('请填写账号 ID 和代理 ID')
    return
  }
  manualWorking.value = true
  try {
    const result = await adminAPI.proxyAffinity.bindAccount({
      account_id: manualAccountId.value,
      proxy_id: manualProxyId.value,
      dry_run: manualDryRun.value,
      reason: manualReason.value
    })
    lastResult.value = singleAssignmentResult(result)
    appStore.showSuccess(manualDryRun.value ? '手动绑定预览完成' : '手动绑定已执行')
    if (!manualDryRun.value) await loadAll()
  } catch (error: any) {
    appStore.showError(error?.message || '手动绑定失败')
  } finally {
    manualWorking.value = false
  }
}

async function manualRelease(): Promise<void> {
  if (!manualAccountId.value) {
    appStore.showError('请填写账号 ID')
    return
  }
  manualWorking.value = true
  try {
    const result = await adminAPI.proxyAffinity.releaseAccount({
      account_id: manualAccountId.value,
      dry_run: manualDryRun.value,
      reason: manualReason.value
    })
    lastResult.value = singleAssignmentResult(result)
    appStore.showSuccess(manualDryRun.value ? '释放预览完成' : '代理绑定已释放')
    if (!manualDryRun.value) await loadAll()
  } catch (error: any) {
    appStore.showError(error?.message || '释放失败')
  } finally {
    manualWorking.value = false
  }
}

async function releaseFromRow(accountId: number): Promise<void> {
  manualAccountId.value = accountId
  manualDryRun.value = false
  manualReason.value = '从账号绑定表手动释放'
  await manualRelease()
}

function singleAssignmentResult(assignment: ProxyAffinityAssignResult['assignments'][number]): ProxyAffinityAssignResult {
  return {
    dry_run: assignment.dry_run,
    scanned: 1,
    assigned: assignment.action === 'assigned' ? 1 : 0,
    released: assignment.action === 'released' ? 1 : 0,
    skipped: assignment.action === 'skipped' ? 1 : 0,
    assignments: [assignment]
  }
}

function togglePlatform(platform: string): void {
  const set = new Set(settings.platforms)
  if (set.has(platform)) set.delete(platform)
  else set.add(platform)
  settings.platforms = Array.from(set)
}

function isProxyPaused(proxyId: number): boolean {
  return settings.paused_proxy_ids.includes(proxyId)
}

function toggleProxyPaused(proxyId: number): void {
  const set = new Set(settings.paused_proxy_ids)
  if (set.has(proxyId)) set.delete(proxyId)
  else set.add(proxyId)
  settings.paused_proxy_ids = Array.from(set)
}

function setProxyWeight(proxyId: number, event: Event): void {
  const value = Number((event.target as HTMLInputElement).value || 1)
  const next = { ...settings.proxy_weights }
  next[proxyId] = Math.min(100, Math.max(1, Math.floor(value || 1)))
  settings.proxy_weights = next
}

function loadPercent(proxy: ProxyAffinityProxyLoad): number {
  if (proxy.max_accounts <= 0) return Math.min(100, proxy.account_count * 5)
  return Math.min(100, Math.max(0, proxy.load_percent || (proxy.account_count * 100 / proxy.max_accounts)))
}

function loadBarClass(proxy: ProxyAffinityProxyLoad): string {
  const value = loadPercent(proxy)
  if (!proxy.assignable || value >= 95) return 'bg-red-500'
  if (value >= 75) return 'bg-amber-500'
  return 'bg-emerald-500'
}

function formatNumber(value: number): string {
  return Number(value || 0).toLocaleString(undefined, { maximumFractionDigits: 1 })
}

function formatDate(value?: string): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function formatShortDate(value?: string): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString(undefined, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function platformLabel(platform: string): string {
  return platformOptions.find((item) => item.value === platform)?.label || platform || '-'
}

function levelLabel(level: string): string {
  if (!level || level === 'unknown') return '未知等级'
  return level
}

function ownerLabel(ownerUserID?: number): string {
  return ownerUserID ? `用户 ${ownerUserID}` : '管理员号'
}

function shareLabel(mode: string, status: string): string {
  if (mode === 'public') return status === 'approved' ? '公有已审核' : `公有${status || '待审核'}`
  return '私有'
}

function sourceLabel(source?: string): string {
  if (source === 'manual') return '手动'
  if (source === 'auto') return '自动'
  if (source === 'pre_validation') return '校验前预绑定'
  if (source === 'proxy_affinity') return '代理亲和'
  return source || '-'
}

function phaseLabel(phase?: string): string {
  const map: Record<string, string> = {
    pre_validation: '校验前已绑定',
    validated: '已校验',
    validation_failed: '校验失败',
    waiting_proxy: '等待可用代理'
  }
  return map[phase || ''] || phase || '-'
}

function healthLabel(status: string): string {
  const map: Record<string, string> = {
    healthy: '正常',
    proxy_down: '代理异常',
    proxy_paused: '代理暂停',
    proxy_missing: '代理缺失',
    account_ineligible: '账号不合规'
  }
  return map[status] || status || '-'
}

function assignmentClass(action: string): string {
  if (action === 'assigned') return 'badge-success'
  if (action === 'released') return 'badge-primary'
  if (action === 'failed') return 'badge-danger'
  return 'badge-warning'
}

function assignmentLabel(action: string, dryRun: boolean): string {
  if (action === 'assigned') return dryRun ? '将分配' : '已分配'
  if (action === 'released') return dryRun ? '将释放' : '已释放'
  if (action === 'failed') return '失败'
  return '跳过'
}

onMounted(loadAll)
</script>
