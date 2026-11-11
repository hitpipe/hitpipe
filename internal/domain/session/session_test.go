package session

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSession_Valid_Active(t *testing.T) {
	now := time.Now()
	s := &Session{
		ID:            uuid.New(),
		PrincipalID:   uuid.New(),
		CreatedAt:     now.Add(-time.Hour),
		ExpiresAt:     now.Add(time.Hour),
		IdleExpiresAt: now.Add(30 * time.Minute),
		RevokedAt:     nil,
		Epoch:         1,
	}

	if err := s.Valid(now); err != nil {
		t.Fatalf("expected active session to be valid, got: %v", err)
	}
}

func TestSession_Valid_Revoked(t *testing.T) {
	now := time.Now()
	revoked := now.Add(-10 * time.Minute)
	s := &Session{
		ID:            uuid.New(),
		PrincipalID:   uuid.New(),
		CreatedAt:     now.Add(-time.Hour),
		ExpiresAt:     now.Add(time.Hour),
		IdleExpiresAt: now.Add(30 * time.Minute),
		RevokedAt:     &revoked,
		Epoch:         1,
	}

	err := s.Valid(now)
	if err == nil {
		t.Fatal("expected error for revoked session")
	}
	if !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("expected ErrSessionRevoked, got: %v", err)
	}
}

func TestSession_Valid_Expired(t *testing.T) {
	now := time.Now()
	s := &Session{
		ID:            uuid.New(),
		PrincipalID:   uuid.New(),
		CreatedAt:     now.Add(-2 * time.Hour),
		ExpiresAt:     now.Add(-time.Minute), // expired 1 minute ago
		IdleExpiresAt: now.Add(-time.Minute),
		RevokedAt:     nil,
		Epoch:         1,
	}

	err := s.Valid(now)
	if err == nil {
		t.Fatal("expected error for expired session")
	}
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got: %v", err)
	}
}

func TestSession_Valid_IdleExpired(t *testing.T) {
	now := time.Now()
	s := &Session{
		ID:            uuid.New(),
		PrincipalID:   uuid.New(),
		CreatedAt:     now.Add(-time.Hour),
		ExpiresAt:     now.Add(time.Hour),
		IdleExpiresAt: now.Add(-time.Minute), // idle-expired 1 minute ago
		RevokedAt:     nil,
		Epoch:         1,
	}

	err := s.Valid(now)
	if err == nil {
		t.Fatal("expected error for idle-expired session")
	}
	if !errors.Is(err, ErrSessionIdleExpired) {
		t.Fatalf("expected ErrSessionIdleExpired, got: %v", err)
	}
}

func TestSession_IsRevoked(t *testing.T) {
	s := &Session{RevokedAt: nil}
	if s.IsRevoked() {
		t.Fatal("expected IsRevoked() to be false when RevokedAt is nil")
	}

	revoked := time.Now()
	s.RevokedAt = &revoked
	if !s.IsRevoked() {
		t.Fatal("expected IsRevoked() to be true when RevokedAt is set")
	}
}

func TestSession_IsExpired(t *testing.T) {
	now := time.Now()
	s := &Session{ExpiresAt: now.Add(time.Hour)}
	if s.IsExpired(now) {
		t.Fatal("expected IsExpired() to be false for future expires_at")
	}

	s.ExpiresAt = now.Add(-time.Minute)
	if !s.IsExpired(now) {
		t.Fatal("expected IsExpired() to be true for past expires_at")
	}
}

func TestSession_IsIdleExpired(t *testing.T) {
	now := time.Now()
	s := &Session{IdleExpiresAt: now.Add(time.Hour)}
	if s.IsIdleExpired(now) {
		t.Fatal("expected IsIdleExpired() to be false for future idle_expires_at")
	}

	s.IdleExpiresAt = now.Add(-time.Minute)
	if !s.IsIdleExpired(now) {
		t.Fatal("expected IsIdleExpired() to be true for past idle_expires_at")
	}
}
