// Package service_test contains unit tests for the auth use-cases using
// in-memory fake repositories.
package service_test

import (
	"context"
	"sync"
	"time"

	"news-admin/backend/internal/domain"
)

// fakeUserRepo is an in-memory domain.UserRepository.
type fakeUserRepo struct {
	mu    sync.Mutex
	users map[string]*domain.User // by username
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{users: make(map[string]*domain.User)}
}

func (f *fakeUserRepo) FindByUsername(_ context.Context, username string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[username]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (f *fakeUserRepo) FindByID(_ context.Context, id string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.ID == id {
			cp := *u
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeUserRepo) UpdatePassword(_ context.Context, id, passwordHash string, changedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.ID == id {
			u.PasswordHash = passwordHash
			u.MustChangePassword = false
			u.PasswordChangedAt = &changedAt
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeUserRepo) upsert(u *domain.User) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[u.Username] = u
}

// fakeSessionRepo is an in-memory domain.SessionRepository.
type fakeSessionRepo struct {
	mu       sync.Mutex
	sessions []*domain.RefreshSession
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{}
}

func (f *fakeSessionRepo) Insert(_ context.Context, s *domain.RefreshSession) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *s
	f.sessions = append(f.sessions, &cp)
	return nil
}

func (f *fakeSessionRepo) FindByJTI(_ context.Context, jti string) (*domain.RefreshSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.sessions {
		if s.JTI == jti {
			cp := *s
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeSessionRepo) Revoke(_ context.Context, jti string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for _, s := range f.sessions {
		if s.JTI == jti && s.RevokedAt == nil {
			s.RevokedAt = &now
		}
	}
	return nil
}

func (f *fakeSessionRepo) RevokeFamily(_ context.Context, familyID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for _, s := range f.sessions {
		if s.FamilyID == familyID && s.RevokedAt == nil {
			s.RevokedAt = &now
		}
	}
	return nil
}

func (f *fakeSessionRepo) RevokeAllByUser(_ context.Context, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for _, s := range f.sessions {
		if s.UserID == userID && s.RevokedAt == nil {
			s.RevokedAt = &now
		}
	}
	return nil
}

// fakeAuditRepo records audit entries.
type fakeAuditRepo struct {
	mu      sync.Mutex
	entries []*domain.AuditLog
}

func newFakeAuditRepo() *fakeAuditRepo {
	return &fakeAuditRepo{}
}

func (f *fakeAuditRepo) Insert(_ context.Context, entry *domain.AuditLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *entry
	f.entries = append(f.entries, &cp)
	return nil
}

func (f *fakeAuditRepo) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.entries))
	for _, e := range f.entries {
		out = append(out, e.Action)
	}
	return out
}
