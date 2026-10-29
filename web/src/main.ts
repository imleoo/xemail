import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router, { loginPathFor } from './router'
import { setUnauthorizedHandler } from './api/client'
import { useAuthStore } from './stores/auth'
import './assets/main.css'

const app = createApp(App).use(createPinia()).use(router)

// 先向服务端确认当前身份：退出再登录后，旧请求迟到的 401 不应清掉新会话。
setUnauthorizedHandler(() => {
  const auth = useAuthStore()
  auth
    .fetchMe()
    .catch(() => {})
    .finally(() => {
      if (!auth.role) router.push(loginPathFor(router.currentRoute.value.path))
    })
})

app.mount('#app')
