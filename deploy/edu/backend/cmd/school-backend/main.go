// school-backend composes the real services in an isolated educational profile.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	workspaceID      = "11111111-1111-4111-8111-111111111111"
	sourceID         = "22222222-2222-4222-8222-222222222222"
	destinationID    = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	ownerID          = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	sessionID        = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	stripeAccount    = "acct_synthetic"
	stripeVersion    = "2026-10-28.endive"
	clickUpWorkspace = "123400"
	clickUpList      = "123401"
	clickUpAssignee  = "123402"
	clickUpToken     = "synthetic-hitpipe-demo"
	label            = "Synthetic service"
	businessAddress  = "127.0.0.1:8443"
	iamAddress       = "127.0.0.2:8443"
	intakeAddress    = "127.0.0.1:18444"
	deliveryAddress  = "127.0.0.6:8443"
	stripeAddress    = "127.0.0.1:18445"
	clickUpAddress   = "127.0.0.1:18446"
)

const (
	intakeMigrator = "postgres://hp_intake_migrator@127.0.0.1:5432/intake_test?sslmode=disable"
	deliveryAdmin  = "postgres://postgres@127.0.0.1:5432/delivery_demo?sslmode=disable"
	processingApp  = "host=/var/run/postgresql dbname=hit78 sslmode=disable user=hp_processing_app"
)

type backend struct {
	state, share, binaries string
	workspaceID            string
	keys                   *identity
	certs                  *certificates
	provider               *providers
	demo                   *demoProviders
	queueMu                sync.Mutex
	childrenMu             sync.Mutex
	children               []*child
}

type child struct {
	name    string
	command *exec.Cmd
	done    chan struct{}
	err     error
}

func main() {
	log.SetFlags(0)
	if len(os.Args) == 2 && os.Args[1] == "--resolve-school-workspace" {
		state := os.Getenv("HITPIPE_SCHOOL_STATE")
		if err := privateDir(state); err != nil {
			log.Fatal(err)
		}
		id, err := resolveWorkspace(state, os.Getenv("HITPIPE_SCHOOL_WORKSPACE_FILE"))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(id)
		return
	}
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	if len(os.Args) != 1 {
		return errors.New("school-backend starts with no positional options")
	}
	state, share, binaries := os.Getenv("HITPIPE_SCHOOL_STATE"), os.Getenv("HITPIPE_BROWSER_SHARE"), os.Getenv("HITPIPE_SCHOOL_BIN")
	for _, path := range []string{state, share, binaries} {
		if err := privateDir(path); err != nil {
			return err
		}
	}
	workspace, err := bindWorkspace(state, os.Getenv("HITPIPE_SCHOOL_WORKSPACE_FILE"), os.Getenv("HITPIPE_SCHOOL_EXPECTED_WORKSPACE_ID"))
	if err != nil {
		return err
	}
	pidFile := filepath.Join(state, "backend.pid")
	if err := writePrivate(pidFile, []byte(fmt.Sprintf("%d\n", os.Getpid()))); err != nil {
		return err
	}
	defer os.Remove(pidFile)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer cancel()
	s := &backend{state: state, share: share, binaries: binaries, workspaceID: workspace}
	defer s.stopChildren()
	if s.demo, err = loadDemoProviders(state, share); err != nil {
		return err
	}
	if s.demo.external() {
		if err = s.demo.attest(ctx); err != nil {
			return err
		}
	}

	if err := os.Remove(filepath.Join(share, "ready.json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if s.keys, err = loadIdentity(state); err != nil {
		return err
	}
	if s.certs, err = initCertificates(state); err != nil {
		return err
	}
	if s.provider, err = loadProviders(filepath.Join(state, "providers.json"), s.keys); err != nil {
		return err
	}
	if err = s.seedConnections(ctx); err != nil {
		return err
	}
	if err = s.prepareConfig(); err != nil {
		return err
	}
	stripeHandler, clickUpHandler := http.HandlerFunc(s.provider.stripe), http.HandlerFunc(s.provider.clickup)
	if s.demo.external() {
		stripeHandler = s.demo.proxy("stripe", func(r *http.Request) bool {
			return constantEqual(r.Header.Get("Authorization"), "Bearer "+s.keys.StripeKey) &&
				r.Header.Get("Stripe-Account") == s.demo.SourceAccount &&
				r.Header.Get("Stripe-Version") == demoStripeVersion
		})
		clickUpHandler = s.demo.proxy("clickup", func(r *http.Request) bool {
			return constantEqual(r.Header.Get("Authorization"), clickUpToken)
		})
	}
	for _, server := range []struct {
		address, peer, cert string
		handler             http.HandlerFunc
	}{
		{iamAddress, "processing-client.test", "iam.test", s.authorize},
		{stripeAddress, "", "stripe.fixture", stripeHandler},
		{clickUpAddress, "", "clickup.fixture", clickUpHandler},
	} {
		if err = s.serveHTTPS(ctx, server.address, server.peer, server.cert, server.handler); err != nil {
			return err
		}
	}
	runtimeDir := filepath.Join(state, "runtime")
	if _, err = s.start("intake", "-config", filepath.Join(runtimeDir, "intake", "intake.json")); err != nil {
		return err
	}
	if _, err = s.start("processing", "-config", filepath.Join(runtimeDir, "processing.json"), "-stand-config", filepath.Join(runtimeDir, "stand.json")); err != nil {
		return err
	}
	if err := s.waitForOwners(ctx); err != nil {
		return err
	}
	delivery, err := s.startDelivery()
	if err != nil {
		return err
	}
	if err := waitTCP(ctx, deliveryAddress); err != nil {
		return err
	}
	if err = s.relay(ctx); err != nil {
		return err
	}
	if err = s.serveControl(ctx); err != nil {
		return err
	}
	if s.demo.external() {
		if err = s.serveDemoWebhook(ctx); err != nil {
			return err
		}
	}
	if err = s.publishReady(); err != nil {
		return err
	}
	defer os.Remove(filepath.Join(share, "ready.json"))
	if err = s.publishResult(ctx); err != nil {
		return err
	}
	fmt.Println("HitPipe school backend ready. Send a signed test event through the private controls.")
	renewal := time.NewTicker(time.Hour)
	defer renewal.Stop()
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-renewal.C:
			stopChild(delivery)
			s.children = s.children[:len(s.children)-1]
			delivery, err = s.startDelivery()
			if err != nil {
				return err
			}
		case <-poll.C:
			if err := s.checkChildren(); err != nil {
				return err
			}
		}
	}
}

