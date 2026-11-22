package iamprocess

import (
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"testing"
)

func TestStrictListenerInput(t *testing.T) {
	const good = `{"business":{"address":"127.0.0.1:8443","certificate_file":"/a.pem","edge_peers":[{"dns_name":"edge.internal","certificate_fingerprint":"value"}]},"ops":{"address":"127.0.0.1:9444","peers":[{"dns_name":"ops.internal","certificate_fingerprint":"value"}]}}`
	var cfg configInput
	if err := json.Unmarshal([]byte(good), &cfg, json.RejectUnknownMembers(true)); err != nil {
		t.Fatalf("provisioned listener JSON was rejected: %v", err)
	}
	if cfg.Business.Address != "127.0.0.1:8443" || cfg.Business.CertificateFile != "/a.pem" ||
		len(cfg.Business.EdgePeers) != 1 || cfg.OPS.Address != "127.0.0.1:9444" || len(cfg.OPS.Peers) != 1 {
		t.Fatal("listener settings were not decoded")
	}
	for _, input := range []string{
		`{"business":{"address":"127.0.0.1:8443","address":"127.0.0.1:9444"}}`,
		`{"business":{"address":"127.0.0.1:8443","address_override":"0.0.0.0:8443"}}`,
	} {
		var bad configInput
		if err := json.Unmarshal([]byte(input), &bad, json.RejectUnknownMembers(true)); err == nil {
			t.Fatal("unprovisioned listener setting was accepted")
		}
	}
}

func TestPoolUsesOnlyExplicitRuntimeProvisioning(t *testing.T) {
	t.Setenv("PGPORT", "1")
	t.Setenv("PGPASSWORD", "synthetic ambient password")
	t.Setenv("PGOPTIONS", "-c role=synthetic_wrong_role")
	t.Setenv("PGMINPROTOCOLVERSION", "invalid-ambient-value")
	t.Setenv("PGMAXPROTOCOLVERSION", "invalid-ambient-value")
	t.Setenv("PGCHANNELBINDING", "disable")
	t.Setenv("PGREQUIREAUTH", "unexpected-ambient-auth")
	t.Setenv("PGSSLSNI", "0")
	path := filepath.Join(t.TempDir(), "dsn")
	if err := os.WriteFile(path, []byte("postgres://fixture_role@127.0.0.1/iam_test?sslmode=disable"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := databaseInput{DSNFile: path}
	settings, err := input.load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.ConnConfig.Port != 5432 || settings.ConnConfig.Password != "" || settings.ConnConfig.RuntimeParams["options"] != "" ||
		settings.ConnConfig.MinProtocolVersion != "3.0" || settings.ConnConfig.MaxProtocolVersion != "3.0" ||
		settings.ConnConfig.ChannelBinding != "prefer" || settings.ConnConfig.RequireAuth != "" ||
		settings.MaxConns != 5 || settings.MinConns != 0 || settings.AfterConnect == nil {
		t.Fatal("a database setting came from outside the permitted sources")
	}
	pool, err := (Config{pool: settings}).OpenPool(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	oversized := int32(11)
	input.MaxConnections = &oversized
	if _, err := input.load(); err == nil {
		t.Fatal("an oversized PostgreSQL pool was accepted")
	}
	input.MaxConnections = nil
	t.Setenv("PGSERVICE", "ambient")
	service := filepath.Join(filepath.Dir(path), "services")
	if err := os.WriteFile(service, []byte("[ambient]\nhost=127.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PGSERVICEFILE", service)
	if _, err := input.load(); err == nil {
		t.Fatal("an unrelated PostgreSQL service file was accepted")
	}
}

func TestDatabaseProvisioningRejectsFallbacks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dsn")
	for _, dsn := range []string{
		"postgres://fixture_role@db.internal/iam_test?sslmode=disable",
		"postgres://fixture_role@127.0.0.1/iam_test?sslmode=prefer",
		"postgres://fixture_role@127.0.0.1/iam_test?sslmode=disable&sslmode=verify-full",
		"postgres://fixture_role@127.0.0.1/iam_test?sslmode=disable&options=-c+role=another",
	} {
		if err := os.WriteFile(path, []byte(dsn), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := (databaseInput{DSNFile: path}).load(); err == nil {
			t.Fatal("a disallowed database route was accepted")
		}
	}
}
