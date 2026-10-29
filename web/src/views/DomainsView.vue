<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, ApiError } from '@/api/client'

interface Domain {
  name: string
  created_at: number
  accounts: number
}

const domains = ref<Domain[]>([])
const loading = ref(true)
const error = ref('')

const newName = ref('')
const busy = ref(false)
const pending = ref<string | null>(null)

function fail(e: unknown) {
  error.value = e instanceof ApiError ? e.message : '操作失败，请稍后重试'
}

function fmt(ts: number) {
  return new Date(ts * 1000).toLocaleString()
}

async function load() {
  try {
    domains.value = (await api<{ domains: Domain[] }>('/admin/domains')).domains
  } catch (e) {
    fail(e)
  } finally {
    loading.value = false
  }
}

async function add() {
  if (busy.value) return
  error.value = ''
  busy.value = true
  try {
    await api('/admin/domains', { method: 'POST', body: JSON.stringify({ name: newName.value }) })
    newName.value = ''
    await load()
  } catch (e) {
    fail(e)
  } finally {
    busy.value = false
  }
}

async function remove(d: Domain) {
  if (busy.value) return
  busy.value = true
  pending.value = null
  error.value = ''
  try {
    await api(`/admin/domains/${encodeURIComponent(d.name)}`, { method: 'DELETE' })
    await load()
  } catch (e) {
    fail(e)
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <h2 class="mb-1 text-xl font-semibold">域名管理</h2>
  <p class="mb-4 text-sm text-slate-600">
    只能为这里列出的域名创建邮箱账号；列表为空时不限制域名。添加前请先在 Resend 完成该域名的验证并开启收信（MX 记录）。
  </p>

  <form class="mb-6 flex flex-wrap items-end gap-3 rounded-lg border border-slate-200 bg-white p-4" @submit.prevent="add">
    <div class="min-w-64 flex-1">
      <label class="mb-1 block text-sm text-slate-600" for="new-domain">域名</label>
      <input
        id="new-domain"
        v-model="newName"
        required
        placeholder="example.com"
        class="w-full rounded border border-slate-300 px-3 py-2 text-sm"
      />
    </div>
    <button
      type="submit"
      :disabled="busy"
      class="rounded bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-700 disabled:opacity-50"
    >
      添加域名
    </button>
  </form>

  <p v-if="error" class="mb-3 text-sm text-red-600" role="alert">{{ error }}</p>

  <div class="overflow-x-auto rounded-lg border border-slate-200 bg-white">
    <table class="w-full text-left text-sm">
      <thead class="border-b border-slate-200 bg-slate-50 text-slate-600">
        <tr>
          <th class="px-4 py-2 font-medium">域名</th>
          <th class="px-4 py-2 font-medium">账号数</th>
          <th class="px-4 py-2 font-medium">添加时间</th>
          <th class="px-4 py-2 font-medium">操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="loading">
          <td colspan="4" class="px-4 py-6 text-center text-slate-500">加载中…</td>
        </tr>
        <tr v-else-if="!domains.length">
          <td colspan="4" class="px-4 py-6 text-center text-slate-500">还没有域名，当前不限制账号域名</td>
        </tr>
        <tr v-for="d in domains" :key="d.name" class="border-b border-slate-100 last:border-0">
          <td class="px-4 py-2 font-medium">{{ d.name }}</td>
          <td class="px-4 py-2 text-slate-600">{{ d.accounts }}</td>
          <td class="px-4 py-2 text-slate-600">{{ fmt(d.created_at) }}</td>
          <td class="px-4 py-2">
            <div v-if="pending === d.name" class="flex items-center gap-2">
              <span class="text-slate-600">删除后无法恢复，确认？</span>
              <button class="rounded bg-red-600 px-2 py-1 text-white hover:bg-red-700 disabled:opacity-50" :disabled="busy" @click="remove(d)">确认</button>
              <button class="rounded px-2 py-1 hover:bg-slate-100" @click="pending = null">取消</button>
            </div>
            <button
              v-else
              class="rounded px-2 py-1 text-red-600 hover:bg-red-50 disabled:opacity-50"
              :disabled="busy || d.accounts > 0"
              :title="d.accounts > 0 ? '该域名下还有账号，请先删除账号' : ''"
              @click="pending = d.name"
            >
              删除
            </button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
