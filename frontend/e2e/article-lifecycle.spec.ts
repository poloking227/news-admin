/**
 * 文章生命周期：单条旅程覆盖 草稿→提交→审核即发布→下架→重提交→驳回
 * （空理由被拦/带理由回落草稿）→再提交，跨编辑/审核/匿名三种视角；
 * 另以独立用例断言乐观锁（If-Match）版本冲突被拒绝，并以 API 断言流转契约。
 */
import { expect, test, type Page } from '@playwright/test'

import {
  activatedToken,
  approveApi,
  bumpArticleVersion,
  createCategoryApi,
  createDraftApi,
  findArticleByTitle,
  loginAtAdmin,
  rejectApi,
  ROLE_PASSWORD,
  submitApi,
  unpublishApi
} from './helpers'

function articleRow(page: Page, title: string) {
  return page.locator('.el-table__row', { hasText: title })
}

test('文章全生命周期走通：草稿、提交、审核发布、下架、重提交、驳回、再提交', async ({
  browser,
  request
}) => {
  const adminToken = await activatedToken(request, 'admin')
  await activatedToken(request, 'editor')
  await activatedToken(request, 'reviewer')
  const category = await createCategoryApi(request, adminToken, `科技-${Date.now().toString(36)}`)

  const runId = Date.now().toString(36)
  const editorTitle = `量子通信前沿-${runId}`
  const revisedTitle = `${editorTitle}-修订版`

  const editorContext = await browser.newContext()
  const reviewerContext = await browser.newContext()
  const anonContext = await browser.newContext()
  const editorPage = await editorContext.newPage()
  const reviewerPage = await reviewerContext.newPage()
  const anonPage = await anonContext.newPage()

  try {
    await loginAtAdmin(editorPage, 'editor', ROLE_PASSWORD.editor)
    await loginAtAdmin(reviewerPage, 'reviewer', ROLE_PASSWORD.reviewer)
    await expect(editorPage.getByRole('heading', { name: '文章管理' })).toBeVisible()
    await expect(reviewerPage.getByRole('heading', { name: '文章管理' })).toBeVisible()

    await test.step('编辑经表单创建草稿', async () => {
      await editorPage.getByRole('button', { name: '新建文章' }).click()
      await editorPage.getByPlaceholder('文章标题').fill(editorTitle)
      await editorPage.getByPlaceholder('文章摘要（500 字内）').fill(`摘要：${editorTitle}`)
      await editorPage.locator('.el-dialog .el-select').click()
      await editorPage.locator('.el-select-dropdown__item', { hasText: category.name }).click()
      await editorPage
        .locator('[data-testid="tiptap-editor"] .tiptap')
        .fill('量子纠缠密钥分发进入实用验证阶段。')
      await editorPage.getByRole('button', { name: '保存草稿' }).click()
      await expect(articleRow(editorPage, editorTitle)).toBeVisible()
      await expect(
        articleRow(editorPage, editorTitle).getByText('草稿', { exact: true })
      ).toBeVisible()
    })

    // 记录文章 id 供浏览端直连校验
    const article = await findArticleByTitle(request, adminToken, editorTitle)

    await test.step('编辑提交审核，编辑侧无审核操作入口', async () => {
      await articleRow(editorPage, editorTitle).getByRole('button', { name: '提交' }).click()
      await editorPage.locator('.el-message-box').getByRole('button', { name: '确认' }).click()
      await expect(
        articleRow(editorPage, editorTitle).getByText('待审核', { exact: true })
      ).toBeVisible()
      await expect(
        articleRow(editorPage, editorTitle).getByRole('button', { name: '通过' })
      ).toHaveCount(0)
      await expect(
        articleRow(editorPage, editorTitle).getByRole('button', { name: '驳回' })
      ).toHaveCount(0)
    })

    await test.step('审核员通过即发布，浏览端立即可见', async () => {
      await reviewerPage.goto('/admin/articles')
      await articleRow(reviewerPage, editorTitle).getByRole('button', { name: '通过' }).click()
      await reviewerPage.locator('.el-message-box').getByRole('button', { name: '确认' }).click()
      await expect(
        articleRow(reviewerPage, editorTitle).getByText('已发布', { exact: true })
      ).toBeVisible()

      await anonPage.goto('/')
      await expect(anonPage.getByText(editorTitle, { exact: true })).toBeVisible()
    })

    await test.step('审核员下架，浏览端直连详情返回 404', async () => {
      await articleRow(reviewerPage, editorTitle).getByRole('button', { name: '下架' }).click()
      // 下架弹窗使用 textarea 输入原因（inputType: textarea）
      await reviewerPage.locator('.el-message-box textarea').fill('内容时效已过，下架复核')
      await reviewerPage.locator('.el-message-box').getByRole('button', { name: '下架' }).click()
      await expect(
        articleRow(reviewerPage, editorTitle).getByText('已下架', { exact: true })
      ).toBeVisible()

      await anonPage.goto(`/articles/${article.id}`)
      await expect(anonPage.getByText('页面不存在或已移除')).toBeVisible()
    })

    await test.step('已下架文章可重新提交审核', async () => {
      await editorPage.goto('/admin/articles')
      await articleRow(editorPage, editorTitle).getByRole('button', { name: '提交' }).click()
      await editorPage.locator('.el-message-box').getByRole('button', { name: '确认' }).click()
      await expect(
        articleRow(editorPage, editorTitle).getByText('待审核', { exact: true })
      ).toBeVisible()
    })

    await test.step('驳回空理由被客户端拦截，文章保持待审核', async () => {
      await reviewerPage.goto('/admin/articles')
      await articleRow(reviewerPage, editorTitle).getByRole('button', { name: '驳回' }).click()
      await reviewerPage.getByRole('button', { name: '确认驳回' }).click()
      await expect(
        reviewerPage.locator('.el-message').filter({ hasText: '请填写驳回理由' })
      ).toBeVisible()
      await expect(
        articleRow(reviewerPage, editorTitle).getByText('待审核', { exact: true })
      ).toBeVisible()
      await reviewerPage.getByRole('button', { name: '取消' }).click()
    })

    await test.step('带理由驳回回落草稿并标记已驳回待修改', async () => {
      await articleRow(reviewerPage, editorTitle).getByRole('button', { name: '驳回' }).click()
      await reviewerPage
        .getByPlaceholder('请填写驳回理由（必填，500 字内），作者将看到「已驳回待修改」')
        .fill('数据来源不明，请补充可信引用')
      await reviewerPage.getByRole('button', { name: '确认驳回' }).click()
      await expect(
        articleRow(reviewerPage, editorTitle).getByText('已驳回待修改', { exact: true })
      ).toBeVisible()
    })

    await test.step('编辑修改被驳回的草稿后再次提交', async () => {
      await editorPage.goto('/admin/articles')
      await articleRow(editorPage, editorTitle).getByRole('button', { name: '编辑' }).click()
      await editorPage.getByPlaceholder('文章标题').fill(revisedTitle)
      await editorPage.getByRole('button', { name: '保存草稿' }).click()
      await expect(
        articleRow(editorPage, revisedTitle).getByText('已驳回待修改', { exact: true })
      ).toBeVisible()

      await articleRow(editorPage, revisedTitle).getByRole('button', { name: '提交' }).click()
      await editorPage.locator('.el-message-box').getByRole('button', { name: '确认' }).click()
      await expect(
        articleRow(editorPage, revisedTitle).getByText('待审核', { exact: true })
      ).toBeVisible()
    })
  } finally {
    await Promise.all([editorContext.close(), reviewerContext.close(), anonContext.close()])
  }
})

