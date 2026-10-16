/**
 * 认证与权限：未登录重定向、错误口令拒绝、首登强制改密（M0）门控、
 * 管理端路由按角色门控（编辑不可进入仅限管理员的模块）。
 */
import { expect, test } from '@playwright/test'

import { activatedToken, createUserApi, loginAtAdmin, ROLE_PASSWORD } from './helpers'

test('未登录访问管理页被重定向到登录页', async ({ page }) => {
  await page.goto('/admin/articles')
  await expect(page).toHaveURL(/\/admin\/login/)
  await expect(page.getByRole('heading', { name: '管理端登录' })).toBeVisible()
})

test('错误口令登录被拒绝并提示，停留在登录页', async ({ page }) => {
  await page.goto('/admin/login')
  await page.getByPlaceholder('请输入账号').fill('admin')
  await page.getByPlaceholder('请输入密码').fill('Wrong-E2e-12345')
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page.locator('.el-message')).toContainText('invalid username or password')
  await expect(page).toHaveURL(/\/admin\/login/)
  await expect(page.getByRole('heading', { name: '管理端登录' })).toBeVisible()
})

test('新用户首次登录被强制修改临时口令，改密前业务入口全部被拦', async ({ browser, request }) => {
  const adminToken = await activatedToken(request, 'admin')
  const username = `m0-${Date.now().toString(36)}`
  const temporaryPassword = 'TempPass1!'
  await createUserApi(request, adminToken, username, 'editor', temporaryPassword)

  const context = await browser.newContext()
  const page = await context.newPage()
  try {
    // 临时口令登录 → 直接落入强制改密页
    await loginAtAdmin(page, username, temporaryPassword)
    await expect(page).toHaveURL(/\/admin\/change-password/)
    await expect(page.getByText('首次登录须修改初始密码后方可继续')).toBeVisible()

    // M0 门控期间，业务路由即使直连也会被拦回改密页
    await page.goto('/admin/articles')
    await expect(page).toHaveURL(/\/admin\/change-password/)

    // 完成改密：填入当前/新/确认密码
    await page.locator('input[name="old-password"]').fill(temporaryPassword)
    await page.getByPlaceholder('至少 8 位').fill('NewPass123!')
    await page.locator('input[name="confirm-password"]').fill('NewPass123!')
    await page.getByRole('button', { name: '确认修改' }).click()
    await expect(page.locator('.el-message')).toContainText('密码已修改，请使用新密码重新登录')
    await expect(page).toHaveURL(/\/admin\/login/)

    // 新密码重新登录 → 进入管理端文章工作台
    await loginAtAdmin(page, username, 'NewPass123!')
    await expect(page).toHaveURL(/\/admin\/articles/)
    await expect(page.getByRole('heading', { name: '文章管理' })).toBeVisible()
  } finally {
    await context.close()
  }
})

test('编辑角色无法进入仅限管理员的模块', async ({ page, request }) => {
  await activatedToken(request, 'editor')
  await loginAtAdmin(page, 'editor', ROLE_PASSWORD.editor)

  await expect(page).toHaveURL(/\/admin\/articles/)
  await expect(page.getByRole('heading', { name: '文章管理' })).toBeVisible()

  // 导航按权限点过滤：编辑看不到用户与审计入口
  await expect(page.getByRole('link', { name: '用户管理' })).toHaveCount(0)
  await expect(page.getByRole('link', { name: '审计日志' })).toHaveCount(0)

  // 直连仅限管理员的地址返回无权访问页（403 语义，区别于未登录的登录跳转）
  await page.goto('/admin/categories')
  await expect(page.getByText('无权访问该页面，请联系管理员')).toBeVisible()
  await page.goto('/admin/users')
  await expect(page.getByText('无权访问该页面，请联系管理员')).toBeVisible()
})

test('管理员可进入全部管理模块', async ({ page, request }) => {
  await activatedToken(request, 'admin')
  await loginAtAdmin(page, 'admin', ROLE_PASSWORD.admin)

  const modules: [string, string][] = [
    ['/admin/articles', '文章管理'],
    ['/admin/categories', '分类管理'],
    ['/admin/users', '用户管理'],
    ['/admin/audit-logs', '审计日志']
  ]
  for (const [path, heading] of modules) {
    await page.goto(path)
    await expect(page.getByRole('heading', { name: heading })).toBeVisible()
  }
})
