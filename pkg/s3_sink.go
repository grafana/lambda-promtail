package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// s3Sink implements Client. It writes the raw log lines of a batch to a central
// SIEM S3 bucket (gzipped, time-partitioned), IN ADDITION to the Loki push done
// by promtailClient. Used for the phase-2 SIEM feed: the SIEM reads this bucket.
//
// It intentionally writes ONLY the raw log line (stream.Entries[].Line), not the
// Loki-proto wrapper, so a SIEM (OpenSearch/Data Prepper) can consume it directly.
type s3Sink struct {
	client *s3.Client
	bucket string
	prefix string
	log    *log.Logger
}

// newS3Sink builds an S3 sink using the ambient AWS credentials (IRSA role in
// the Lambda's execution environment). Region is resolved from the environment.
func newS3Sink(ctx context.Context, bucket, prefix string, logger *log.Logger) (*s3Sink, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("siem s3 sink: load aws config: %w", err)
	}
	return &s3Sink{
		client: s3.NewFromConfig(cfg),
		bucket: bucket,
		prefix: strings.TrimSuffix(prefix, "/"),
		log:    logger,
	}, nil
}

// sendToPromtail satisfies the Client interface. Despite the name (kept to match
// the interface), this writes the batch's raw log lines to S3.
func (s *s3Sink) sendToPromtail(ctx context.Context, b *batch) error {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)

	lines := 0
	for _, stream := range b.streams {
		for _, e := range stream.Entries {
			if e.Line == "" {
				continue
			}
			if _, err := gz.Write([]byte(e.Line + "\n")); err != nil {
				_ = gz.Close()
				return fmt.Errorf("siem s3 sink: gzip write: %w", err)
			}
			lines++
		}
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("siem s3 sink: gzip close: %w", err)
	}

	// Nothing to write (batch held only empty lines) — skip silently.
	if lines == 0 {
		return nil
	}

	key := s.objectKey()
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:          aws.String(s.bucket),
		Key:             aws.String(key),
		Body:            bytes.NewReader(buf.Bytes()),
		ContentType:     aws.String("application/gzip"),
		ContentEncoding: aws.String("gzip"),
	})
	if err != nil {
		return fmt.Errorf("siem s3 sink: put object s3://%s/%s: %w", s.bucket, key, err)
	}
	level.Info(*s.log).Log("msg", "siem s3 sink wrote object", "bucket", s.bucket, "key", key, "lines", lines) // nolint:errcheck
	return nil
}

// objectKey builds a Hive-style time-partitioned key with a random suffix so
// concurrent Lambda invocations never collide.
func (s *s3Sink) objectKey() string {
	now := time.Now().UTC()
	var rnd [8]byte
	_, _ = rand.Read(rnd[:])
	part := now.Format("year=2006/month=01/day=02/hour=15")
	name := fmt.Sprintf("%d_%s.log.gz", now.UnixNano(), hex.EncodeToString(rnd[:]))
	if s.prefix != "" {
		return fmt.Sprintf("%s/%s/%s", s.prefix, part, name)
	}
	return fmt.Sprintf("%s/%s", part, name)
}

// multiSink fans a batch to several Client sinks. The FIRST sink is authoritative
// for retry/error semantics (typically the Loki promtailClient): its error is
// returned so the Lambda's existing retry/DLQ behavior is unchanged. Secondary
// sinks (e.g. the SIEM S3 sink) are best-effort — their failures are logged but
// do NOT fail the invocation, so a SIEM-side problem cannot block Loki delivery.
type multiSink struct {
	primary   Client
	secondary []Client
	log       *log.Logger
}

func (m *multiSink) sendToPromtail(ctx context.Context, b *batch) error {
	// Primary first — its error governs retry/DLQ.
	err := m.primary.sendToPromtail(ctx, b)

	for _, sink := range m.secondary {
		if serr := sink.sendToPromtail(ctx, b); serr != nil {
			level.Error(*m.log).Log("msg", "secondary sink failed (best-effort, not failing invocation)", "err", serr) // nolint:errcheck
		}
	}

	return err
}
