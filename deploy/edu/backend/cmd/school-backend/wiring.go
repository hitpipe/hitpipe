package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	config "github.com/hitpipe/hitpipe/services/processing/internal/runtime"
	"github.com/jackc/pgx/v5"
)

const intakeRelayAddress = "127.0.0.5:8443"

func (s *backend) seedConnections(ctx context.Context) error {
	seed, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	intake, err := pgx.Connect(seed, intakeMigrator)
	if err != nil {
		return fmt.Errorf("open isolated Intake fixture database: %w", err)
	}
	defer intake.Close(context.Background())
	tx, err := intake.Begin(seed)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(seed, `SELECT set_config('hp.workspace_id',$1,true)`, workspaceID); err != nil {
		return err
	}
	if _, err = tx.Exec(seed, `INSERT INTO hp_intake.source_connections
		(workspace_id,connection_id,account_id,livemode,security_epoch,health,verified_at)
		VALUES ($1,$2,$3,false,7,'READY',clock_timestamp()) ON CONFLICT (workspace_id,connection_id) DO NOTHING`,
		workspaceID, sourceID, stripeAccount); err != nil {
		return err
	}
	if err = tx.Commit(seed); err != nil {
		return err
	}
	delivery, err := pgx.Connect(seed, deliveryAdmin)
	if err != nil {
		return fmt.Errorf("open isolated Delivery fixture database: %w", err)
	}
	defer delivery.Close(context.Background())
	_, err = delivery.Exec(seed, `INSERT INTO hp_delivery.destination_connections
		(workspace_id,connection_id,external_workspace_id,security_epoch,token_fingerprint,health)
		VALUES ($1,$2,$3,7,$4,'READY') ON CONFLICT (workspace_id,connection_id) DO NOTHING`,
		workspaceID, destinationID, clickUpWorkspace, bytes.Repeat([]byte{0x13}, 32))
	return err
}

type configWriter struct {
	dir string
	err error
}

func (c *configWriter) file(name string, data []byte) string {
	path := filepath.Join(c.dir, name)
	if c.err == nil {
		c.err = writePrivate(path, data)
	}
	return path
}

func (c *configWriter) json(name string, data any) string {
	path := filepath.Join(c.dir, name)
	if c.err == nil {
		c.err = writeJSON(path, data)
	}
	return path
}

func (c *configWriter) keypair(name string, pair keypair) (string, string) {
	return c.file(name+".pem", pair.Cert), c.file(name+".key", pair.Key)
}

