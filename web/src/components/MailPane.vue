<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, ApiError } from '@/api/client'

// base 为邮件接口前缀：用户收件箱是 /mails，管理员未匹配邮件是 /admin/mails/unrouted。
const props = defineProps<{ base: string; emptyText: string }>()

interface Summary {
  id: string
  from: string
  to: string[]
  subject: string
  received_at: number
  is_read: boolean
  has_attachments: boolean
}

interface Attachment {
  id: string
  filename: string
  content_type: string
}

interface Mail extends Summary {
  cc: string[]
  html: string
  text: string
  attachments: Attachment[]
}

const PAGE_SIZE = 30
const REFRESH_MS = 60_000

const mails = ref<Summary[]>([])
const total = ref(0)
const page = ref(0)
const loading = ref(true)
const listError = ref('')

const selectedId = ref<string | null>(null)
const mail = ref<Mail | null>(null)
const mailLoading = ref(false)
const mailError = ref('')
let openSeq = 0

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))

function errText(e: unknown) {
  return e instanceof ApiError ? e.message : '加载失败，请稍后重试'
}

function fmt(ts: number) {
  return new Date(ts * 1000).toLocaleString()
}

async function load() {
  try {
    const res = await api<{ mails: Summary[]; total: number }>(
      `${props.base}?limit=${PAGE_SIZE}&offset=${page.value * PAGE_SIZE}`,
    )
    mails.value = res.mails
    total.value = res.total
    listError.value = ''
  } catch (e) {
    listError.value = errText(e)
  } finally {
    loading.value = false
  }
}

async function open(id: string) {
  selectedId.value = id
  mailLoading.value = true
  mailError.value = ''
  const seq = ++openSeq // 连续点击时只采用最后一次请求的结果
  try {
    const res = await api<Mail>(`${props.base}/${encodeURIComponent(id)}`)
    if (seq !== openSeq) return
    mail.value = res
    const item = mails.value.find((m) => m.id === id)
    if (item) item.is_read = true
  } catch (e) {
    if (seq !== openSeq) return
    mail.value = null
    mailError.value = errText(e)
  } finally {
    if (seq === openSeq) mailLoading.value = false
  }
}

function go(delta: number) {
  page.value = Math.min(Math.max(page.value + delta, 0), pageCount.value - 1)
  load()
}

// 沙箱 iframe：不允许脚本和同源访问；链接在新窗口打开；不发送 Referer。
function srcdoc(html: string) {
  return `<base target="_blank"><meta name="referrer" content="no-referrer">${html}`
}

function attachmentHref(id: string, aid: string) {
  return `/api${props.base}/${encodeURIComponent(id)}/attachments/${encodeURIComponent(aid)}`
}

let timer: number | undefined
onMounted(() => {
  load()
  timer = window.setInterval(load, REFRESH_MS)
})
onBeforeUnmount(() => window.clearInterval(timer))

defineExpose({ reload: load })
</script>

