/**
 * 端到端套件共享工具：真实后端 API 驱动数据准备 + 管理端 UI 登录。
 * 种子账号（admin/editor/reviewer）由 cmd/e2e seed 写入已知首登口令并保持
 * must_change_password=true；activatedToken 幂等激活：已改密则直接登录，
 * 否则走首登强制改密流程，因此各用例可任意顺序独立运行。
 */
import { expect, type APIRequestContext, type Page } from '@playwright/test'

export const API_BASE_URL = 'http://localhost:8080/api/v1'

/** cmd/e2e seed 写入的首登口令（三账号共享） */
export const SEED_PASSWORD = 'E2eSeedPass1!'

/** 首登改密后各角色固定的业务口令 */
export const ROLE_PASSWORD: Record<string, string> = {
  admin: 'AdminE2ePass1!',
  editor: 'EditorE2ePass1!',
  reviewer: 'ReviewerE2ePass1!'
}

export interface E2eArticle {
  id: string
  title: string
  summary: string
  categoryId: string
  version: number
  status: string
  rejectReason: string | null
}

export interface E2eCategory {
  id: string
  name: string
}

export interface E2eUser {
  id: string
  username: string
}

function authHeader(token: string): Record<string, string> {
  return { Authorization: `Bearer ${token}` }
}

export async function apiLogin(
  request: APIRequestContext,
  username: string,
  password: string
): Promise<{ accessToken: string; mustChangePassword: boolean } | null> {
  const res = await request.post(`${API_BASE_URL}/auth/login`, {
    data: { username, password }
  })
  if (!res.ok()) return null
  const body = await res.json()
  return { accessToken: body.accessToken, mustChangePassword: body.user.mustChangePassword }
}

export async function changePasswordApi(
  request: APIRequestContext,
  token: string,
  oldPassword: string,
  newPassword: string
): Promise<void> {
  const res = await request.post(`${API_BASE_URL}/auth/change-password`, {
    headers: authHeader(token),
    data: { oldPassword, newPassword }
  })
  expect(res.status()).toBe(204)
}

/**
 * 幂等激活：先试角色固定口令（已激活则零副作用），失败再用首登口令走强制
 * 改密。新库首轮每账号至多一次失败探测（低于登录限流阈值），已激活账号探测
 * 零失败。
 */
export async function activatedToken(request: APIRequestContext, role: string): Promise<string> {
  const final = ROLE_PASSWORD[role]
  const current = await apiLogin(request, role, final)
  if (current) return current.accessToken
  const seeded = await apiLogin(request, role, SEED_PASSWORD)
  if (!seeded) throw new Error(`cannot log in seed account "${role}"`)
  expect(seeded.mustChangePassword).toBe(true)
  await changePasswordApi(request, seeded.accessToken, SEED_PASSWORD, final)
  const after = await apiLogin(request, role, final)
  if (!after) throw new Error(`cannot log in activated account "${role}"`)
  return after.accessToken
}

/** 管理端 UI 登录（账号需已激活）：等待登录请求完成并离开登录页再返回 */
export async function loginAtAdmin(page: Page, username: string, password: string): Promise<void> {
  await page.goto('/admin/login')
  await page.getByPlaceholder('请输入账号').fill(username)
  await page.getByPlaceholder('请输入密码').fill(password)
  const loginResponse = page.waitForResponse(
    (r) => r.url().includes('/api/v1/auth/login') && r.request().method() === 'POST'
  )
  await page.getByRole('button', { name: '登录' }).click()
  const response = await loginResponse
  expect(response.status()).toBe(200)
  // 等路由完成跳转（M0 未改密 → change-password；已激活 → 管理后台），
  // 避免调用方立即 goto 中止在途请求
  await page.waitForURL((url) => !url.pathname.startsWith('/admin/login'))
}

export async function createCategoryApi(
  request: APIRequestContext,
  token: string,
  name: string
): Promise<E2eCategory> {
  const res = await request.post(`${API_BASE_URL}/admin/categories`, {
    headers: authHeader(token),
    data: {
      name,
      slug: `cat-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`,
      sortOrder: 0
    }
  })
  expect(res.status()).toBe(201)
  return res.json()
}

