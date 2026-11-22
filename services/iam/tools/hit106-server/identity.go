//go:build hit106lab

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This loopback-only, explicitly tagged lab endpoint proves the persisted
// identity behind a real browser session. Raw subjects, principal IDs and session
// references never leave it; synthetic fixture IDs can be compared by digest.
func serveIdentityProof(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, authenticate authenticatesession.Service) {
	cookie, err := r.Cookie("__Host-hp_session")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	reference, err := sessionlookup.Reference(cookie.Value)
	if err != nil || !authenticate.Execute(r.Context(), reference).IsValid() {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	digest, err := hex.DecodeString(reference)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	var principal, issuer, subject string
	err = pool.QueryRow(ctx, `SELECT p.principal_id::text,p.oidc_issuer,p.oidc_subject
		FROM hp_iam.sessions s JOIN hp_iam.principals p USING(principal_id)
		WHERE s.session_hash=$1`, digest).Scan(&principal, &issuer, &subject)
	if err != nil || principal == "" || issuer == "" || subject == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	hash := func(value string) string {
		digest := sha256.Sum256([]byte(value))
		return hex.EncodeToString(digest[:])
	}
	result := struct {
		PrincipalDigest string `json:"principal_digest"`
		IssuerDigest    string `json:"issuer_digest"`
		SubjectDigest   string `json:"subject_digest"`
	}{hash(principal), hash(issuer), hash(subject)}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
