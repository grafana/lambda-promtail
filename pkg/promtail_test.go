package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/dskit/backoff"
	"github.com/grafana/loki/v3/pkg/logproto"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/grafana/loki/pkg/push"
)

// Test_batch_add_SharedLabelsAcrossBatch reproduces a bug where, when a Lambda invocation
// receives multiple log events in one batch (e.g. a CloudWatch put-log-events call with
// several events), parseCWEvent/processLogEvents build one labels map and reuse it, by
// reference, for every entry{} in the batch. The structured_metadata stage mutates Entry.Labels in place
// (deleting a label once it's promoted to structured metadata), so without batch.add cloning
// e.labels first, only the first entry in the batch keeps the field -- every later entry's
// Extracted copy is built from an already-mutated map and never sees the label at all.
func Test_batch_add_SharedLabelsAcrossBatch(t *testing.T) {
	pipeline, err := ParsePipelineConfigs(
		`[{"structured_metadata":{"log_stream":"__aws_cloudwatch_log_stream"}}]`,
		nil, nil,
	)
	require.NoError(t, err)

	sharedLabels := model.LabelSet{
		model.LabelName("__aws_log_type"):              model.LabelValue("cloudwatch"),
		model.LabelName("__aws_cloudwatch_log_group"):  model.LabelValue("testLogGroup"),
		model.LabelName("__aws_cloudwatch_log_stream"): model.LabelValue("testLogStream"),
	}

	b := &batch{
		streams:   map[string]*logproto.Stream{},
		processor: pipeline,
	}

	batchSize = defaultBatchSize // large enough that add() never flushes mid-test

	want := sharedLabels.Clone()

	for i, line := range []string{"first event", "second event", "third event"} {
		err := b.add(context.Background(), entry{sharedLabels, logproto.Entry{
			Line:      line,
			Timestamp: time.Now(),
		}})
		require.NoError(t, err, "event %d", i)
		require.Equal(t, want, sharedLabels, "batch.add mutated caller's labels on event %d", i)
	}

	require.Len(t, b.streams, 1, "all three entries should share the same remaining labels, hence one stream")

	var stream *logproto.Stream
	for _, s := range b.streams {
		stream = s
	}
	require.Len(t, stream.Entries, 3)

	for i, e := range stream.Entries {
		require.Containsf(t, e.StructuredMetadata, push.LabelAdapter{Name: "log_stream", Value: "testLogStream"},
			"entry %d (%q) is missing log_stream structured metadata", i, e.Line)
	}
}

// newTestPromtailClient points the client at an HTTP server that answers every
// push with the given status and counts the requests it receives.
func newTestPromtailClient(t *testing.T, status int, dropPermanentErrors bool) (Client, *atomic.Int32) {
	t.Helper()

	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	prevAddress := writeAddress
	t.Cleanup(func() { writeAddress = prevAddress })
	var err error
	writeAddress, err = url.Parse(srv.URL)
	require.NoError(t, err)

	client := NewPromtailClient(&promtailClientConfig{
		backoff: &backoff.Config{
			MinBackoff: time.Millisecond,
			MaxBackoff: time.Millisecond,
			MaxRetries: 3,
		},
		http:                &httpClientConfig{timeout: time.Second},
		dropPermanentErrors: dropPermanentErrors,
	}, NewLogger("error"))

	return client, &requests
}

func newTestBatch(lines ...string) *batch {
	b := &batch{streams: map[string]*logproto.Stream{}}
	if len(lines) == 0 {
		return b
	}
	stream := &logproto.Stream{Labels: `{job="test"}`}
	for _, line := range lines {
		stream.Entries = append(stream.Entries, logproto.Entry{Timestamp: time.Now(), Line: line})
		b.size += len(line)
	}
	b.streams[stream.Labels] = stream
	return b
}

// An empty batch must not be pushed: Loki answers a push without streams with
// 422, which used to fail the invocation and make Kinesis/SQS retry it forever.
func Test_sendToPromtail_EmptyBatchIsNotSent(t *testing.T) {
	client, requests := newTestPromtailClient(t, http.StatusUnprocessableEntity, false)

	err := client.sendToPromtail(context.Background(), newTestBatch())

	require.NoError(t, err)
	require.Equal(t, int32(0), requests.Load())
}

func Test_sendToPromtail_StatusHandling(t *testing.T) {
	for _, tc := range []struct {
		name                string
		status              int
		dropPermanentErrors bool
		wantErr             bool
		wantRequests        int32
	}{
		{name: "success", status: http.StatusNoContent, wantRequests: 1},
		{name: "4xx returns error by default", status: http.StatusBadRequest, wantErr: true, wantRequests: 1},
		{name: "4xx is dropped when enabled", status: http.StatusBadRequest, dropPermanentErrors: true, wantRequests: 1},
		{name: "422 is dropped when enabled", status: http.StatusUnprocessableEntity, dropPermanentErrors: true, wantRequests: 1},
		{name: "413 is dropped when enabled", status: http.StatusRequestEntityTooLarge, dropPermanentErrors: true, wantRequests: 1},
		{name: "429 is retried and returns error when enabled", status: http.StatusTooManyRequests, dropPermanentErrors: true, wantErr: true, wantRequests: 3},
		{name: "5xx is retried and returns error when enabled", status: http.StatusBadGateway, dropPermanentErrors: true, wantErr: true, wantRequests: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, requests := newTestPromtailClient(t, tc.status, tc.dropPermanentErrors)

			err := client.sendToPromtail(context.Background(), newTestBatch("a log line"))

			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantRequests, requests.Load())
		})
	}
}

// When the last entry of an event crosses batchSize, batch.add flushes the
// batch, so the final send of the event gets an empty batch. That final send
// must not push anything.
func Test_batch_FlushAtBatchSizeDoesNotSendEmptyPush(t *testing.T) {
	client, requests := newTestPromtailClient(t, http.StatusNoContent, false)

	prevBatchSize := batchSize
	t.Cleanup(func() { batchSize = prevBatchSize })
	batchSize = 5

	pipeline, err := ParsePipelineConfigs("", nil, nil)
	require.NoError(t, err)
	b, err := newBatch(context.Background(), client, pipeline)
	require.NoError(t, err)

	err = b.add(context.Background(), entry{
		labels: model.LabelSet{"job": "test"},
		entry:  logproto.Entry{Timestamp: time.Now(), Line: "longer than five bytes"},
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), requests.Load(), "batch.add should flush once the batch crosses batchSize")

	require.NoError(t, client.sendToPromtail(context.Background(), b))
	require.Equal(t, int32(1), requests.Load(), "the final send of the now empty batch must not push")
}
