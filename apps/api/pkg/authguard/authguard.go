// Package authguard provides Redis-backed login-attempt lockout and token
// revocation. It is deliberately fail-open on Redis errors (a Redis outage must
// not lock every user out or reject every token) but fail-safe by default when
// no Redis client is configured: lockout and revocation simply become no-ops,
// so a single-process deployment without Redis still runs.
package authguard

import (
	"context"
	"time"

	"veemon-common/infra/redis"
)

// Guard enforces account lockout and token revocation.
type Guard struct {
	redis       *redis.Client
	maxAttempts int
	lockout     time.Duration
}

// New builds a Guard. A nil redis client yields a no-op guard.
func New(r *redis.Client, maxAttempts, lockoutMinutes int) *Guard {
	return &Guard{
		redis:       r,
		maxAttempts: maxAttempts,
		lockout:     time.Duration(lockoutMinutes) * time.Minute,
	}
}

func (g *Guard) enabled() bool { return g != nil && g.redis != nil }

func lockKey(email string) string  { return "login:lock:" + email }
func failKey(email string) string  { return "login:fail:" + email }
func revokedKey(jti string) string { return "token:revoked:" + jti }

// IsLocked reports whether the account is currently locked out. On Redis error
// it returns false (fail open) so an outage cannot lock everyone out.
func (g *Guard) IsLocked(ctx context.Context, email string) bool {
	if !g.enabled() {
		return false
	}
	locked, err := g.redis.Exists(ctx, lockKey(email))
	if err != nil {
		return false
	}
	return locked
}

// RecordFailure increments the failure counter and locks the account once the
// configured threshold is reached, both expiring after the lockout window.
func (g *Guard) RecordFailure(ctx context.Context, email string) {
	if !g.enabled() {
		return
	}
	n, err := g.redis.Incr(ctx, failKey(email))
	if err != nil {
		return
	}
	if n == 1 {
		_ = g.redis.Expire(ctx, failKey(email), g.lockout)
	}
	if int(n) >= g.maxAttempts {
		_ = g.redis.Set(ctx, lockKey(email), "1", g.lockout)
	}
}

// Reset clears failure and lock state after a successful authentication.
func (g *Guard) Reset(ctx context.Context, email string) {
	if !g.enabled() {
		return
	}
	_ = g.redis.Delete(ctx, failKey(email), lockKey(email))
}

// Revoke marks a token id (jti) as revoked until ttl elapses. ttl should be the
// token's remaining lifetime so the entry is dropped once the token expires
// naturally. A non-positive ttl or empty jti is a no-op.
func (g *Guard) Revoke(ctx context.Context, jti string, ttl time.Duration) error {
	if !g.enabled() || jti == "" || ttl <= 0 {
		return nil
	}
	return g.redis.Set(ctx, revokedKey(jti), "1", ttl)
}

// IsRevoked reports whether a token id has been revoked. On Redis error it
// returns false (fail open).
func (g *Guard) IsRevoked(ctx context.Context, jti string) bool {
	if !g.enabled() || jti == "" {
		return false
	}
	revoked, err := g.redis.Exists(ctx, revokedKey(jti))
	if err != nil {
		return false
	}
	return revoked
}