func privateDir(path string) error {
	st, err := os.Lstat(path)
	if err != nil || !filepath.IsAbs(path) || !st.IsDir() || st.Mode().Perm() != 0700 {
		return errors.New("school directories must be absolute private directories (0700)")
	}
	return nil
}

func writeJSON(path string, data any) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(path, append(raw, '\n'))
}

func writePrivate(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".school-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (s *backend) start(name string, args ...string) (*child, error) {
	cmd := exec.Command(filepath.Join(s.binaries, name), args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &child{name: name, command: cmd, done: make(chan struct{})}
	s.childrenMu.Lock()
	s.children = append(s.children, p)
	s.childrenMu.Unlock()
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

func (s *backend) checkChildren() error {
	s.childrenMu.Lock()
	defer s.childrenMu.Unlock()
	for _, p := range s.children {
		select {
		case <-p.done:
			return fmt.Errorf("%s stopped unexpectedly (%v)", p.name, p.err)
		default:
		}
	}
	return nil
}

func stopChild(p *child) {
	_ = p.command.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		_ = p.command.Process.Kill()
		<-p.done
	}
}

func (s *backend) stopChildren() {
	for i := len(s.children) - 1; i >= 0; i-- {
		stopChild(s.children[i])
	}
}

func waitTCP(ctx context.Context, address string) error {
	run, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	for {
		dialer := net.Dialer{Timeout: 300 * time.Millisecond}
		if conn, err := dialer.DialContext(run, "tcp", address); err == nil {
			conn.Close()
			return nil
		}
		select {
		case <-run.Done():
			return fmt.Errorf("no listener at %s: %w", address, run.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (s *backend) waitForOwners(ctx context.Context) error {
	if err := waitTCP(ctx, intakeAddress); err != nil {
		return err
	}
	run, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client := s.certs.client("ops.test", "processing.fixture")
	for {
		err := s.checkChildren()
		if err != nil {
			return err
		}
		req, _ := http.NewRequestWithContext(run, http.MethodGet, "https://127.0.0.1:9444/internal/v1/health/processing", nil)
		req.Header.Set("X-Request-ID", mustUUID())
		response, err := client.Do(req)
		if err == nil {
			var status struct {
				Status string `json:"status"`
			}
			err = json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&status)
			response.Body.Close()
			if err == nil && status.Status == "READY" && response.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-run.Done():
			return fmt.Errorf("Processing did not report READY: %w", run.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}
