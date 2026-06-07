<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">代理亲和调度</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            自动把未绑定代理的账号分配到负载最低的代理，并保持账号与 IP 的稳定绑定。
          </p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="loading" @click="loadAll">
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            刷新
          </button>
          <button type="button" class="btn btn-secondary" :disabled="assigning" @click="runAssign(true)">
            预览分配
          </button>
          <button type="button" class="btn btn-primary" :disabled="assigning" @click="runAssign(false)">
            执行分配
          </button>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-5">
        <div v-for="item in statCards" :key="item.label" class="rounded-xl border border-gray-100 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</p>
          <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ item.value }}</p>
          <p v-if="item.hint" class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ item.hint }}</p>
        </div>
      </div>

      <div class="grid gap-6 xl:grid-cols-[minmax(0,420px)_1fr]">
        <div class="card p-6">
          <div class="mb-5">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">调度规则</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              默认只处理未绑定代理的账号。已经绑定的账号不会因为负载变化自动迁移。
            </p>
          </div>

          <div class="space-y-4">
            <label class="flex items-start justify-between gap-4 rounded-lg border border-gray-100 p-3 dark:border-dark-700">
              <span>
                <span class="block text-sm font-medium text-gray-900 dark:text-white">启用自动分配</span>
                <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">
                  开启后后台按扫描周期自动处理，面板也可以立即执行。
                </span>
              </span>
              <input v-model="settings.enabled" type="checkbox" class="mt-1" />
            </label>

            <div class="grid grid-cols-1 gap-3">
              <label class="flex items-start justify-between gap-4 rounded-lg border border-gray-100 p-3 dark:border-dark-700">
                <span>
                  <span class="block text-sm font-medium text-gray-900 dark:text-white">代理不可用时释放重分配</span>
                  <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">
                    绑定的代理被删除或不再 active 时，自动清空账号代理绑定，下次分配到可用代理。
                  </span>
                </span>
                <input v-model="settings.allow_reassign_when_proxy_down" type="checkbox" class="mt-1" />
              </label>
              <label class="flex items-start justify-between gap-4 rounded-lg border border-gray-100 p-3 dark:border-dark-700">
                <span>
                  <span class="block text-sm font-medium text-gray-900 dark:text-white">异常账号释放绑定</span>
                  <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">
                    账号停用、手动不可调度、公有账号未审核或不再符合规则时释放代理绑定。临时限流或 5 小时额度用完不会释放，避免账号 IP 乱跳。
                  </span>
                </span>
                <input v-model="settings.release_when_account_inactive" type="checkbox" class="mt-1" />
              </label>
            </div>

            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <label v-for="item in switchItems" :key="item.key" class="flex items-center gap-2 rounded-lg bg-gray-50 px-3 py-2 text-sm text-gray-700 dark:bg-dark-700/60 dark:text-gray-200">
                <input v-model="settings[item.key]" type="checkbox" />
                <span>{{ item.label }}</span>
              </label>
            </div>

            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <label class="form-field">
                <span class="form-label">单个代理最大账号数</span>
                <input v-model.number="settings.max_accounts_per_proxy" type="number" min="0" class="input" />
                <span class="mt-1 text-xs text-gray-500">0 表示不限制。</span>
              </label>
              <label class="form-field">
                <span class="form-label">每批最多分配账号</span>
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

            <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs leading-5 text-amber-900 dark:border-amber-900/60 dark:bg-amber-900/20 dark:text-amber-200">
              规则说明：新增账号、公有/私有切换后，如果账号通过校验且未绑定代理，会在下一次扫描或手动执行时分配代理。系统不会因为负载变化移动已绑定账号；只有代理不可用或账号不再符合规则时，才会按上面的开关释放绑定。
            </div>

            <div class="flex justify-end gap-2 pt-2">
              <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadAll">取消修改</button>
              <button type="button" class="btn btn-primary" :disabled="saving" @click="saveSettings">
                {{ saving ? '保存中...' : '保存规则' }}
              </button>
            </div>
          </div>
        </div>

        <div class="space-y-6">
          <div class="card overflow-hidden">
            <div class="flex flex-col gap-2 border-b border-gray-100 px-6 py-4 dark:border-dark-700 md:flex-row md:items-center md:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">代理负载</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  按当前绑定账号数排序。可分配代表未达到上限且代理处于 active 状态。
                </p>
              </div>
              <span class="text-sm text-gray-500 dark:text-gray-400">平均负载 {{ formatNumber(overview?.average_load ?? 0) }} 个账号/代理</span>
            </div>
            <div class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-700">
                <thead class="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                  <tr>
                    <th class="px-5 py-3">代理</th>
                    <th class="px-5 py-3">地址</th>
                    <th class="px-5 py-3">绑定账号</th>
                    <th class="px-5 py-3">负载</th>
                    <th class="px-5 py-3">状态</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
                  <tr v-if="loading">
                    <td colspan="5" class="px-5 py-10 text-center text-gray-500">加载中...</td>
                  </tr>
                  <tr v-else-if="proxyLoads.length === 0">
                    <td colspan="5" class="px-5 py-10 text-center text-gray-500">还没有可用代理。请先在代理管理中添加并启用代理。</td>
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
                      <div class="mt-1 text-xs text-gray-500">{{ proxy.max_accounts > 0 ? `${loadPercent(proxy).toFixed(0)}%` : '未设置上限' }}</div>
                    </td>
                    <td class="px-5 py-4">
                      <span class="badge" :class="proxy.assignable ? 'badge-success' : 'badge-warning'">
                        {{ proxy.assignable ? '可分配' : '不可分配' }}
                      </span>
                      <div v-if="proxy.quality_grade" class="mt-1 text-xs text-gray-500">质量 {{ proxy.quality_grade }}</div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="card overflow-hidden">
            <div class="flex flex-col gap-2 border-b border-gray-100 px-6 py-4 dark:border-dark-700 md:flex-row md:items-center md:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">最近分配结果</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  预览不会写入数据库；执行分配会写入账号的代理绑定。
                </p>
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
                    <td colspan="5" class="px-5 py-10 text-center text-gray-500">没有需要分配的账号。</td>
                  </tr>
                  <tr v-for="row in lastResult.assignments" v-else :key="`${row.candidate.account_id}-${row.action}-${row.proxy_id || 0}`" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                    <td class="px-5 py-4">
                      <div class="font-medium text-gray-900 dark:text-white">{{ row.candidate.account_name || `账号 #${row.candidate.account_id}` }}</div>
                      <div class="text-xs text-gray-500">ID {{ row.candidate.account_id }} · {{ ownerLabel(row.candidate.owner_user_id) }}</div>
                    </td>
                    <td class="px-5 py-4 text-gray-700 dark:text-gray-200">
                      {{ platformLabel(row.candidate.platform) }} / {{ row.candidate.type }}
                      <div class="text-xs text-gray-500">{{ shareLabel(row.candidate.share_mode, row.candidate.share_status) }} · {{ row.candidate.account_level }}</div>
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
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type {
  ProxyAffinityAssignResult,
  ProxyAffinityOverview,
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
const overview = ref<ProxyAffinityOverview | null>(null)
const lastResult = ref<ProxyAffinityAssignResult | null>(null)
const assignLimit = ref(100)

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
  release_when_account_inactive: false
})

