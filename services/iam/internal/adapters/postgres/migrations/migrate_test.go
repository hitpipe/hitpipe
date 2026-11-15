package migrations

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestAcceptedMigrationBytes(t *testing.T) {
	data := SchemaSQL()
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != SQLSHA256 {
		t.Fatal("migration differs from accepted immutable IAM SQL")
	}
	fixture, err := os.ReadFile("../sessionlookup/testdata/iam.sql")
	if err != nil || !bytes.Equal(fixture, data) {
		t.Fatal("migration must preserve the complete HIT-68 schema fixture")
	}
	data[0] = '!'
	if bytes.Equal(data, SchemaSQL()) {
		t.Fatal("exported bytes changed the embedded migration")
	}
}
