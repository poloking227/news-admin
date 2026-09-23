import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/shared/stores/auth'
import type { Role } from '@/shared/types/api'

declare module 'vue-router' {
  interface RouteMeta {
    /** 是否需要认证（admin 壳整体声明，子路由继承） */
    requiresAuth?: boolean
    /** 路由级 RBAC 角色声明；守卫基于 /auth/me 返回的 role 校验 */
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
        meta: { title: '首页' }
      },
      {
        path: 'articles/:id',
        name: 'public-article-detail',
        component: () => import('@/public/views/ArticleDetailView.vue'),
        meta: { title: '文章详情' }
      },
      {
        path: 'search',
        name: 'public-search',
        component: () => import('@/public/views/SearchView.vue'),
        meta: { title: '搜索' }
      }
    ]
  },
  {
    path: '/admin/login',
    name: 'admin-login',
    component: () => import('@/admin/views/LoginView.vue'),
    meta: { guestOnly: true, title: '登录' }
  },
  {
    path: '/admin',
    component: () => import('@/admin/layouts/AdminLayout.vue'),
    meta: { requiresAuth: true, title: '管理台' },
    children: [
      { path: '', redirect: { name: 'admin-articles' } },
      {
        path: 'change-password',
        name: 'admin-change-password',
        component: () => import('@/admin/views/ChangePasswordView.vue'),
        // M0 门控期间唯一放行的管理路由；所有管理角色均可修改本人密码
        meta: { requiresAuth: true, title: '修改密码' }
      },
      {
        path: 'articles',
        name: 'admin-articles',
        component: () => import('@/admin/views/ArticlesView.vue'),
        // 所有管理角色均可查看/操作文章（编辑/提交/审核权由权限点细分）
        meta: { roles: ['admin', 'editor', 'reviewer'], title: '文章管理' }
      },
      {
        path: 'categories',
        name: 'admin-categories',
        component: () => import('@/admin/views/CategoriesView.vue'),
        // 分类管理仅管理员可操作（契约 /admin/categories）
        meta: { roles: ['admin'], title: '分类管理' }
      },
      {
        path: 'users',
        name: 'admin-users',
        component: () => import('@/admin/views/UsersView.vue'),
        meta: { roles: ['admin'], title: '用户管理' }
      },
      {
        path: 'audit-logs',
        name: 'admin-audit-logs',
        component: () => import('@/admin/views/AuditLogsView.vue'),
        meta: { roles: ['admin'], title: '审计日志' }
      }
    ]
  },
  {
    path: '/forbidden',
    name: 'forbidden',
    component: () => import('@/shared/views/ForbiddenView.vue'),
    meta: { title: '无权访问' }
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/shared/views/NotFoundView.vue'),
    meta: { title: '页面不存在' }
  }
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes
})

/** 登录后着陆：M0 门控优先改密 */
function homeRedirect(): { name: string; query?: Record<string, string> } {
  return useAuthStore().mustChangePassword
    ? { name: 'admin-change-password' }
    : { name: 'admin-articles' }
}

router.beforeEach(async (to) => {
  const auth = useAuthStore()

  if (to.meta.guestOnly) {
    if (auth.isAuthenticated) return homeRedirect()
    // 刷新页面场景：凭 refresh cookie 静默恢复会话后直接进入管理端
    if (await auth.restoreSession()) return homeRedirect()
    return true
  }

  if (to.meta.requiresAuth) {
    if (!auth.isAuthenticated && !(await auth.restoreSession())) {
      return { name: 'admin-login', query: { redirect: to.fullPath } }
    }
    // M0 首登强制改密门控：仅放行改密页（logout/me 由会话层放行）
    if (auth.mustChangePassword && to.name !== 'admin-change-password') {
      return { name: 'admin-change-password', query: { redirect: to.fullPath } }
    }
    if (to.meta.roles && (!auth.role || !to.meta.roles.includes(auth.role))) {
      // 已认证但角色不匹配：403 提示（区别于未认证的 404/登录跳转）
      return { name: 'forbidden' }
    }
    return true
  }

  return true
})

export default router