const switchItems: Array<{ key: keyof ProxyAffinitySettings; label: string }> = [
  { key: 'user_owned_enabled', label: '用户上传账号参与' },
  { key: 'admin_accounts_enabled', label: '管理员账号参与' },
  { key: 'private_accounts_enabled', label: '私有账号参与' },
  { key: 'public_accounts_enabled', label: '公有账号参与' },
  { key: 'only_approved_public_accounts', label: '仅审核通过公有账号' },
  { key: 'include_oauth_accounts', label: 'OAuth 账号参与' },
  { key: 'include_api_key_accounts', label: 'API Key 账号参与' }
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
  { label: '待分配账号', value: overview.value?.unassigned_eligible_accounts ?? 0, hint: '符合规则但未绑定代理' },
  { label: '已满代理', value: overview.value?.full_proxies ?? 0, hint: '达到单代理账号上限' },
  { label: '平均负载', value: formatNumber(overview.value?.average_load ?? 0), hint: '账号/代理' }
])

const proxyLoads = computed<ProxyAffinityProxyLoad[]>(() => overview.value?.proxy_loads ?? [])

function applySettings(next: ProxyAffinitySettings): void {
  Object.assign(settings, {
    ...next,
    platforms: Array.isArray(next.platforms) ? [...next.platforms] : []
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
    const saved = await adminAPI.proxyAffinity.updateSettings({ ...settings, platforms: [...settings.platforms] })
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
    appStore.showSuccess(dryRun ? '预览完成，没有写入数据库' : `执行完成，绑定 ${lastResult.value.assigned} 个账号，释放 ${lastResult.value.released || 0} 个绑定`)
    if (!dryRun) {
      await loadAll()
    }
  } catch (error: any) {
    appStore.showError(error?.message || '分配失败')
  } finally {
    assigning.value = false
  }
}

function togglePlatform(platform: string): void {
  const set = new Set(settings.platforms)
  if (set.has(platform)) {
    set.delete(platform)
  } else {
    set.add(platform)
  }
  settings.platforms = Array.from(set)
}

function loadPercent(proxy: ProxyAffinityProxyLoad): number {
  if (proxy.max_accounts <= 0) {
    return Math.min(100, proxy.account_count * 5)
  }
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

function platformLabel(platform: string): string {
  return platformOptions.find((item) => item.value === platform)?.label || platform || '-'
}

function ownerLabel(ownerUserID?: number): string {
  return ownerUserID ? `用户 ${ownerUserID}` : '管理员号'
}

function shareLabel(mode: string, status: string): string {
  if (mode === 'public') {
    return status === 'approved' ? '公有已审核' : `公有${status || '待审核'}`
  }
  return '私有'
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
