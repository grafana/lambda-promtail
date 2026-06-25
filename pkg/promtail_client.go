package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/backoff"
)

type Client interface {
	sendToPromtail(ctx context.Context, b *batch) error
}

// AuthOption mutates an outgoing request to add authentication information.
// Options are applied in the order they are passed to the client.
type AuthOption interface {
	Apply(ctx context.Context, req *http.Request) error
}

// basicAuthOption sets HTTP basic auth credentials on the request.
type basicAuthOption struct {
	username string
	password string
}

func (o basicAuthOption) Apply(_ context.Context, req *http.Request) error {
	req.SetBasicAuth(o.username, o.password)
	return nil
}

// bearerTokenOption sets a static bearer token Authorization header on the request.
type bearerTokenOption struct {
	token string
}

func (o bearerTokenOption) Apply(_ context.Context, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+o.token)
	return nil
}

// tenantIDOption sets the X-Scope-OrgID header used by Loki for multi-tenancy.
type tenantIDOption struct {
	tenantID string
}

func (o tenantIDOption) Apply(_ context.Context, req *http.Request) error {
	req.Header.Set("X-Scope-OrgID", o.tenantID)
	return nil
}

// Implements Client
type promtailClient struct {
	config *promtailClientConfig
	http   *http.Client
	log    *log.Logger
}

type promtailClientConfig struct {
	backoff *backoff.Config
	http    *httpClientConfig
	auth    []AuthOption
}

type httpClientConfig struct {
	timeout       time.Duration
	skipTLSVerify bool
}

func NewPromtailClient(cfg *promtailClientConfig, log *log.Logger) Client {
	return &promtailClient{
		config: cfg,
		http:   NewHTTPClient(cfg.http),
		log:    log,
	}
}

func NewHTTPClient(cfg *httpClientConfig) *http.Client {
	transport := http.DefaultTransport
	if cfg.skipTLSVerify {
		transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //#nosec G402 -- User has explicitly requested to disable TLS
	}
	return &http.Client{
		Timeout:   cfg.timeout,
		Transport: transport,
	}
}