export async function createDraftApi(
  request: APIRequestContext,
  token: string,
  categoryId: string,
  title: string,
  bodyText = '正文内容'
): Promise<E2eArticle> {
  const res = await request.post(`${API_BASE_URL}/admin/articles`, {
    headers: authHeader(token),
    data: {
      title,
      summary: `摘要：${title}`,
      bodyHtml: `<p>${bodyText}</p>`,
      categoryId
    }
  })
  expect(res.status()).toBe(201)
  return res.json()
}

async function transition(
  request: APIRequestContext,
  token: string,
  id: string,
  action: 'submit' | 'approve' | 'reject' | 'unpublish' | 'pin',
  body?: Record<string, unknown>
): Promise<E2eArticle> {
  const res = await request.post(`${API_BASE_URL}/admin/articles/${id}/${action}`, {
    headers: authHeader(token),
    data: body
  })
  expect(res.status()).toBe(200)
  return res.json()
}

export async function submitApi(
  request: APIRequestContext,
  token: string,
  id: string
): Promise<E2eArticle> {
  return transition(request, token, id, 'submit')
}

export async function approveApi(
  request: APIRequestContext,
  token: string,
  id: string
): Promise<E2eArticle> {
  return transition(request, token, id, 'approve')
}

export async function rejectApi(
  request: APIRequestContext,
  token: string,
  id: string,
  reason: string
): Promise<E2eArticle> {
  return transition(request, token, id, 'reject', { reason })
}

export async function unpublishApi(
  request: APIRequestContext,
  token: string,
  id: string,
  reason?: string
): Promise<E2eArticle> {
  return transition(request, token, id, 'unpublish', reason ? { reason } : undefined)
}

export async function softDeleteApi(
  request: APIRequestContext,
  token: string,
  id: string
): Promise<void> {
  const res = await request.delete(`${API_BASE_URL}/admin/articles/${id}`, {
    headers: authHeader(token)
  })
  expect(res.status()).toBe(204)
}

/** 读取当前版本并对外提交一次更新（乐观锁：If-Match 当前版本），用于制造版本冲突 */
export async function bumpArticleVersion(
  request: APIRequestContext,
  token: string,
  id: string
): Promise<void> {
  const res = await request.get(`${API_BASE_URL}/admin/articles/${id}`, {
    headers: authHeader(token)
  })
  expect(res.status()).toBe(200)
  const article: E2eArticle = await res.json()
  const upd = await request.put(`${API_BASE_URL}/admin/articles/${id}`, {
    headers: { ...authHeader(token), 'If-Match': String(article.version) },
    data: {
      title: article.title,
      summary: `${article.summary}（外部修订）`,
      bodyHtml: `<p>正文内容</p>`,
      categoryId: article.categoryId
    }
  })
  expect(upd.status()).toBe(200)
}

/** 按标题在管理端列表中定位文章（返回最近一条匹配） */
export async function findArticleByTitle(
  request: APIRequestContext,
  token: string,
  title: string
): Promise<E2eArticle> {
  const res = await request.get(
    `${API_BASE_URL}/admin/articles?page=1&pageSize=100&keyword=${encodeURIComponent(title)}`,
    { headers: authHeader(token) }
  )
  expect(res.status()).toBe(200)
  const body = await res.json()
  const hit: E2eArticle | undefined = body.items.find((item: E2eArticle) => item.title === title)
  expect(hit).toBeTruthy()
  return hit as E2eArticle
}

export async function createUserApi(
  request: APIRequestContext,
  adminToken: string,
  username: string,
  role: string,
  temporaryPassword: string
): Promise<E2eUser> {
  const res = await request.post(`${API_BASE_URL}/admin/users`, {
    headers: authHeader(adminToken),
    data: { username, password: temporaryPassword, displayName: username, role }
  })
  expect(res.status()).toBe(201)
  return res.json()
}
