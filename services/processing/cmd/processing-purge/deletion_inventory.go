package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deletionregistry"
)

type inventoryConfig struct {
	Origin       string `json:"origin"`
	ServerPin    string `json:"server_pin"`
	PublisherID  string `json:"publisher_id"`
	WorkspaceID  string `json:"workspace_id"`
	Generation   uint64 `json:"generation"`
	CA           string `json:"ca"`
	Certificate  string `json:"client_certificate"`
	PrivateKey   string `json:"client_key"`
	PublisherKey string `json:"publisher_public_key"`
}

func privateInput(path string, limit int64) ([]byte, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if !filepath.IsAbs(path) || err != nil || resolved != path {
		return nil, deletionregistry.ErrUnavailable
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit || info.Mode().Perm() != 0600 {
		return nil, deletionregistry.ErrUnavailable
	}
	if owner, ok := info.Sys().(*syscall.Stat_t); !ok || int(owner.Uid) != os.Geteuid() {
		return nil, deletionregistry.ErrUnavailable
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, deletionregistry.ErrUnavailable
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, deletionregistry.ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, deletionregistry.ErrUnavailable
	}
	return data, nil
}

func inventoryClient(path string) (*deletionregistry.Client, error) {
	raw, err := privateInput(path, 16<<10)
	if err != nil {
		return nil, err
	}
	var cfg inventoryConfig
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&cfg) != nil || d.Decode(new(any)) != io.EOF {
		return nil, deletionregistry.ErrUnavailable
	}
	ca, err := privateInput(cfg.CA, 32<<10)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, deletionregistry.ErrUnavailable
	}
	cert, err := privateInput(cfg.Certificate, 32<<10)
	if err != nil {
		return nil, err
	}
	key, err := privateInput(cfg.PrivateKey, 32<<10)
	if err != nil {
		return nil, err
	}
	leaf, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, deletionregistry.ErrUnavailable
	}
	pub, err := privateInput(cfg.PublisherKey, 4096)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(pub)
	if block == nil || block.Type != "PUBLIC KEY" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, deletionregistry.ErrUnavailable
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	publisher, ok := parsed.(ed25519.PublicKey)
	if !ok || err != nil {
		return nil, deletionregistry.ErrUnavailable
	}
	return deletionregistry.New(deletionregistry.Provision{
		Origin: cfg.Origin, ServerPin: cfg.ServerPin, PublisherID: cfg.PublisherID,
		WorkspaceID: cfg.WorkspaceID, Generation: cfg.Generation,
	}, pool, leaf, publisher)
}

func observeDeletionInventory(ctx context.Context, path string) int {
	client, err := inventoryClient(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, deletionregistry.ErrUnavailable)
		return 2
	}
	observation, err := client.Observe(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, deletionregistry.ErrUnavailable)
		return 1
	}
	response := struct {
		deletionregistry.Observation
		Serving string `json:"serving"`
	}{observation, "CLOSED"}
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		return 1
	}
	return 0
}
