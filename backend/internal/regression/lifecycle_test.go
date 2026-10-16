package regression

import (
	"net/http"
	"testing"
)

// TestArticleLifecycleHappyPath walks the full article workflow against the
// real database: register via admin-created user, first-login forced password
// change (M0), draft creation, submit, approve (approval publishes),
// unpublish, re-submit, reject with reason, and re-submit of the rejected
// draft. It also asserts optimistic-lock version bumps and the audit action
// sequence written along the way.
func TestArticleLifecycleHappyPath(t *testing.T) {
	requirePG(t)
	resetForTest(t)

	// M0: the seeded admin must change the initial password before any
	// business endpoint works.
	adminFirst := login(t, "admin", seedPassword)
	me := doJSON(http.MethodGet, "/api/v1/auth/me", adminFirst, nil)
	if me.status != http.StatusOK {
		t.Fatalf("admin /auth/me status = %d", me.status)
	}
	meBody := decode[struct {
		MustChangePassword bool `json:"mustChangePassword"`
	}](t, me)
	if !meBody.MustChangePassword {
		t.Fatal("admin first login: mustChangePassword should be true")
	}
	blocked := doJSON(http.MethodGet, "/api/v1/admin/categories", adminFirst, nil)
	if blocked.status != http.StatusForbidden {
		t.Errorf("admin business endpoint before change: status = %d, want 403", blocked.status)
	}

	admin := activatedToken(t, "admin")

	// Admin prepares the taxonomy and an editor account.
	catID := createCategory(t, admin, "技术", "tech")
	createUser(t, admin, "editor_full", "editor")
	editor := activateUser(t, "editor_full")

	// Editor drafts an article.
	articleID := createDraft(t, editor, catID, "Go 并发模型")

	// Optimistic lock: update with the current version bumps to v2, and a
	// stale If-Match is rejected.
	upd := doJSONHeaders(http.MethodPut, "/api/v1/admin/articles/"+articleID, h.router, editor,
		map[string]string{"If-Match": "1"},
		map[string]any{
			"title": "Go 并发模型（修订）", "summary": "回归摘要", "bodyHtml": "<p>b</p>", "categoryId": catID,
		})
	if upd.status != http.StatusOK {
		t.Fatalf("update draft status = %d, body=%s", upd.status, upd.body)
	}
	stale := doJSONHeaders(http.MethodPut, "/api/v1/admin/articles/"+articleID, h.router, editor,
		map[string]string{"If-Match": "1"},
		map[string]any{
			"title": "Go 并发模型", "summary": "回归摘要", "bodyHtml": "<p>b</p>", "categoryId": catID,
		})
	if stale.status != http.StatusConflict {
		t.Errorf("stale If-Match status = %d, want 409", stale.status)
	}

	// submit → pending_review
	sub := doJSON(http.MethodPost, "/api/v1/admin/articles/"+articleID+"/submit", editor, nil)
	wantStatus(t, sub, http.StatusOK, "submit")
	if got := decode[articlePayload](t, sub); got.Status != "pending_review" {
		t.Errorf("after submit status = %q", got.Status)
	}

	// approve → published (approval publishes)
	app := doJSON(http.MethodPost, "/api/v1/admin/articles/"+articleID+"/approve", admin, nil)
	wantStatus(t, app, http.StatusOK, "approve")
	appArt := decode[articlePayload](t, app)
	if appArt.Status != "published" || appArt.PublishedAt == nil {
		t.Errorf("after approve: status=%q publishedAt=%v, want published + stamp", appArt.Status, appArt.PublishedAt)
	}

	// unpublish → unpublished
	unpub := doJSON(http.MethodPost, "/api/v1/admin/articles/"+articleID+"/unpublish", admin, map[string]any{"reason": "下架整改"})
	wantStatus(t, unpub, http.StatusOK, "unpublish")
	if got := decode[articlePayload](t, unpub); got.Status != "unpublished" {
		t.Errorf("after unpublish status = %q", got.Status)
	}

	// re-submit → pending_review
	resub := doJSON(http.MethodPost, "/api/v1/admin/articles/"+articleID+"/submit", editor, nil)
	wantStatus(t, resub, http.StatusOK, "re-submit")
	if got := decode[articlePayload](t, resub); got.Status != "pending_review" {
		t.Errorf("after re-submit status = %q", got.Status)
	}

	// reject without a reason → 400
	badReject := doJSON(http.MethodPost, "/api/v1/admin/articles/"+articleID+"/reject", admin, map[string]any{"reason": ""})
	if badReject.status != http.StatusBadRequest {
		t.Errorf("reject w/o reason status = %d, want 400", badReject.status)
	}

	// reject with reason → draft + rejectReason/rejectedAt
	rej := doJSON(http.MethodPost, "/api/v1/admin/articles/"+articleID+"/reject", admin, map[string]any{"reason": "标题不规范"})
	wantStatus(t, rej, http.StatusOK, "reject")
	rejArt := decode[articlePayload](t, rej)
	if rejArt.Status != "draft" || rejArt.RejectReason == nil || *rejArt.RejectReason != "标题不规范" {
		t.Errorf("after reject: status=%q reason=%v, want draft + set reason", rejArt.Status, rejArt.RejectReason)
	}

	// rejected draft may be submitted again
	resub2 := doJSON(http.MethodPost, "/api/v1/admin/articles/"+articleID+"/submit", editor, nil)
	wantStatus(t, resub2, http.StatusOK, "re-submit rejected draft")
	if got := decode[articlePayload](t, resub2); got.Status != "pending_review" {
		t.Errorf("after rejection re-submit status = %q", got.Status)
	}

	// optimistic lock: the version must have advanced monotonically with
	// every guarded write (1 create + 2 updates + transitions).
	art := getAdminArticle(t, admin, articleID)
	if art.Version < 8 {
		t.Errorf("final version = %d, want >= 8 after full workflow", art.Version)
	}

	// audit sequence: article_create → update → submit → approve →
	// unpublish → submit → reject → submit, newest-first from the API.
	audit := getAuditActions(t, admin, "article", articleID)
	want := []string{
		"article_create", "article_update", "article_submit",
		"article_approve", "article_unpublish", "article_submit",
		"article_reject", "article_submit",
	}
	if len(audit) < len(want) {
		t.Fatalf("audit actions = %v, want at least %v", audit, want)
	}
	// The listing returns newest first, so walk it backwards.
	for i := len(want) - 1; i >= 0; i-- {
		idx := len(want) - 1 - i
		if idx >= len(audit) {
			t.Errorf("audit actions too short: got %v want %v", audit, want)
			break
		}
		if audit[idx] != want[i] {
			t.Errorf("audit sequence mismatch: got %v want %v", audit, want)
			break
		}
	}
}

