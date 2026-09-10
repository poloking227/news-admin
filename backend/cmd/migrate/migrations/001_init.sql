-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username             CITEXT NOT NULL,
    password_hash        TEXT NOT NULL,
    display_name         TEXT NOT NULL,
    role                 TEXT NOT NULL CHECK (role IN ('admin', 'editor', 'reviewer', 'operator')),
    status               TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
    password_changed_at  TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at           TIMESTAMPTZ
);

CREATE TABLE categories (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    sort_order  INT  NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);

CREATE TABLE articles (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title          TEXT NOT NULL CHECK (char_length(title) <= 200),
    summary        TEXT NOT NULL DEFAULT '' CHECK (char_length(summary) <= 500),
    body_html      TEXT NOT NULL,
    body_text      TEXT NOT NULL DEFAULT '',
    category_id    UUID NOT NULL REFERENCES categories(id),
    cover_url      TEXT CHECK (cover_url IS NULL OR cover_url ~ '^https?://'),
    status         TEXT NOT NULL DEFAULT 'draft'
                   CHECK (status IN ('draft', 'pending_review', 'published', 'unpublished')),
    reject_reason  TEXT CHECK (reject_reason IS NULL OR char_length(reject_reason) <= 500),
    rejected_at    TIMESTAMPTZ,
    pinned         BOOLEAN NOT NULL DEFAULT FALSE,
    pinned_at      TIMESTAMPTZ,
    submitted_at   TIMESTAMPTZ,
    published_at   TIMESTAMPTZ,
    unpublished_at TIMESTAMPTZ,
    created_by     UUID NOT NULL REFERENCES users(id),
    updated_by     UUID NOT NULL REFERENCES users(id),
    version        INT NOT NULL DEFAULT 1,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ
);

CREATE TABLE audit_logs (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor         UUID NOT NULL REFERENCES users(id),
    action        TEXT NOT NULL CHECK (action IN (
        'login', 'failed_login', 'logout',
        'article_create', 'article_update', 'article_soft_delete',
        'article_submit', 'article_approve', 'article_reject',
        'article_unpublish', 'article_pin',
        'user_create', 'user_update', 'user_disable',
        'user_reset_password', 'user_password_change',
        'category_create', 'category_update', 'category_soft_delete'
    )),
    resource_type TEXT NOT NULL,
    resource_id   TEXT,
    before        JSONB,
    after         JSONB,
    ip            TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE refresh_sessions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id),
    jti        TEXT NOT NULL,
    family_id  TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 唯一性仅对未软删行生效，软删后允许复用用户名/别名。
CREATE UNIQUE INDEX uq_users_username ON users (username) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_categories_name ON categories (name) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_categories_slug ON categories (slug) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_refresh_jti ON refresh_sessions (jti);

-- 公共查询路径（published + 未软删）常用列。
CREATE INDEX idx_articles_published ON articles (published_at DESC) WHERE status = 'published' AND deleted_at IS NULL;
CREATE INDEX idx_articles_category ON articles (category_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_articles_created_by ON articles (created_by) WHERE deleted_at IS NULL;
CREATE INDEX idx_articles_pinned ON articles (pinned, pinned_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_articles_body_text_trgm ON articles USING GIN (body_text gin_trgm_ops);
CREATE INDEX idx_articles_title_trgm ON articles USING GIN (title gin_trgm_ops);
CREATE INDEX idx_articles_summary_trgm ON articles USING GIN (summary gin_trgm_ops);
CREATE INDEX idx_categories_sort ON categories (sort_order) WHERE deleted_at IS NULL;
CREATE INDEX idx_audit_actor ON audit_logs (actor, created_at DESC);
CREATE INDEX idx_audit_resource ON audit_logs (resource_type, resource_id);
CREATE INDEX idx_refresh_user ON refresh_sessions (user_id, expires_at);

-- 状态机约束：仅 5 条合法迁移，非法迁移由数据库拒绝（与 API 层双重校验）。
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION check_article_status_transition()
RETURNS trigger AS $$
BEGIN
    IF OLD.status = NEW.status THEN
        RETURN NEW;
    END IF;
    IF NOT (
        (OLD.status = 'draft' AND NEW.status = 'pending_review')
        OR (OLD.status = 'pending_review' AND NEW.status = 'published')
        OR (OLD.status = 'pending_review' AND NEW.status = 'draft')
        OR (OLD.status = 'published' AND NEW.status = 'unpublished')
        OR (OLD.status = 'unpublished' AND NEW.status = 'pending_review')
    ) THEN
        RAISE EXCEPTION 'illegal article status transition: % -> %', OLD.status, NEW.status
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_articles_status_before_update
    BEFORE UPDATE OF status ON articles
    FOR EACH ROW EXECUTE FUNCTION check_article_status_transition();

-- +goose Down
DROP TRIGGER IF EXISTS trg_articles_status_before_update ON articles;
DROP FUNCTION IF EXISTS check_article_status_transition();

DROP TABLE IF EXISTS refresh_sessions;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS articles;
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS users;

DROP EXTENSION IF EXISTS citext;
DROP EXTENSION IF EXISTS pg_trgm;