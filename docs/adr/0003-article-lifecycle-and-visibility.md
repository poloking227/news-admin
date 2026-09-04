# ADR-0003 内容生命周期与可见性

- 状态：已接受
- 日期：2026-09-01

## 背景

内容治理要求「编辑与审核职责分离、审核通过即发布」；草稿、待审核、已下架、已删除内容在任何入口都不可对访客可见，且不暴露其存在性。

## 决策

- 状态枚举：`draft | pending_review | published | unpublished`，不含独立 rejected 状态；驳回 = draft + `reject_reason`/`rejected_at` 非空（前端显示「已驳回待修改」）。
- 仅 5 条合法迁移：draft→pending_review；pending_review→published｜draft（驳回附理由必填）；published→unpublished（下架，理由可选）；unpublished→pending_review（重新提交审核，无直接重发）。API 层与数据库 CHECK 约束双重拒绝非法迁移（409）。
- 软删：所有业务表 `deleted_at`；已发布禁止直接删；删除仅置标记 + 审计。
- 可见性：公共侧所有读路径（列表/搜索/详情/分类）强制 `status='published' AND deleted_at IS NULL`，详情对隐藏内容一律 404；管理端按角色过滤（editor 可见自有全部非删除内容 + 全部已发布）。

## 影响

- 「下架后重新上架必须重新审核」保证任何回到公开态的路径都经过审核。
- 隐藏内容不可见由唯一权威（后端查询 scope）保证，前端不持有任何可泄露的状态开关。