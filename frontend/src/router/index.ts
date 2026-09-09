import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/shared/stores/auth'
import type { Role } from '@/shared/types/api'

declare module 'vue-router' {
  interface RouteMeta {
    /** 是否需要认证（admin 壳整体声明，子路由继承） */
    requiresAuth?: boolean
    /** 角色声明占位：路由级 RBAC 声明，真实鉴权随会话模块（C20）与本店守卫落地 */
    roles?: Role[]
    /** 仅未登录可访问（如登录页） */
    guestOnly?: boolean
    title?: string
  }
}

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    component: () => import('@/public/layouts/PublicLayout.vue'),
    children: [
      {
        path: '',
        name: 'public-home',
        component: () => import('@/public/views/HomeView.vue'),
        meta: { title: '首页' },
      },
      {
        path: 'articles/:id',
        name: 'public-article-detail',
        component: () => import('@/public/views/ArticleDetailView.vue'),
        meta: { title: '文章详情' },
      },
    ],
  },
  {
    path: '/admin/login',
    name: 'admin-login',
    component: () => import('@/admin/views/LoginView.vue'),
    meta: { guestOnly: true, title: '登录' },
  },
  {
    path: '/admin',
    component: () => import('@/admin/layouts/AdminLayout.vue'),
    meta: { requiresAuth: true, title: '管理台' },
    children: [
      { path: '', redirect: { name: 'admin-articles' } },
      {
        path: 'articles',
        name: 'admin-articles',
        component: () => import('@/admin/views/ArticlesView.vue'),
        // 所有管理角色均可查看/操作文章（编辑/提交/审核权由权限点细分）
        meta: { roles: ['admin', 'editor', 'reviewer'], title: '文章管理' },
      },
      {
        path: 'categories',
        name: 'admin-categories',
        component: () => import('@/admin/views/CategoriesView.vue'),
        meta: { roles: ['admin', 'editor'], title: '分类管理' },
      },
      {
        path: 'users',
        name: 'admin-users',
        component: () => import('@/admin/views/UsersView.vue'),
        meta: { roles: ['admin'], title: '用户管理' },
      },
      {
        path: 'audit-logs',
        name: 'admin-audit-logs',
        component: () => import('@/admin/views/AuditLogsView.vue'),
        meta: { roles: ['admin'], title: '审计日志' },
      },
    ],
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/shared/views/NotFoundView.vue'),
    meta: { title: '页面不存在' },
  },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
})

// 守卫占位：仅处理 requiresAuth 与 guestOnly，RBAC 角色校验待会话模块落地后启用
router.beforeEach((to) => {
  const authStore = useAuthStore()

  if (to.meta.guestOnly && authStore.isAuthenticated) {
    return { name: 'admin-articles' }
  }
  if (to.meta.requiresAuth && !authStore.isAuthenticated) {
    return { name: 'admin-login', query: { redirect: to.fullPath } }
  }
  return true
})

export default router