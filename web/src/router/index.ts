import { createRouter, createWebHistory } from 'vue-router'
import { homeFor, useAuthStore, type Role } from '@/stores/auth'

declare module 'vue-router' {
  interface RouteMeta {
    public?: boolean
    role?: Role
    admin?: boolean // 管理员登录入口
  }
}

// 未登录时按访问的区域决定去哪个登录页：/admin/* 去管理员登录，其余去邮箱账号登录。
export function loginPathFor(path: string) {
  return path === '/admin' || path.startsWith('/admin/') ? '/admin/login' : '/login'
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/inbox' },
    { path: '/login', component: () => import('@/views/LoginView.vue'), meta: { public: true } },
    { path: '/admin', redirect: '/admin/accounts' },
    { path: '/admin/login', component: () => import('@/views/LoginView.vue'), meta: { public: true, admin: true } },
    { path: '/inbox', component: () => import('@/views/InboxView.vue'), meta: { role: 'user' } },
    { path: '/admin/accounts', component: () => import('@/views/AccountsView.vue'), meta: { role: 'admin' } },
    { path: '/admin/domains', component: () => import('@/views/DomainsView.vue'), meta: { role: 'admin' } },
    { path: '/admin/unrouted', component: () => import('@/views/UnroutedView.vue'), meta: { role: 'admin' } },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.loaded) {
    try {
      await auth.fetchMe()
    } catch {
      // 服务暂不可用：不标记 loaded，下次导航会重试；登录页仍可进入
      return to.meta.public ? true : loginPathFor(to.path)
    }
  }

  if (to.meta.public) return auth.role ? homeFor(auth.role) : true
  if (!auth.role) return loginPathFor(to.path)
  if (to.meta.role && to.meta.role !== auth.role) return homeFor(auth.role)
  return true
})

export default router
