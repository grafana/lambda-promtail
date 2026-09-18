package main

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/loki/v3/pkg/logproto"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/grafana/loki/pkg/push"
)

// Test_batch_add_SharedLabelsAcrossBatch reproduces
// https://github.com/grafana/support-escalations/issues/24182: when a Lambda invocation
// receives multiple log events in one batch (e.g. a CloudWatch put-log-events call with several
// events), parseCWEvent/processLogEvents build one labels map and reuse it, by reference, for
// every entry{} in the batch. The structured_metadata stage mutates Entry.Labels in place
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

	batchSize = 131072 // large enough that add() never flushes mid-test

	for i, line := range []string{"first event", "second event", "third event"} {
		err := b.add(context.Background(), entry{sharedLabels, logproto.Entry{
			Line:      line,
			Timestamp: time.Now(),
		}})
		require.NoError(t, err, "event %d", i)
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
