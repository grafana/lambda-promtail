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

// Implements Client
type promtailClient struct {
	config *promtailClientConfig
	http   *http.Client
	log    *log.Logger
}

type promtailClientConfig struct {
	backoff *backoff.Config
	http    *httpClientConfig
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
