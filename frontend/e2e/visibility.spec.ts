/**
 * 浏览端可见性：首页仅展示已发布文章；直连非发布文章一律 404 防枚举；
 * 公共分类只列出含已发布内容的分类；搜索仅命中已发布文章且空关键词不触发。
 * 数据由 beforeAll 通过真实 API 铺设（两个分类：一个含已发布内容、一个仅含非发布内容）。
 */
import { expect, test, type APIRequestContext } from '@playwright/test'

import {
  activatedToken,
  approveApi,
  createCategoryApi,
  createDraftApi,
  loginAtAdmin,
  ROLE_PASSWORD,
  softDeleteApi,
  submitApi,
  unpublishApi,
  type E2eCategory
} from './helpers'

let api: APIRequestContext
let adminToken: string
let catVisible: E2eCategory
let catHidden: E2eCategory

let pubTitle: string
let pubBodyToken: string
let pubId: string
let draftTitle: string
let draftId: string
let unpubTitle: string
let unpubId: string
let deletedTitle: string
let deletedId: string
let hiddenDraftTitle: string

test.beforeAll(async ({ playwright }) => {
  api = await playwright.request.newContext()
  adminToken = await activatedToken(api, 'admin')
  const runId = Date.now().toString(36)

  catVisible = await createCategoryApi(api, adminToken, `时讯频道-${runId}`)
  catHidden = await createCategoryApi(api, adminToken, `内部档案-${runId}`)

  pubTitle = `时讯发布稿-${runId}`
  pubBodyToken = `量子隧道${runId}`
  draftTitle = `时讯草稿-${runId}`
  unpubTitle = `时讯下架稿-${runId}`
  deletedTitle = `时讯已删稿-${runId}`
  hiddenDraftTitle = `存档草稿-${runId}`

  const pub = await createDraftApi(api, adminToken, catVisible.id, pubTitle, pubBodyToken)
  await submitApi(api, adminToken, pub.id)
  await approveApi(api, adminToken, pub.id)
  pubId = pub.id

  const draft = await createDraftApi(api, adminToken, catVisible.id, draftTitle)
  draftId = draft.id

  const unpub = await createDraftApi(api, adminToken, catVisible.id, unpubTitle)
  await submitApi(api, adminToken, unpub.id)
  await approveApi(api, adminToken, unpub.id)
  await unpublishApi(api, adminToken, unpub.id)
  unpubId = unpub.id

  const deleted = await createDraftApi(api, adminToken, catVisible.id, deletedTitle)
  deletedId = deleted.id
  await softDeleteApi(api, adminToken, deleted.id)

  await createDraftApi(api, adminToken, catHidden.id, hiddenDraftTitle)
  const hiddenUnpub = await createDraftApi(api, adminToken, catHidden.id, `存档下架稿-${runId}`)
  await submitApi(api, adminToken, hiddenUnpub.id)
  await approveApi(api, adminToken, hiddenUnpub.id)
  await unpublishApi(api, adminToken, hiddenUnpub.id)
})

test.afterAll(async () => {
  await api.dispose()
})

test('首页列表只展示已发布文章', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByText(pubTitle, { exact: true })).toBeVisible()
  for (const title of [draftTitle, unpubTitle, deletedTitle, hiddenDraftTitle]) {
    await expect(page.getByText(title, { exact: true })).toHaveCount(0)
  }
})

test('浏览端直连非发布文章一律返回 404', async ({ page }) => {
  await page.goto(`/articles/${pubId}`)
  await expect(page.getByRole('heading', { name: pubTitle, exact: true })).toBeVisible()

  for (const id of [draftId, unpubId, deletedId]) {
    await page.goto(`/articles/${id}`)
    await expect(page.getByText('页面不存在或已移除')).toBeVisible()
  }
})

test('公共分类只列出含有已发布内容的分类', async ({ page, request }) => {
  await page.goto('/')
  const chip = (name: string) => page.getByRole('button', { name })
  await expect(chip('全部')).toBeVisible()
  await expect(chip(catVisible.name)).toBeVisible()
  await expect(chip(catHidden.name)).toHaveCount(0)

  // 分类筛选仅显示该分类下的已发布内容
  await chip(catVisible.name).click()
  await expect(page.getByText(pubTitle, { exact: true })).toBeVisible()
  await expect(page.getByText(draftTitle, { exact: true })).toHaveCount(0)

  // 管理端分类列表仍完整展示两类（含仅存非发布内容的分类）
  await activatedToken(request, 'admin')
  await loginAtAdmin(page, 'admin', ROLE_PASSWORD.admin)
  await page.goto('/admin/categories')
  await expect(page.locator('.el-table__row', { hasText: catVisible.name })).toBeVisible()
  await expect(page.locator('.el-table__row', { hasText: catHidden.name })).toBeVisible()
})

test('搜索只返回已发布文章且能命中正文', async ({ page }) => {
  await page.goto('/search')
  await page.getByPlaceholder('输入关键词，匹配标题/摘要/正文').fill(pubBodyToken)
  await page.getByRole('button', { name: '搜索' }).click()
  await expect(page.getByText(pubTitle, { exact: true })).toBeVisible()

  await page.getByPlaceholder('输入关键词，匹配标题/摘要/正文').fill(draftTitle)
  await page.getByRole('button', { name: '搜索' }).click()
  await expect(page.getByText(`未找到与「${draftTitle}」相关的内容`)).toBeVisible()
})

test('空关键词不触发搜索', async ({ page }) => {
  await page.goto('/search')
  await page.getByRole('button', { name: '搜索' }).click()
  await expect(page.getByText('输入关键词开始搜索（支持标题、摘要与正文内容）')).toBeVisible()
  await expect(page.getByText(/「」的结果/)).toHaveCount(0)
})
