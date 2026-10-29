<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ApiError } from '@/api/client'
import { homeFor, useAuthStore } from '@/stores/auth'

// 同一个页面服务两个入口：/login 是邮箱账号登录，/admin/login 是管理员登录（路由 meta.admin）。
const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const isAdmin = computed(() => route.meta.admin === true)

const address = ref('')
const password = ref('')
const token = ref('')
const error = ref('')
const submitting = ref(false)

async function submit() {
  error.value = ''
  submitting.value = true
  try {
    if (isAdmin.value) await auth.adminLogin(token.value)
    else await auth.login(address.value, password.value)
    router.replace(homeFor(auth.role!))
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '登录失败，请稍后重试'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="flex min-h-screen items-center justify-center p-4">
    <form class="w-full max-w-sm rounded-lg border border-slate-200 bg-white p-6 shadow-sm" @submit.prevent="submit">
      <h2 class="mb-4 text-xl font-semibold">{{ isAdmin ? '管理员登录' : '登录 xemail' }}</h2>

      <template v-if="!isAdmin">
        <label class="mb-1 block text-sm text-slate-600" for="address">邮箱地址</label>
        <input
          id="address"
          v-model="address"
          type="email"
          required
          autocomplete="username"
          class="mb-3 w-full rounded border border-slate-300 px-3 py-2 text-sm"
        />
        <label class="mb-1 block text-sm text-slate-600" for="password">密码</label>
        <input
          id="password"
          v-model="password"
          type="password"
          required
          autocomplete="current-password"
          class="mb-4 w-full rounded border border-slate-300 px-3 py-2 text-sm"
        />
      </template>
      <template v-else>
        <label class="mb-1 block text-sm text-slate-600" for="token">管理员令牌</label>
        <input
          id="token"
          v-model="token"
          type="password"
          required
          autocomplete="off"
          class="mb-4 w-full rounded border border-slate-300 px-3 py-2 text-sm"
        />
      </template>

      <p v-if="error" class="mb-3 text-sm text-red-600" role="alert">{{ error }}</p>
      <button
        type="submit"
        :disabled="submitting"
        class="w-full rounded bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-700 disabled:opacity-50"
      >
        {{ submitting ? '登录中…' : '登录' }}
      </button>
    </form>
  </div>
</template>