<template>
  <div class="flex flex-col gap-4 lg:h-full lg:min-h-0 lg:flex-row">
    <section class="flex w-full shrink-0 flex-col overflow-hidden rounded-lg border border-slate-200 bg-white lg:min-h-0 lg:w-96">
      <div class="flex shrink-0 items-center justify-between border-b border-slate-200 px-4 py-2 text-sm text-slate-600">
        <span>共 {{ total }} 封</span>
        <button class="rounded px-2 py-1 hover:bg-slate-100" @click="load">刷新</button>
      </div>

      <p v-if="listError" class="px-4 py-3 text-sm text-red-600" role="alert">{{ listError }}</p>
      <p v-else-if="loading" class="px-4 py-6 text-center text-sm text-slate-500">加载中…</p>
      <p v-else-if="!mails.length" class="px-4 py-6 text-center text-sm text-slate-500">{{ emptyText }}</p>

      <ul class="max-h-72 overflow-y-auto lg:max-h-none lg:min-h-0 lg:flex-1">
        <li v-for="m in mails" :key="m.id">
          <button
            class="block w-full border-b border-slate-100 px-4 py-3 text-left hover:bg-slate-50"
            :class="selectedId === m.id ? 'bg-slate-100' : ''"
            @click="open(m.id)"
          >
            <div class="flex items-center gap-2">
              <span v-if="!m.is_read" class="h-2 w-2 shrink-0 rounded-full bg-blue-600" aria-label="未读"></span>
              <span class="truncate text-sm" :class="m.is_read ? 'text-slate-700' : 'font-semibold'">
                {{ m.subject || '（无主题）' }}
              </span>
              <span v-if="m.has_attachments" class="ml-auto shrink-0 text-xs text-slate-500" aria-label="含附件">📎</span>
            </div>
            <div class="mt-0.5 truncate text-xs text-slate-500">{{ m.from }}</div>
            <div class="mt-0.5 text-xs text-slate-400">{{ fmt(m.received_at) }}</div>
          </button>
        </li>
      </ul>

      <div v-if="pageCount > 1" class="flex shrink-0 items-center justify-between border-t border-slate-200 px-4 py-2 text-sm">
        <button class="rounded px-2 py-1 hover:bg-slate-100 disabled:opacity-40" :disabled="page === 0" @click="go(-1)">
          上一页
        </button>
        <span class="text-slate-500">{{ page + 1 }} / {{ pageCount }}</span>
        <button
          class="rounded px-2 py-1 hover:bg-slate-100 disabled:opacity-40"
          :disabled="page >= pageCount - 1"
          @click="go(1)"
        >
          下一页
        </button>
      </div>
    </section>

    <section class="min-h-96 min-w-0 flex-1 overflow-hidden rounded-lg border border-slate-200 bg-white lg:min-h-0">
      <p v-if="!selectedId" class="px-6 py-10 text-center text-sm text-slate-500">选择左侧的邮件查看内容</p>
      <p v-else-if="mailLoading" class="px-6 py-10 text-center text-sm text-slate-500">加载中…</p>
      <p v-else-if="mailError" class="px-6 py-4 text-sm text-red-600" role="alert">{{ mailError }}</p>

      <article v-else-if="mail" class="flex flex-col lg:h-full">
        <header class="shrink-0 border-b border-slate-200 px-6 py-4">
          <h3 class="mb-2 text-lg font-semibold break-words">{{ mail.subject || '（无主题）' }}</h3>
          <dl class="grid grid-cols-[3rem_1fr] gap-y-1 text-sm text-slate-600">
            <dt class="text-slate-400">发件人</dt>
            <dd class="break-all">{{ mail.from }}</dd>
            <dt class="text-slate-400">收件人</dt>
            <dd class="break-all">{{ mail.to.join(', ') }}</dd>
            <template v-if="mail.cc.length">
              <dt class="text-slate-400">抄送</dt>
              <dd class="break-all">{{ mail.cc.join(', ') }}</dd>
            </template>
            <dt class="text-slate-400">时间</dt>
            <dd>{{ fmt(mail.received_at) }}</dd>
          </dl>
          <ul v-if="mail.attachments.length" class="mt-3 flex flex-wrap gap-2">
            <li v-for="a in mail.attachments" :key="a.id">
              <a
                :href="attachmentHref(mail.id, a.id)"
                target="_blank"
                rel="noopener noreferrer"
                class="inline-block rounded border border-slate-300 px-2 py-1 text-xs hover:bg-slate-50"
              >
                📎 {{ a.filename || '未命名附件' }}
              </a>
            </li>
          </ul>
        </header>

        <iframe
          v-if="mail.html"
          :key="mail.id"
          :srcdoc="srcdoc(mail.html)"
          sandbox="allow-popups allow-popups-to-escape-sandbox"
          referrerpolicy="no-referrer"
          title="邮件正文"
          class="block h-[60vh] w-full lg:h-auto lg:min-h-0 lg:flex-1"
        ></iframe>
        <pre v-else class="max-h-[60vh] overflow-auto px-6 py-4 text-sm break-words whitespace-pre-wrap lg:max-h-none lg:min-h-0 lg:flex-1">{{ mail.text }}</pre>
      </article>
    </section>
  </div>
</template>
