package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestConnectionDiagnosticDoesNotExposeServerOrDSN(t *testing.T) {
	secret := "synthetic-private-connection-data"
	for _, input := range []error{
		errors.New(secret),
		&pgconn.PgError{Code: "42501", Message: secret, Detail: secret},
		&pgconn.PgError{Code: secret, Message: secret},
	} {
		if got := connectionCode(input); strings.Contains(got, secret) {
			t.Fatal("connection diagnostic exposed underlying data")
		}
	}
	if got := connectionCode(&pgconn.PgError{Code: "42501", Message: secret}); got != "IAM_MIGRATION_CONNECT_FAILED SQLSTATE=42501" {
		t.Fatal("lost safe PostgreSQL diagnostic")
	}
}
