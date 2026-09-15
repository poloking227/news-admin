package service

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

// LoginLimiter enforces the contract's failed-login limits:
// 5 attempts/min/IP and 10 attempts/15 min/account. Windows are fixed and
// reset when the recorded window expires.
type LoginLimiter struct {
	mu    sync.Mutex
	ip    map[string]*window
	user  map[string]*window
	now   func() time.Time
	limit ipLimit
}

type window struct {
	start time.Time
	count int
}

type ipLimit struct {
	ipPerMinute   int
	ipWindow      time.Duration
	userPerWindow int
	userWindow    time.Duration
}

func newLoginLimiter() *LoginLimiter {
	return &LoginLimiter{
		ip:   make(map[string]*window),
		user: make(map[string]*window),
		now:  time.Now,
		limit: ipLimit{
			ipPerMinute:   5,
			ipWindow:      time.Minute,
			userPerWindow: 10,
			userWindow:    15 * time.Minute,
		},
	}
}

// IsBlockedIP reports whether the IP has exceeded its per-minute budget.
func (l *LoginLimiter) IsBlockedIP(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.exceeded(l.ip, ip, l.limit.ipPerMinute, l.limit.ipWindow)
}

// IsBlockedUser reports whether the account has exceeded its 15-minute budget.
func (l *LoginLimiter) IsBlockedUser(username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.exceeded(l.user, username, l.limit.userPerWindow, l.limit.userWindow)
}

// RecordFailure increments both counters for a failed attempt.
func (l *LoginLimiter) RecordFailure(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.bump(l.ip, ip, l.limit.ipWindow)
	l.bump(l.user, username, l.limit.userWindow)
}

// ResetUser clears the account counter after a successful login.
func (l *LoginLimiter) ResetUser(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.user, username)
}

func (l *LoginLimiter) exceeded(m map[string]*window, key string, max int, win time.Duration) bool {
	w, ok := m[key]
	if !ok {
		return false
	}
	if l.now().Sub(w.start) >= win {
		delete(m, key)
		return false
	}
	return w.count >= max
}

func (l *LoginLimiter) bump(m map[string]*window, key string, win time.Duration) {
	now := l.now()
	if w, ok := m[key]; ok && now.Sub(w.start) < win {
		w.count++
		return
	}
	m[key] = &window{start: now, count: 1}
}

// newUUID returns a random v4-style UUID string for row primary keys.
func newUUID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("uuid: %v", err))
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}
