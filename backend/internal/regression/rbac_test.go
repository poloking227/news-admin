package regression

import (
	"net/http"
	"testing"
)

// TestRBACDenialMatrix asserts every crossed-role call is rejected with 403
// (or 401 for anonymous), walking the exact matrix from the assignment:
// editor cannot approve/reject/unpublish/pin or touch user/audit admin;
// reviewer cannot create/update/soft-delete; operator is denied everywhere;
// anonymous is unauthenticated.
func TestRBACDenialMatrix(t *testing.T) {
	requirePG(t)
	resetForTest(t)

	admin := activatedToken(t, "admin")
	catID := createCategory(t, admin, "文化", "culture")

	createUser(t, admin, "editor_deny", "editor")
	editor := activateUser(t, "editor_deny")
	createUser(t, admin, "reviewer_deny", "reviewer")
	reviewer := activateUser(t, "reviewer_deny")
	if _, err := insertOperator(h.sqlDB); err != nil {
		t.Fatalf("provision operator: %v", err)
	}
	operator := login(t, "operator", "OperatorSeed1!")

	// A concrete draft owned by the editor, used as the resource for the
	// lifecycle-denied endpoints.
	articleID := createDraft(t, editor, catID, "文化随笔")

	t.Run("editor denied publisher actions", func(t *testing.T) {
		denied := []struct {
			method string
			path   string
		}{
			{http.MethodPost, "/api/v1/admin/articles/" + articleID + "/approve"},
			{http.MethodPost, "/api/v1/admin/articles/" + articleID + "/reject"},
			{http.MethodPost, "/api/v1/admin/articles/" + articleID + "/unpublish"},
			{http.MethodPut, "/api/v1/admin/articles/" + articleID + "/pin"},
			{http.MethodGet, "/api/v1/admin/users"},
			{http.MethodPost, "/api/v1/admin/users"},
			{http.MethodGet, "/api/v1/admin/audit-logs"},
		}
		for _, d := range denied {
			r := doJSON(d.method, d.path, editor, nil)
			if r.status != http.StatusForbidden {
				t.Errorf("editor %s %s = %d, want 403 (body=%s)", d.method, d.path, r.status, r.body)
			}
		}
	})

	t.Run("reviewer denied authoring actions", func(t *testing.T) {
		denied := []struct {
			method string
			path   string
		}{
			{http.MethodPost, "/api/v1/admin/articles"},
			{http.MethodPut, "/api/v1/admin/articles/" + articleID},
			{http.MethodDelete, "/api/v1/admin/articles/" + articleID},
		}
		for _, d := range denied {
			r := doJSON(d.method, d.path, reviewer, nil)
			if r.status != http.StatusForbidden {
				t.Errorf("reviewer %s %s = %d, want 403 (body=%s)", d.method, d.path, r.status, r.body)
			}
		}
	})

	t.Run("operator denied every admin endpoint", func(t *testing.T) {
		denied := []struct {
			method string
			path   string
		}{
			{http.MethodGet, "/api/v1/admin/articles"},
			{http.MethodPost, "/api/v1/admin/articles"},
			{http.MethodGet, "/api/v1/admin/categories"},
			{http.MethodPost, "/api/v1/admin/categories"},
			{http.MethodGet, "/api/v1/admin/users"},
			{http.MethodGet, "/api/v1/admin/audit-logs"},
		}
		for _, d := range denied {
			r := doJSON(d.method, d.path, operator, nil)
			if r.status != http.StatusForbidden {
				t.Errorf("operator %s %s = %d, want 403 (body=%s)", d.method, d.path, r.status, r.body)
			}
		}
	})

	t.Run("anonymous unauthenticated on admin", func(t *testing.T) {
		for _, path := range []string{
			"/api/v1/admin/articles",
			"/api/v1/admin/categories",
			"/api/v1/admin/users",
			"/api/v1/admin/audit-logs",
		} {
			r := doJSON(http.MethodGet, path, "", nil)
			if r.status != http.StatusUnauthorized {
				t.Errorf("anonymous GET %s = %d, want 401 (body=%s)", path, r.status, r.body)
			}
		}
	})

	t.Run("granted paths stay reachable", func(t *testing.T) {
		// The denial matrix must not over-block: editor can author,
		// reviewer can approve once the article is in review.
		sub := doJSON(http.MethodPost, "/api/v1/admin/articles/"+articleID+"/submit", editor, nil)
		if sub.status != http.StatusOK {
			t.Fatalf("editor submit = %d, want 200 (body=%s)", sub.status, sub.body)
		}
		app := doJSON(http.MethodPost, "/api/v1/admin/articles/"+articleID+"/approve", reviewer, nil)
		if app.status != http.StatusOK {
			t.Errorf("reviewer approve = %d, want 200 (body=%s)", app.status, app.body)
		}
	})
}
