//go:build integration

package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSessionRepository_FindByID_Existing(t *testing.T) {
	dsn := os.Getenv("HITPIPE_TEST_DSN")
	if dsn == "" {
		t.Skip("HITPIPE_TEST_DSN not set; skipping integration test")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	defer pool.Close()

	repo := NewSessionRepository(pool)
	id := uuid.New()
	principalID := uuid.New()
	now := time.Now()
	expiresAt := now.Add(24 * time.Hour)
	idleExpiresAt := now.Add(time.Hour)

	// Insert a test session
	_, err = pool.Exec(context.Background(), `
		INSERT INTO hp_iam.sessions
		(session_id, principal_id, session_hash, csrf_hash,
		 created_at, expires_at, idle_expires_at, revoked_at, epoch)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, id, principalID, []byte("test-session-hash-32bytes!!!!!!!!!!!!!!"),
		[]byte("test-csrf-hash-32bytes!!!!!!!!!!!!!!!!"), now, expiresAt,
		idleExpiresAt, nil, 1)
	if err != nil {
		t.Fatalf("failed to insert test session: %v", err)
	}

	s, err := repo.FindByID(context.Background(), id)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if s == nil {
		t.Fatal("FindByID returned nil session")
	}
	if s.ID != id {
		t.Fatalf("expected session id %s, got %s", id, s.ID)
	}
	if s.PrincipalID != principalID {
		t.Fatalf("expected principal id %s, got %s", principalID, s.PrincipalID)
	}
	if s.RevokedAt != nil {
		t.Fatal("expected nil RevokedAt for active session")
	}
	if !s.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at %v, got %v", expiresAt, s.ExpiresAt)
	}
	if !s.IdleExpiresAt.Equal(idleExpiresAt) {
		t.Fatalf("expected idle_expires_at %v, got %v", idleExpiresAt, s.IdleExpiresAt)
	}
	if s.Epoch != 1 {
		t.Fatalf("expected epoch 1, got %d", s.Epoch)
	}

	// Opaque fields must not be nil/empty
	if len(s.SessionHash) == 0 {
		t.Fatal("expected non-empty SessionHash")
	}
	if len(s.CSRFHash) == 0 {
		t.Fatal("expected non-empty CSRFHash")
	}
}

func TestSessionRepository_FindByID_NotFound(t *testing.T) {
	dsn := os.Getenv("HITPIPE_TEST_DSN")
	if dsn == "" {
		t.Skip("HITPIPE_TEST_DSN not set; skipping integration test")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	defer pool.Close()

	repo := NewSessionRepository(pool)
	missingID := uuid.New()

	s, err := repo.FindByID(context.Background(), missingID)
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got: %v", err)
	}
	if s != nil {
		t.Fatal("expected nil session for missing id")
	}
}

func TestSessionRepository_FindByID_Revoked(t *testing.T) {
	dsn := os.Getenv("HITPIPE_TEST_DSN")
	if dsn == "" {
		t.Skip("HITPIPE_TEST_DSN not set; skipping integration test")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	defer pool.Close()

	repo := NewSessionRepository(pool)
	id := uuid.New()
	principalID := uuid.New()
	now := time.Now()
	revokedAt := now.Add(-10 * time.Minute)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO hp_iam.sessions
		(session_id, principal_id, session_hash, csrf_hash,
		 created_at, expires_at, idle_expires_at, revoked_at, epoch)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, id, principalID, []byte("test-session-hash-32bytes!!!!!!!!!!!!!!"),
		[]byte("test-csrf-hash-32bytes!!!!!!!!!!!!!!!!"), now,
		now.Add(24*time.Hour), now.Add(time.Hour), revokedAt, 1)
	if err != nil {
		t.Fatalf("failed to insert revoked test session: %v", err)
	}

	s, err := repo.FindByID(context.Background(), id)
	if err != nil {
		t.Fatalf("FindByID returned error: %v", err)
	}
	if s == nil {
		t.Fatal("FindByID returned nil session")
	}
	if !s.IsRevoked() {
		t.Fatal("expected session to be revoked")
	}
	if s.RevokedAt == nil || !s.RevokedAt.Equal(revokedAt) {
		t.Fatalf("expected revoked_at %v, got %v", revokedAt, s.RevokedAt)
	}
}
