<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, ApiError } from '@/api/client'
import MailPane from '@/components/MailPane.vue'

interface SyncStatus {
  running: boolean
  last_started_at: number
  last_finished_at: number
  last_error: string
  listed: number
  fetched: number
  failed: number
}

const status = ref<SyncStatus | null>(null)
const error = ref('')
const paneKey = ref(0) // 同步结束后递增，重新挂载邮件列表
let timer: number | undefined

const summary = computed(() => {
  const s = status.value
  if (!s) return ''
  if (s.running) return '同步中…'
  if (!s.last_finished_at) return '尚未同步'
  const when = new Date(s.last_finished_at * 1000).toLocaleString()
  return `上次同步 ${when}：共 ${s.listed} 封，新增 ${s.fetched} 封${s.failed ? `，失败 ${s.failed} 封` : ''}`
})

async function refreshStatus() {
  try {
    const wasRunning = status.value?.running ?? false
    status.value = await api<SyncStatus>('/admin/sync')
    if (wasRunning && !status.value.running) paneKey.value++
    if (!status.value.running) stopPolling()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '获取同步状态失败'
    stopPolling()
  }
}

function startPolling() {
  if (timer === undefined) timer = window.setInterval(refreshStatus, 2000)
}

function stopPolling() {
  window.clearInterval(timer)
  timer = undefined
}

async function sync() {
  error.value = ''
  try {
    status.value = await api<SyncStatus>('/admin/sync', { method: 'POST' })
  } catch (e) {
    // 409 表示已有同步在进行，直接跟踪它的进度即可
    if (!(e instanceof ApiError && e.status === 409)) {
      error.value = e instanceof ApiError ? e.message : '启动同步失败'
      return
    }
    status.value = await api<SyncStatus>('/admin/sync')
  }
  startPolling()
}

onMounted(async () => {
  await refreshStatus()
  if (status.value?.running) startPolling()
})
onBeforeUnmount(stopPolling)
</script>

<template>
  <div class="flex min-h-[32rem] flex-col lg:h-full lg:min-h-0">
    <div class="mb-4 flex shrink-0 flex-wrap items-center gap-3">
      <h2 class="text-xl font-semibold">未匹配邮件</h2>
      <button
        class="rounded bg-slate-900 px-3 py-1.5 text-sm text-white hover:bg-slate-700 disabled:opacity-50"
        :disabled="status?.running"
        @click="sync"
      >
        {{ status?.running ? '同步中…' : '立即同步' }}
      </button>
      <span class="text-sm text-slate-500">{{ summary }}</span>
    </div>
    <p class="mb-3 shrink-0 text-sm text-slate-500">收件地址没有对应账号的邮件。创建同地址账号后，邮件会自动归入该账号的收件箱。</p>
    <p v-if="status?.last_error && !status.running" class="mb-3 shrink-0 text-sm text-red-600" role="alert">
      上次同步出错：{{ status.last_error }}
    </p>
    <p v-if="error" class="mb-3 shrink-0 text-sm text-red-600" role="alert">{{ error }}</p>

    <MailPane class="lg:min-h-0 lg:flex-1" :key="paneKey" base="/admin/mails/unrouted" empty-text="没有未匹配的邮件" />
  </div>
</template>
