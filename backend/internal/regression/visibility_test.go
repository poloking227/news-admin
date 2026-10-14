package regression

import (
	"net/http"
	"net/url"
	"testing"
)

// TestPublicHiddenContentInvisible proves draft, pending_review, unpublished,
// and soft-deleted articles never leak to the public reader: list, direct
// fetch, and search all exclude them, published content stays reachable
// through every entry point, and the public category list only surfaces
// categories that carry published articles.
func TestPublicHiddenContentInvisible(t *testing.T) {
	requirePG(t)
	resetForTest(t)

	admin := activatedToken(t, "admin")
	createUser(t, admin, "editor_vis", "editor")
	editor := activateUser(t, "editor_vis")

	catVis := createCategory(t, admin, "科技", "vis-tech")
	catBiz := createCategory(t, admin, "商业", "vis-biz")
	createCategory(t, admin, "空分类", "vis-empty")

	// published article on catVis
	pub1 := createDraft(t, editor, catVis, "可见文章甲")
	sub1 := doJSON(http.MethodPost, "/api/v1/admin/articles/"+pub1+"/submit", editor, nil)
	wantStatus(t, sub1, http.StatusOK, "submit pub1")
	app1 := doJSON(http.MethodPost, "/api/v1/admin/articles/"+pub1+"/approve", admin, nil)
	wantStatus(t, app1, http.StatusOK, "approve pub1")

	// published article on catBiz
	pub2 := createDraft(t, editor, catBiz, "可见文章乙")
	sub2 := doJSON(http.MethodPost, "/api/v1/admin/articles/"+pub2+"/submit", editor, nil)
	wantStatus(t, sub2, http.StatusOK, "submit pub2")
	app2 := doJSON(http.MethodPost, "/api/v1/admin/articles/"+pub2+"/approve", admin, nil)
	wantStatus(t, app2, http.StatusOK, "approve pub2")

	// hidden: draft, pending_review, unpublished, soft-deleted
	hiddenDraft := createDraft(t, editor, catVis, "隐藏草稿")
	hiddenPending := createDraft(t, editor, catBiz, "隐藏待审")
	sub3 := doJSON(http.MethodPost, "/api/v1/admin/articles/"+hiddenPending+"/submit", editor, nil)
	wantStatus(t, sub3, http.StatusOK, "submit hiddenPending")
	hiddenUnpub := createDraft(t, editor, catVis, "已下架")
	sub4 := doJSON(http.MethodPost, "/api/v1/admin/articles/"+hiddenUnpub+"/submit", editor, nil)
	wantStatus(t, sub4, http.StatusOK, "submit hiddenUnpub")
	app4 := doJSON(http.MethodPost, "/api/v1/admin/articles/"+hiddenUnpub+"/approve", admin, nil)
	wantStatus(t, app4, http.StatusOK, "approve hiddenUnpub")
	unpub4 := doJSON(http.MethodPost, "/api/v1/admin/articles/"+hiddenUnpub+"/unpublish", admin, map[string]any{"reason": "下架"})
	wantStatus(t, unpub4, http.StatusOK, "unpublish hiddenUnpub")
	hiddenDeleted := createDraft(t, editor, catBiz, "已删除文章")
	del := doJSON(http.MethodDelete, "/api/v1/admin/articles/"+hiddenDeleted, editor, nil)
	if del.status != http.StatusNoContent {
		t.Fatalf("soft delete = %d, want 204 (body=%s)", del.status, del.body)
	}

	t.Run("public list only published", func(t *testing.T) {
		r := doJSON(http.MethodGet, "/api/v1/public/articles?pageSize=100", "", nil)
		if r.status != http.StatusOK {
			t.Fatalf("public list = %d (body=%s)", r.status, r.body)
		}
		page := decode[struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
			Total int `json:"total"`
		}](t, r)
		if page.Total != 2 {
			t.Errorf("public total = %d, want 2", page.Total)
		}
		ids := map[string]bool{}
		for _, it := range page.Items {
			ids[it.ID] = true
		}
		if !ids[pub1] || !ids[pub2] {
			t.Errorf("public list missing published articles: %v", ids)
		}
		hidden := map[string]bool{hiddenDraft: true, hiddenPending: true, hiddenUnpub: true, hiddenDeleted: true}
		for id := range hidden {
			if ids[id] {
				t.Errorf("public list leaked hidden article %s", id)
			}
		}
	})

	t.Run("public direct fetch 404 on hidden", func(t *testing.T) {
		for _, id := range []string{pub1, pub2} {
			r := doJSON(http.MethodGet, "/api/v1/public/articles/"+id, "", nil)
			if r.status != http.StatusOK {
				t.Errorf("public direct %s = %d, want 200", id, r.status)
			}
		}
		for _, id := range []string{hiddenDraft, hiddenPending, hiddenUnpub, hiddenDeleted} {
			r := doJSON(http.MethodGet, "/api/v1/public/articles/"+id, "", nil)
			if r.status != http.StatusNotFound {
				t.Errorf("public direct hidden %s = %d, want 404", id, r.status)
			}
		}
		r := doJSON(http.MethodGet, "/api/v1/public/articles/00000000-0000-0000-0000-000000000000", "", nil)
		if r.status != http.StatusNotFound {
			t.Errorf("public direct unknown = %d, want 404", r.status)
		}
	})

	t.Run("public search filters hidden", func(t *testing.T) {
		// published title hits
		r := doJSON(http.MethodGet, "/api/v1/public/search?q="+urlQuery(pubTitle(t, admin, pub1)), "", nil)
		if r.status != http.StatusOK {
			t.Fatalf("search published = %d (body=%s)", r.status, r.body)
		}
		total := decode[struct {
			Total int `json:"total"`
		}](t, r).Total
		if total != 1 {
			t.Errorf("search published total = %d, want 1", total)
		}
		// hidden titles never hit
		for _, title := range []string{"隐藏草稿", "隐藏待审", "已下架", "已删除文章"} {
			r := doJSON(http.MethodGet, "/api/v1/public/search?q="+urlQuery(title), "", nil)
			if r.status != http.StatusOK {
				t.Fatalf("search %q = %d (body=%s)", title, r.status, r.body)
			}
			if total := decode[struct {
				Total int `json:"total"`
			}](t, r).Total; total != 0 {
				t.Errorf("search %q total = %d, want 0", title, total)
			}
		}
		// empty keyword rejected
		r = doJSON(http.MethodGet, "/api/v1/public/search?q=", "", nil)
		if r.status != http.StatusBadRequest {
			t.Errorf("search empty q = %d, want 400", r.status)
		}
	})

	t.Run("public categories published-only", func(t *testing.T) {
		r := doJSON(http.MethodGet, "/api/v1/public/categories", "", nil)
		if r.status != http.StatusOK {
			t.Fatalf("public categories = %d (body=%s)", r.status, r.body)
		}
		cats := decode[[]struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Count int64  `json:"articleCount"`
		}](t, r)
		names := map[string]int64{}
		for _, c := range cats {
			names[c.Name] = c.Count
		}
		if _, ok := names["科技"]; !ok {
			t.Errorf("public categories missing 科技 (has published): %v", names)
		}
		if _, ok := names["商业"]; !ok {
			t.Errorf("public categories missing 商业 (has published): %v", names)
		}
		if _, ok := names["空分类"]; ok {
			t.Errorf("public categories leaked 空分类 (no published articles): %v", names)
		}
		for name, count := range names {
			if count <= 0 {
				t.Errorf("public category %s articleCount = %d, want > 0", name, count)
			}
		}
	})

	t.Run("admin list still sees hidden states", func(t *testing.T) {
		r := doJSON(http.MethodGet, "/api/v1/admin/articles?pageSize=100", admin, nil)
		if r.status != http.StatusOK {
			t.Fatalf("admin list = %d (body=%s)", r.status, r.body)
		}
		page := decode[struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}](t, r)
		ids := map[string]bool{}
		for _, it := range page.Items {
			ids[it.ID] = true
		}
		for _, id := range []string{pub1, pub2, hiddenDraft, hiddenPending, hiddenUnpub} {
			if !ids[id] {
				t.Errorf("admin list missing article %s (should see draft/pending/unpublished)", id)
			}
		}
		if ids[hiddenDeleted] {
			t.Error("admin list returned soft-deleted article")
		}
	})
}

// urlQuery percent-encodes a search keyword for the query string.
func urlQuery(s string) string {
	return url.QueryEscape(s)
}

// pubTitle fetches the article title via the admin API for the search probe.
func pubTitle(t *testing.T, token, id string) string {
	t.Helper()
	r := doJSON(http.MethodGet, "/api/v1/admin/articles/"+id, token, nil)
	if r.status != http.StatusOK {
		t.Fatalf("admin get %s = %d (body=%s)", id, r.status, r.body)
	}
	return decode[struct {
		Title string `json:"title"`
	}](t, r).Title
}
