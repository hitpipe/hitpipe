package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/deliveryrun"
)

var revision string

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := configuration()
	if err == nil {
		err = launch(ctx, cfg)
	}
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func configuration() (deliveryrun.Config, error) {
	cfg := deliveryrun.Config{
		AppDSN: os.Getenv("DELIVERY_APP_DSN"), WorkerDSN: os.Getenv("DELIVERY_WORKER_DSN"),
		ContentPath: os.Getenv("DELIVERY_CONTENT_DIR"), Revision: revision,
		BusinessAddr: os.Getenv("DELIVERY_BUSINESS_ADDR"), HealthAddr: os.Getenv("DELIVERY_HEALTH_ADDR"),
		ProcessingOrigin: os.Getenv("DELIVERY_PROCESSING_ORIGIN"), ProcessingSAN: "processing.internal",
		OpsSAN: "ops.internal",
	}
	if cfg.AppDSN == "" || cfg.WorkerDSN == "" || cfg.ContentPath == "" {
		return cfg, errors.New("closed: database or private content configuration is missing")
	}
	dir := os.Getenv("DELIVERY_TLS_DIR")
	if dir == "" {
		return cfg, errors.New("closed: TLS configuration is missing")
	}
	var err error
	cfg.Server, err = certificate(dir, "delivery-server")
	if err != nil {
		return cfg, err
	}
	cfg.Client, err = certificate(dir, "delivery-client")
	if err != nil {
		return cfg, err
	}
	cfg.ProcessingCallerCA, err = roots(dir, "processing-client-ca")
	if err != nil {
		return cfg, err
	}
	cfg.ProcessingPeerCA, err = roots(dir, "processing-server-ca")
	if err != nil {
		return cfg, err
	}
	cfg.OpsCA, err = roots(dir, "ops-client-ca")
	return cfg, err
}

func certificate(dir, name string) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, name+".pem"), filepath.Join(dir, name+".key"))
	if err != nil || cert.Leaf == nil || time.Now().Before(cert.Leaf.NotBefore) || !time.Now().Before(cert.Leaf.NotAfter) {
		return tls.Certificate{}, errors.New("closed: " + name + " TLS identity unavailable or expired")
	}
	return cert, nil
}

func roots(dir, name string) (*x509.CertPool, error) {
	raw, err := os.ReadFile(filepath.Join(dir, name+".pem"))
	ca := x509.NewCertPool()
	if err != nil || !ca.AppendCertsFromPEM(raw) {
		return nil, errors.New("closed: " + name + " trust unavailable")
	}
	return ca, nil
}
