<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, ApiError } from '@/api/client'

interface Account {
  id: number
  address: string
  enabled: boolean
  note: string
  created_at: number
  last_login_at: number | null
}

type PendingKind = 'reset' | 'delete'

const accounts = ref<Account[]>([])
const loading = ref(true)
const error = ref('')

const newAddress = ref('')
const newNote = ref('')
const busy = ref(false)

const pending = ref<{ id: number; kind: PendingKind } | null>(null)
const secret = ref<{ address: string; password: string } | null>(null)
const copied = ref(false)

function fail(e: unknown) {
  error.value = e instanceof ApiError ? e.message : '操作失败，请稍后重试'
}

function fmt(ts: number | null) {
  return ts ? new Date(ts * 1000).toLocaleString() : '—'
}

async function load() {
  try {
    accounts.value = (await api<{ accounts: Account[] }>('/admin/accounts')).accounts
  } catch (e) {
    fail(e)
  } finally {
    loading.value = false
  }
}

async function create() {
  if (busy.value) return
  error.value = ''
  busy.value = true
  try {
    const res = await api<{ account: Account; password: string }>('/admin/accounts', {
      method: 'POST',
      body: JSON.stringify({ address: newAddress.value, note: newNote.value }),
    })
    secret.value = { address: res.account.address, password: res.password }
    newAddress.value = ''
    newNote.value = ''
    await load()
  } catch (e) {
    fail(e)
  } finally {
    busy.value = false
  }
}

async function confirmPending(a: Account) {
  const p = pending.value
  if (!p || busy.value) return
  // 同一时刻只允许一个写操作在途，且立即收起确认行，避免重复提交产生多个互相作废的密码
  busy.value = true
  pending.value = null
  error.value = ''
  try {
    if (p.kind === 'reset') {
      const res = await api<{ password: string }>(`/admin/accounts/${a.id}/reset-password`, { method: 'POST' })
      secret.value = { address: a.address, password: res.password }
    } else {
      await api(`/admin/accounts/${a.id}`, { method: 'DELETE' })
    }
    await load()
  } catch (e) {
    fail(e)
  } finally {
    busy.value = false
  }
}

async function toggle(a: Account) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await api(`/admin/accounts/${a.id}`, { method: 'PATCH', body: JSON.stringify({ enabled: !a.enabled }) })
    await load()
  } catch (e) {
    fail(e)
  } finally {
    busy.value = false
  }
}

async function copy() {
  if (!secret.value) return
  try {
    await navigator.clipboard.writeText(secret.value.password)
    copied.value = true
  } catch {
    copied.value = false
  }
}

function closeSecret() {
  secret.value = null
  copied.value = false
}

onMounted(load)
</script>

<template>
  <h2 class="mb-4 text-xl font-semibold">账号管理</h2>

  <form class="mb-6 flex flex-wrap items-end gap-3 rounded-lg border border-slate-200 bg-white p-4" @submit.prevent="create">
    <div class="min-w-64 flex-1">
      <label class="mb-1 block text-sm text-slate-600" for="new-address">邮箱地址</label>
      <input
        id="new-address"
        v-model="newAddress"
        type="email"
        required
        placeholder="name@your-domain.com"
        class="w-full rounded border border-slate-300 px-3 py-2 text-sm"
      />
    </div>
    <div class="min-w-48 flex-1">
      <label class="mb-1 block text-sm text-slate-600" for="new-note">备注（可选）</label>
      <input id="new-note" v-model="newNote" maxlength="200" class="w-full rounded border border-slate-300 px-3 py-2 text-sm" />
    </div>
    <button
      type="submit"
      :disabled="busy"
      class="rounded bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-700 disabled:opacity-50"
    >
      创建并生成密码
    </button>
  </form>

  <p v-if="error" class="mb-3 text-sm text-red-600" role="alert">{{ error }}</p>

  <div class="overflow-x-auto rounded-lg border border-slate-200 bg-white">
    <table class="w-full text-left text-sm">
      <thead class="border-b border-slate-200 bg-slate-50 text-slate-600">
        <tr>
          <th class="px-4 py-2 font-medium">邮箱地址</th>
          <th class="px-4 py-2 font-medium">备注</th>
          <th class="px-4 py-2 font-medium">状态</th>
          <th class="px-4 py-2 font-medium">最近登录</th>
          <th class="px-4 py-2 font-medium">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="loading">
          <td colspan="5" class="px-4 py-6 text-center text-slate-500">加载中…</td>
        </tr>
        <tr v-else-if="!accounts.length">
          <td colspan="5" class="px-4 py-6 text-center text-slate-500">还没有账号</td>
        </tr>
        <tr v-for="a in accounts" :key="a.id" class="border-b border-slate-100 last:border-0">
          <td class="px-4 py-2 font-medium">{{ a.address }}</td>
          <td class="px-4 py-2 text-slate-600">{{ a.note || '—' }}</td>
          <td class="px-4 py-2">
            <span
              class="rounded px-2 py-0.5 text-xs"
              :class="a.enabled ? 'bg-green-100 text-green-800' : 'bg-slate-200 text-slate-700'"
            >
              {{ a.enabled ? '启用' : '已禁用' }}
            </span>
          </td>
          <td class="px-4 py-2 text-slate-600">{{ fmt(a.last_login_at) }}</td>
          <td class="px-4 py-2">
            <div v-if="pending && pending.id === a.id" class="flex items-center gap-2">
              <span class="text-slate-600">
                {{ pending.kind === 'reset' ? '重置后旧密码和登录状态立即失效，确认？' : '删除后无法恢复，确认？' }}
              </span>
              <button class="rounded bg-red-600 px-2 py-1 text-white hover:bg-red-700 disabled:opacity-50" :disabled="busy" @click="confirmPending(a)">确认</button>
              <button class="rounded px-2 py-1 hover:bg-slate-100" @click="pending = null">取消</button>
            </div>
            <div v-else class="flex gap-1">
              <button class="rounded px-2 py-1 hover:bg-slate-100 disabled:opacity-50" :disabled="busy" @click="pending = { id: a.id, kind: 'reset' }">重置密码</button>
              <button class="rounded px-2 py-1 hover:bg-slate-100 disabled:opacity-50" :disabled="busy" @click="toggle(a)">{{ a.enabled ? '禁用' : '启用' }}</button>
              <button class="rounded px-2 py-1 text-red-600 hover:bg-red-50 disabled:opacity-50" :disabled="busy" @click="pending = { id: a.id, kind: 'delete' }">删除</button>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>

  <div v-if="secret" class="fixed inset-0 z-10 flex items-center justify-center bg-black/40 p-4" role="dialog" aria-modal="true">
    <div class="w-full max-w-md rounded-lg bg-white p-6 shadow-lg">
      <h3 class="mb-1 text-lg font-semibold">密码已生成</h3>
      <p class="mb-4 text-sm text-red-600">关闭后无法再次查看，请立即复制并通过安全渠道交给使用者。</p>
      <p class="mb-1 text-sm text-slate-600">{{ secret.address }}</p>
      <code class="mb-4 block select-all rounded bg-slate-100 px-3 py-2 text-lg tracking-wider">{{ secret.password }}</code>
      <div class="flex justify-end gap-2">
        <button class="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50" @click="copy">
          {{ copied ? '已复制' : '复制密码' }}
        </button>
        <button class="rounded bg-slate-900 px-3 py-1.5 text-sm text-white hover:bg-slate-700" @click="closeSecret">我已保存</button>
      </div>
    </div>
  </div>
</template>
