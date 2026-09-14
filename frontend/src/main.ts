import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import { createPinia } from 'pinia'
import { createApp } from 'vue'

import 'element-plus/dist/index.css'
import App from './App.vue'
import router from './router'
import { setAuthFailureHandler } from './shared/api/http'
import { useAuthStore } from './shared/stores/auth'
import './style/main.css'

// 会话失效（refresh 旋转失败）时清理本地会话并回登录页
setAuthFailureHandler(() => {
  const authStore = useAuthStore()
  authStore.clear()
  if (router.currentRoute.value.name !== 'admin-login') {
    void router.push({
      name: 'admin-login',
      query: { redirect: router.currentRoute.value.fullPath }
    })
  }
})

createApp(App).use(createPinia()).use(router).use(ElementPlus, { locale: zhCn }).mount('#app')