// TestArticleLifecycleRejectRequiresReason guards the mandatory-reason rule
// separately from the happy path.
func TestArticleLifecycleRejectRequiresReason(t *testing.T) {
	requirePG(t)
	resetForTest(t)

	admin := activatedToken(t, "admin")
	catID := createCategory(t, admin, "商业", "biz")
	createUser(t, admin, "editor_reject", "editor")
	editor := activateUser(t, "editor_reject")
	articleID := createDraft(t, editor, catID, "商业观察")

	sub := doJSON(http.MethodPost, "/api/v1/admin/articles/"+articleID+"/submit", editor, nil)
	wantStatus(t, sub, http.StatusOK, "submit")

	r := doJSON(http.MethodPost, "/api/v1/admin/articles/"+articleID+"/reject", admin, map[string]string{"reason": ""})
	if r.status != http.StatusBadRequest {
		t.Errorf("empty reason reject status = %d, want 400", r.status)
	}
	// The article must still be pending_review after the failed reject.
	art := getAdminArticle(t, admin, articleID)
	if art.Status != "pending_review" {
		t.Errorf("article after failed reject = %q, want pending_review", art.Status)
	}
}

// ArticleDetail keeps the admin Article DTO projection for assertions.
type ArticleDetail struct {
	ID           string  `json:"id"`
	Version      int     `json:"version"`
	Status       string  `json:"status"`
	RejectReason *string `json:"rejectReason"`
	PublishedAt  *string `json:"publishedAt"`
}

func getAdminArticle(t *testing.T, token, id string) ArticleDetail {
	t.Helper()
	r := doJSON(http.MethodGet, "/api/v1/admin/articles/"+id, token, nil)
	if r.status != http.StatusOK {
		t.Fatalf("get admin article %s status = %d, body=%s", id, r.status, r.body)
	}
	return decode[ArticleDetail](t, r)
}

type auditPage struct {
	Items []struct {
		Action string `json:"action"`
	} `json:"items"`
}

func getAuditActions(t *testing.T, token, resourceType, resourceID string) []string {
	t.Helper()
	r := doJSON(http.MethodGet,
		"/api/v1/admin/audit-logs?resourceType="+resourceType+"&resourceId="+resourceID+"&pageSize=100",
		token, nil)
	if r.status != http.StatusOK {
		t.Fatalf("audit-logs status = %d, body=%s", r.status, r.body)
	}
	page := decode[auditPage](t, r)
	out := make([]string, 0, len(page.Items))
	for _, it := range page.Items {
		out = append(out, it.Action)
	}
	return out
}

func wantStatus(t *testing.T, r resp, want int, what string) {
	t.Helper()
	if r.status != want {
		t.Errorf("%s status = %d, want %d, body=%s", what, r.status, want, r.body)
	}
}