test('编辑保存被他人抢先更新时提示版本冲突', async ({ browser, request }) => {
  const editorToken = await activatedToken(request, 'editor')
  const adminToken = await activatedToken(request, 'admin')
  const category = await createCategoryApi(request, adminToken, `财经-${Date.now().toString(36)}`)
  const lockTitle = `乐观锁冲突稿-${Date.now().toString(36)}`
  const { id } = await createDraftApi(request, editorToken, category.id, lockTitle)

  const context = await browser.newContext()
  const page = await context.newPage()
  try {
    await loginAtAdmin(page, 'editor', ROLE_PASSWORD.editor)
    const row = articleRow(page, lockTitle)
    await expect(row).toBeVisible()

    // 打开编辑对话框（此时捕获当前版本），随后他人（管理员）抢先更新 +1 版本
    await row.getByRole('button', { name: '编辑' }).click()
    await bumpArticleVersion(request, adminToken, id)

    await page.getByPlaceholder('文章标题').fill(`${lockTitle}-外部改动后保存`)
    await page.getByRole('button', { name: '保存草稿' }).click()
    await expect(page.locator('.el-message')).toContainText(
      'version conflict, article was modified'
    )
  } finally {
    await context.close()
  }
})

test('生命周期契约：提交→审核发布→下架→重提交→驳回', async ({ request }) => {
  const adminToken = await activatedToken(request, 'admin')
  const editorToken = await activatedToken(request, 'editor')
  const reviewerToken = await activatedToken(request, 'reviewer')
  const category = await createCategoryApi(request, adminToken, `法务-${Date.now().toString(36)}`)
  const article = await createDraftApi(
    request,
    editorToken,
    category.id,
    `契约流转稿-${Date.now().toString(36)}`
  )

  await submitApi(request, editorToken, article.id)
  await approveApi(request, reviewerToken, article.id)
  await unpublishApi(request, reviewerToken, article.id, '契约校验')
  await submitApi(request, editorToken, article.id)
  const rejected = await rejectApi(request, reviewerToken, article.id, '格式不合规')
  expect(rejected.status).toBe('draft')
  expect(rejected.rejectReason).toBe('格式不合规')
})