func (s *backend) prepareConfig() error {
	runtimeDir := filepath.Join(s.state, "runtime")
	intakeDir := filepath.Join(runtimeDir, "intake")
	processingData := filepath.Join(s.state, "processing-content")
	for _, dir := range []string{runtimeDir, intakeDir, filepath.Join(intakeDir, "content"), processingData, filepath.Join(s.state, "delivery-content")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := privateDir(dir); err != nil {
			return err
		}
	}
	ic := &configWriter{dir: intakeDir}
	ic.file("service-ca.pem", s.certs.CA)
	ic.file("stripe-ca.pem", s.certs.CA)
	ic.keypair("intake-server", s.certs.Leaves["intake.test-server"])
	ic.keypair("intake-client", s.certs.Leaves["intake.test"])
	ic.file("webhook.secret", []byte(s.keys.Signing))
	ic.file("stripe.secret", []byte(s.keys.StripeKey))
	ic.json("intake.json", map[string]any{
		"profile": "qualification", "listen": intakeAddress, "database_port": 5432,
		"stripe_origin": "https://" + stripeAddress, "processing_address": businessAddress, "batch": 1, "poll_ms": 125, "qualification_epoch": 7,
	})
	if ic.err != nil {
		return ic.err
	}
	rc := &configWriter{dir: runtimeDir}
	ca := rc.file("service-ca.pem", s.certs.CA)
	cert, key := rc.keypair("processing-server", s.certs.Leaves["processing.fixture"])
	clientCert, clientKey := rc.keypair("processing-client", s.certs.Leaves["processing-client.test"])
	rc.json("processing.json", config.Config{
		Business: businessAddress, Control: "127.0.0.1:9443", Health: "127.0.0.1:9444",
		Identity: s.keys.RuntimeID, Generation: 1, OwnerDB: "hit78",
		AppDSN:    rc.file("processing-app.dsn", []byte(processingApp)),
		WorkerDSN: rc.file("processing-worker.dsn", []byte("host=/var/run/postgresql dbname=hit78 sslmode=disable user=hp_processing_worker")),
		CursorKey: rc.file("cursor.key", s.keys.CursorKey), Content: processingData, Connections: 4,
		TLS:      config.TLSFiles{ServerCertificate: cert, ServerKey: key, ClientCertificate: clientCert, ClientKey: clientKey, InboundCA: ca, OutboundCA: ca},
		IAM:      config.Peer{Origin: "https://" + iamAddress, Name: "iam.test"},
		Intake:   config.Peer{Origin: "https://" + intakeRelayAddress, Name: "intake.test"},
		Delivery: config.Peer{Origin: "https://" + deliveryAddress, Name: "delivery.test"},
		BFFName:  "bff.test", OPSName: "ops.test",
	})
	lock := filepath.Join(runtimeDir, "queue.lock")
	file, err := os.OpenFile(lock, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	rc.json("stand.json", config.StandConfig{Workspaces: []config.StandWorkspace{{
		ID: workspaceID, Owner: ownerID, SessionID: sessionID,
		SessionFile: rc.file("session", []byte(s.keys.Token)), CSRFFile: rc.file("csrf", []byte(s.keys.CSRF)),
		Source: sourceID, Destination: destinationID, Account: stripeAccount, DestinationWorkspace: clickUpWorkspace,
		Mode: "TEST", PriceAlias: "PRICE_ALIAS", ListAlias: "LIST_ALIAS", AssigneeAlias: "ASSIGNEE_ALIAS",
		Price: "price_fixed", List: clickUpList, Assignee: clickUpAssignee, Label: label,
		QueueSnapshot: filepath.Join(runtimeDir, "queue.json"), QueueLock: lock,
	}}})
	return rc.err
}

func (s *backend) startDelivery() (*child, error) {
	rc := &configWriter{dir: filepath.Join(s.state, "runtime")}
	serverCert, serverKey := rc.keypair("delivery-server", s.certs.Leaves["delivery.test-server"])
	clientCert, clientKey := rc.keypair("delivery-client", s.certs.Leaves["delivery.test"])
	ca := filepath.Join(rc.dir, "service-ca.pem")
	dsnPrefix := "postgres://"
	dsnSuffix := "@127.0.0.1:5432/delivery_demo?sslmode=disable"
	path := rc.json("delivery.json", map[string]any{
		"listen": deliveryAddress, "profile": "offline-receipt",
		"database_dsn_file":  rc.file("delivery-app.dsn", []byte(dsnPrefix+"hp_delivery_app"+dsnSuffix)),
		"content_directory":  filepath.Join(s.state, "delivery-content"),
		"server_certificate": serverCert, "server_key": serverKey, "client_ca": ca, "processing_san": "processing-client.test",
		"workspace_id": workspaceID, "connection_id": destinationID, "epoch": 7, "destination_workspace_id": clickUpWorkspace,
		"target_list_id": clickUpList, "assignee_id": clickUpAssignee, "valid_until": time.Now().UTC().Add(90 * time.Minute),
		"worker": map[string]any{
			"worker_dsn_file": rc.file("delivery-worker.dsn", []byte(dsnPrefix+"hp_delivery_worker"+dsnSuffix)),
			"provider_origin": "https://" + clickUpAddress, "provider_ca": ca,
			"processing_origin": "https://" + businessAddress, "processing_server_name": "processing.fixture", "processing_ca": ca,
			"processing_client_certificate": clientCert, "processing_client_key": clientKey,
		},
	})
	if rc.err != nil {
		return nil, rc.err
	}
	return s.start("delivery-offline-receipt", "-config", path)
}

func (s *backend) relay(ctx context.Context) error {
	if err := startRelay(ctx, "unix", filepath.Join(s.share, "processing.sock"), businessAddress); err != nil {
		return err
	}
	return startRelay(ctx, "tcp", intakeRelayAddress, intakeAddress)
}

func startRelay(ctx context.Context, network, address, target string) error {
	if network == "unix" {
		if err := resetSocket(address); err != nil {
			return err
		}
	}
	listener, err := net.Listen(network, address)
	if err != nil {
		return err
	}
	if network == "unix" {
		if err := os.Chmod(address, 0600); err != nil {
			listener.Close()
			return err
		}
	}
	context.AfterFunc(ctx, func() { listener.Close() })
	go func() {
		for {
			in, err := listener.Accept()
			if err != nil {
				return
			}
			go forward(ctx, in, target)
		}
	}()
	return nil
}

func resetSocket(path string) error {
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing non-socket at %s", path)
	}
	return os.Remove(path)
}

func forward(ctx context.Context, in net.Conn, target string) {
	defer in.Close()
	dialer := net.Dialer{Timeout: 2 * time.Second}
	out, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return
	}
	defer out.Close()
	stop := context.AfterFunc(ctx, func() { in.Close(); out.Close() })
	defer stop()
	// Requests on the relay carry TLS end-to-end. A bounded idle session
	// prevents one stale retained browser connection from trapping resources.
	expires := time.Now().Add(2 * time.Minute)
	_ = in.SetDeadline(expires)
	_ = out.SetDeadline(expires)
	closeWrite := func(connection net.Conn) {
		if half, ok := connection.(interface{ CloseWrite() error }); ok {
			_ = half.CloseWrite()
		}
	}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(out, in)
		closeWrite(out)
		close(done)
	}()
	_, _ = io.Copy(in, out)
	closeWrite(in)
	<-done
}

func (s *backend) publishReady() error {
	w := &configWriter{dir: s.share}
	w.file("ca.pem", s.certs.CA)
	w.keypair("bff", s.certs.Leaves["bff.test"])
	w.json("ready.json", map[string]any{
		"workspaceId": workspaceID, "sessionId": sessionID, "sessionToken": s.keys.Token, "csrfToken": s.keys.CSRF, "principalId": ownerID,
		"securityEpoch": 7, "processingServerName": "processing.fixture", "processingSocket": "processing.sock", "providerSocket": "provider.sock",
		"caPath": "ca.pem", "certPath": "bff.pem", "keyPath": "bff.key", "sourceConnectionId": sourceID, "destinationConnectionId": destinationID,
		"priceAlias": "PRICE_ALIAS", "listAlias": "LIST_ALIAS", "assigneeAlias": "ASSIGNEE_ALIAS", "serviceLabel": label,
	})
	return w.err
}

func (s *backend) publishResult(ctx context.Context) error {
	status, err := s.status(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	if status.CaseState != "CONFIRMED" {
		err := os.Remove(filepath.Join(s.share, "result.json"))
		if os.IsNotExist(err) {
			err = nil
		}
		return err
	}
	return writeJSON(filepath.Join(s.share, "result.json"), status)
}
