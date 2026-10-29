import { ref } from 'vue'
import { defineStore } from 'pinia'
import { api, ApiError } from '@/api/client'

export type Role = 'admin' | 'user'

interface Me {
  role: Role
  address?: string
}

export const useAuthStore = defineStore('auth', () => {
  const role = ref<Role | null>(null)
  const address = ref<string | null>(null)
  const loaded = ref(false)

  function set(me: Me) {
    role.value = me.role
    address.value = me.address ?? null
    loaded.value = true
  }

  function reset() {
    role.value = null
    address.value = null
  }

  // 只有拿到明确结果（身份或 401）才标记 loaded；网络错误、5xx 抛给调用方，保留重试机会。
  async function fetchMe() {
    try {
      set(await api<Me>('/me'))
    } catch (e) {
      if (!(e instanceof ApiError && e.status === 401)) throw e
      reset()
      loaded.value = true
    }
  }

  async function login(addr: string, password: string) {
    set(await api<Me>('/auth/login', { method: 'POST', body: JSON.stringify({ address: addr, password }) }))
  }

  async function adminLogin(token: string) {
    set(await api<Me>('/auth/admin-login', { method: 'POST', body: JSON.stringify({ token }) }))
  }

  async function logout() {
    await api('/auth/logout', { method: 'POST' })
    reset()
  }

  return { role, address, loaded, fetchMe, login, adminLogin, logout, reset }
})

export function homeFor(role: Role) {
  return role === 'admin' ? '/admin/accounts' : '/inbox'
}
