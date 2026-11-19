// Package intakeruntime owns the single Intake lifecycle and its local sandbox composition.
package intakeruntime

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gowebpki/jcs"
)

var (
	ErrConfig = errors.New("invalid Intake configuration")
	ErrClosed = errors.New("Intake is closed: installation and runtime authority are not configured")
)

type Config struct {
	Profile           string `json:"profile"`
	Listen            string `json:"listen"`
	DatabasePort      int    `json:"database_port"`
	StripeOrigin      string `json:"stripe_origin"`
	ProcessingAddress string `json:"processing_address"`
	Batch             int    `json:"batch"`
	PollMillis        int    `json:"poll_ms"`
	dir               string
}

func Load(path string) (Config, error) {
	var c Config
	if path == "" {
		return c, ErrClosed
	}
	f, err := os.Open(path)
	if err != nil {
		return c, ErrConfig
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(raw) > 16384 {
		return c, ErrConfig
	}
	if _, err = jcs.Transform(raw); err != nil {
		return c, ErrConfig
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&c) != nil {
		return c, ErrConfig
	}
	if c.Profile == "" || c.Profile == "ordinary" {
		if c.Listen != "" || c.DatabasePort != 0 || c.StripeOrigin != "" || c.ProcessingAddress != "" || c.Batch != 0 || c.PollMillis != 0 {
			return c, ErrConfig
		}
		return c, ErrClosed
	}
	if c.Profile != "sandbox" || !loopback(c.Listen) || !loopback(c.ProcessingAddress) || c.Listen == c.ProcessingAddress ||
		c.DatabasePort <= 0 || c.DatabasePort > 65535 || c.Batch < 1 || c.Batch > 32 || c.PollMillis < 25 || c.PollMillis > 10000 {
		return c, ErrConfig
	}
	u, err := url.Parse(c.StripeOrigin)
	if err != nil || u.Scheme != "https" || !loopback(u.Host) || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		u.Host == c.Listen || u.Host == c.ProcessingAddress {
		return c, ErrConfig
	}
	if c.Listen == net.JoinHostPort("127.0.0.1", strconv.Itoa(c.DatabasePort)) ||
		c.ProcessingAddress == net.JoinHostPort("127.0.0.1", strconv.Itoa(c.DatabasePort)) ||
		u.Host == net.JoinHostPort("127.0.0.1", strconv.Itoa(c.DatabasePort)) {
		return c, ErrConfig
	}
	c.dir, err = filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return c, ErrConfig
	}
	c.dir, err = filepath.Abs(c.dir)
	if err != nil {
		return c, ErrConfig
	}
	info, err := os.Stat(c.dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return c, ErrConfig
	}
	return c, nil
}

func loopback(address string) bool {
	host, port, err := net.SplitHostPort(address)
	p, e := strconv.ParseUint(port, 10, 16)
	return err == nil && host == "127.0.0.1" && e == nil && p > 0 && port == strconv.FormatUint(p, 10)
}
