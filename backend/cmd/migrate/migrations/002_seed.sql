-- +goose Up
-- 预置三个内置账号（admin/editor/reviewer）。口令占位为不可登录哨兵值，
-- 真实口令由 seed 命令从环境变量注入后覆盖（本迁移不内嵌任何明文口令）。
-- 全部置 must_change_password=true，首登强制改密（M0）。
INSERT INTO users (username, password_hash, display_name, role, status, must_change_password)
VALUES
    ('admin',    'seed-pending', '系统管理员', 'admin',    'active', TRUE),
    ('editor',   'seed-pending', '内容编辑',   'editor',   'active', TRUE),
    ('reviewer', 'seed-pending', '审核员',     'reviewer', 'active', TRUE)
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM users WHERE username IN ('admin', 'editor', 'reviewer');