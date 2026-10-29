<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, RouterView, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()

const nav = computed(() =>
  auth.role === 'admin'
    ? [
        { to: '/admin/accounts', label: '账号管理' },
        { to: '/admin/domains', label: '域名管理' },
        { to: '/admin/unrouted', label: '未匹配邮件' },
      ]
    : [{ to: '/inbox', label: '收件箱' }],
)

async function logout() {
  const target = auth.role === 'admin' ? '/admin/login' : '/login' // 退出后回到各自的登录入口
  await auth.logout()
  router.push(target)
}
</script>

<template>
  <div v-if="auth.role" class="flex h-screen overflow-hidden bg-slate-50 text-slate-900">
    <aside class="flex w-52 shrink-0 flex-col overflow-y-auto border-r border-slate-200 bg-white p-4">
      <h1 class="mb-6 text-lg font-semibold">xemail</h1>
      <nav class="flex flex-col gap-1">
        <RouterLink
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          class="rounded px-3 py-2 text-sm hover:bg-slate-100"
          active-class="bg-slate-100 font-medium"
        >
          {{ item.label }}
        </RouterLink>
      </nav>
      <div class="mt-auto border-t border-slate-200 pt-3 text-sm">
        <p class="mb-2 truncate text-slate-500">{{ auth.role === 'admin' ? '管理员' : auth.address }}</p>
        <button class="rounded px-3 py-1.5 text-slate-700 hover:bg-slate-100" @click="logout">退出登录</button>
      </div>
    </aside>
    <main class="min-w-0 flex-1 overflow-y-auto p-6">
      <RouterView />
    </main>
  </div>
  <div v-else class="min-h-screen bg-slate-50 text-slate-900">
    <RouterView />
  </div>
</template>
