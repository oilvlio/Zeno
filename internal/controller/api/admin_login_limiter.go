package api

import (
	"sync"
	"time"
)

type adminLoginLimiter struct {
	mu       sync.Mutex
	attempts map[string]adminLoginAttempt
}

type adminLoginAttempt struct {
	Count       int
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	LockedUntil time.Time
}

type adminLoginReservation struct {
	limiter *adminLoginLimiter
	key     string
}

const (
	adminLoginWindow       = 15 * time.Minute
	adminLoginLockDuration = 10 * time.Minute
	adminLoginMaxFailures  = 5
	adminLoginMaxEntries   = 4096
)

func newAdminLoginLimiter() *adminLoginLimiter {
	return &adminLoginLimiter{attempts: map[string]adminLoginAttempt{}}
}

func (limiter *adminLoginLimiter) reserve(key string) (adminLoginReservation, bool) {
	now := time.Now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	limiter.pruneLocked(now)
	attempt := limiter.attempts[key]
	if !attempt.LockedUntil.IsZero() && now.Before(attempt.LockedUntil) {
		return adminLoginReservation{}, false
	}
	if attempt.FirstSeenAt.IsZero() || now.Sub(attempt.FirstSeenAt) > adminLoginWindow {
		attempt = adminLoginAttempt{FirstSeenAt: now}
	}
	attempt.Count++
	attempt.LastSeenAt = now
	if attempt.Count >= adminLoginMaxFailures {
		attempt.LockedUntil = now.Add(adminLoginLockDuration)
	}
	limiter.attempts[key] = attempt
	return adminLoginReservation{limiter: limiter, key: key}, true
}

func (reservation adminLoginReservation) release(success bool) {
	if reservation.limiter == nil || reservation.key == "" || !success {
		return
	}
	reservation.limiter.recordSuccess(reservation.key)
}

func (reservation adminLoginReservation) cancel() {
	if reservation.limiter == nil || reservation.key == "" {
		return
	}
	reservation.limiter.cancel(reservation.key)
}

func (limiter *adminLoginLimiter) pruneLocked(now time.Time) {
	oldestKey := ""
	oldestSeen := now
	for key, attempt := range limiter.attempts {
		lastSeen := attempt.LastSeenAt
		if lastSeen.IsZero() {
			lastSeen = attempt.FirstSeenAt
		}
		if (!attempt.LockedUntil.IsZero() && now.After(attempt.LockedUntil.Add(adminLoginWindow))) || (attempt.LockedUntil.IsZero() && !attempt.FirstSeenAt.IsZero() && now.Sub(attempt.FirstSeenAt) > adminLoginWindow) {
			delete(limiter.attempts, key)
			continue
		}
		if oldestKey == "" || lastSeen.Before(oldestSeen) {
			oldestKey = key
			oldestSeen = lastSeen
		}
	}
	for len(limiter.attempts) > adminLoginMaxEntries && oldestKey != "" {
		delete(limiter.attempts, oldestKey)
		oldestKey = ""
		oldestSeen = now
		for key, attempt := range limiter.attempts {
			lastSeen := attempt.LastSeenAt
			if lastSeen.IsZero() {
				lastSeen = attempt.FirstSeenAt
			}
			if oldestKey == "" || lastSeen.Before(oldestSeen) {
				oldestKey = key
				oldestSeen = lastSeen
			}
		}
	}
}

func (limiter *adminLoginLimiter) recordSuccess(key string) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	delete(limiter.attempts, key)
}

func (limiter *adminLoginLimiter) cancel(key string) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	attempt, ok := limiter.attempts[key]
	if !ok {
		return
	}
	if attempt.Count <= 1 {
		delete(limiter.attempts, key)
		return
	}
	attempt.Count--
	if attempt.Count < adminLoginMaxFailures {
		attempt.LockedUntil = time.Time{}
	}
	limiter.attempts[key] = attempt
}
