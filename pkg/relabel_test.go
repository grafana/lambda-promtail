package main

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/stretchr/testify/require"

	"github.com/grafana/loki/v3/pkg/logproto"
	"github.com/grafana/regexp"
)

func TestParseRelabelConfigs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []*relabel.Config
		wantErr bool
	}{
		{
			name:    "empty input",
			input:   "",
			want:    nil,
			wantErr: false,
		},
		{
			name:  "default config",
			input: `[{"target_label": "new_label"}]`,
			want: []*relabel.Config{
				{
					TargetLabel: "new_label",
					Action:      relabel.Replace,
					Regex:       relabel.Regexp{Regexp: regexp.MustCompile("(.*)")},
					Replacement: "$1",
				},
			},
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   "invalid json",
			wantErr: true,
		},
		{
			name: "valid single config",
			input: `[{
				"source_labels": ["__name__"],
				"regex": "my_metric_.*",
				"target_label": "new_label",
				"replacement": "foo",
				"action": "replace"
			}]`,
			wantErr: false,
		},
		{
			name: "invalid regex",
			input: `[{
				"source_labels": ["__name__"],
				"regex": "[[invalid regex",
				"target_label": "new_label",
				"action": "replace"
			}]`,
			wantErr: true,
		},
		{
			name: "multiple valid configs",
			input: `[
				{
					"source_labels": ["__name__"],
					"regex": "my_metric_.*",
					"target_label": "new_label",
					"replacement": "foo",
					"action": "replace"
				},
				{
					"source_labels": ["label1", "label2"],
					"separator": ";",
					"regex": "val1;val2",
					"target_label": "combined",
					"action": "replace"
				}
			]`,
			wantErr: false,
		},
		{
			name: "invalid action",
			input: `[{
				"source_labels": ["__name__"],
				"regex": "my_metric_.*",
				"target_label": "new_label",
				"action": "invalid_action"
			}]`,
			wantErr: false,
		},
		{
			name: "labeldrop",
			input: `[{
				"regex": "__aws_.*",
				"action": "labeldrop"
			}]`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRelabelConfigs(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tt.input == "" {
				require.Nil(t, got)
				return
			}

			require.NotNil(t, got)
			// For valid configs, verify they can be used for relabeling
			// This implicitly tests that the conversion was successful
			if len(got) > 0 {
				for _, cfg := range got {
					require.NotNil(t, cfg)
					require.NotEmpty(t, cfg.Action)
				}
			}
		})
	}
}

func setupBatchTestGlobals(t *testing.T) {
	t.Helper()
	origBatchSize := batchSize
	origRelabelConfigs := relabelConfigs
	t.Cleanup(func() {
		batchSize = origBatchSize
		relabelConfigs = origRelabelConfigs
	})
	batchSize = 131072
}

func TestBatchAddDropsLogLinesViaRelabelConfig(t *testing.T) {
	setupBatchTestGlobals(t)

	configs, err := parseRelabelConfigs(`[{"source_labels":["__log_line__"],"regex":"END RequestId:.*","action":"drop"}]`)
	require.NoError(t, err)
	relabelConfigs = configs

	b := &batch{
		streams:   map[string]*logproto.Stream{},
		processor: &LokiStages{},
	}
	ctx := context.Background()
	labels := model.LabelSet{
		model.LabelName("job"): model.LabelValue("test"),
	}

	// Entry matching the drop pattern should be dropped
	err = b.add(ctx, entry{labels: labels.Clone(), entry: logproto.Entry{
		Line:      "END RequestId: abc-123",
		Timestamp: time.Now(),
	}})
	require.NoError(t, err)

	// Entry not matching the drop pattern should be kept
	err = b.add(ctx, entry{labels: labels.Clone(), entry: logproto.Entry{
		Line:      "Hello World",
		Timestamp: time.Now(),
	}})
	require.NoError(t, err)

	require.Len(t, b.streams, 1)
	for _, stream := range b.streams {
		require.Len(t, stream.Entries, 1)
		require.Equal(t, "Hello World", stream.Entries[0].Line)
	}
}

func TestBatchAddKeepsEntriesWithNoRelabelConfig(t *testing.T) {
	setupBatchTestGlobals(t)
	relabelConfigs = nil

	b := &batch{
		streams:   map[string]*logproto.Stream{},
		processor: &LokiStages{},
	}
	ctx := context.Background()
	labels := model.LabelSet{
		model.LabelName("job"): model.LabelValue("test"),
	}

	err := b.add(ctx, entry{labels: labels.Clone(), entry: logproto.Entry{
		Line:      "END RequestId: abc-123",
		Timestamp: time.Now(),
	}})
	require.NoError(t, err)

	err = b.add(ctx, entry{labels: labels.Clone(), entry: logproto.Entry{
		Line:      "Hello World",
		Timestamp: time.Now(),
	}})
	require.NoError(t, err)

	require.Len(t, b.streams, 1)
	for _, stream := range b.streams {
		require.Len(t, stream.Entries, 2)
	}
}

func TestBatchAddKeepActionFiltersLogLines(t *testing.T) {
	setupBatchTestGlobals(t)

	// Keep only lines starting with "REPORT"
	configs, err := parseRelabelConfigs(`[{"source_labels":["__log_line__"],"regex":"REPORT.*","action":"keep"}]`)
	require.NoError(t, err)
	relabelConfigs = configs

	b := &batch{
		streams:   map[string]*logproto.Stream{},
		processor: &LokiStages{},
	}
	ctx := context.Background()
	labels := model.LabelSet{
		model.LabelName("job"): model.LabelValue("test"),
	}

	err = b.add(ctx, entry{labels: labels.Clone(), entry: logproto.Entry{
		Line:      "START RequestId: abc-123",
		Timestamp: time.Now(),
	}})
	require.NoError(t, err)

	err = b.add(ctx, entry{labels: labels.Clone(), entry: logproto.Entry{
		Line:      "REPORT RequestId: abc-123 Duration: 100ms",
		Timestamp: time.Now(),
	}})
	require.NoError(t, err)

	err = b.add(ctx, entry{labels: labels.Clone(), entry: logproto.Entry{
		Line:      "END RequestId: abc-123",
		Timestamp: time.Now(),
	}})
	require.NoError(t, err)

	require.Len(t, b.streams, 1)
	for _, stream := range b.streams {
		require.Len(t, stream.Entries, 1)
		require.Equal(t, "REPORT RequestId: abc-123 Duration: 100ms", stream.Entries[0].Line)
	}
}

func TestBatchAddLogLineLabelNotInOutput(t *testing.T) {
	setupBatchTestGlobals(t)

	// Use a replace action so entries are kept, not dropped
	configs, err := parseRelabelConfigs(`[{"source_labels":["__log_line__"],"regex":"(.*)","target_label":"extracted","action":"replace"}]`)
	require.NoError(t, err)
	relabelConfigs = configs

	b := &batch{
		streams:   map[string]*logproto.Stream{},
		processor: &LokiStages{},
	}
	ctx := context.Background()
	labels := model.LabelSet{
		model.LabelName("job"): model.LabelValue("test"),
	}

	err = b.add(ctx, entry{labels: labels.Clone(), entry: logproto.Entry{
		Line:      "test log line",
		Timestamp: time.Now(),
	}})
	require.NoError(t, err)

	require.Len(t, b.streams, 1)
	for streamLabels := range b.streams {
		require.NotContains(t, streamLabels, reservedLabelLogLine)
	}
}

func TestBatchAddLabelOnlyRelabelStillWorks(t *testing.T) {
	setupBatchTestGlobals(t)

	// Drop entries where label "env" equals "debug"
	configs, err := parseRelabelConfigs(`[{"source_labels":["env"],"regex":"debug","action":"drop"}]`)
	require.NoError(t, err)
	relabelConfigs = configs

	b := &batch{
		streams:   map[string]*logproto.Stream{},
		processor: &LokiStages{},
	}
	ctx := context.Background()

	// Entry with env=debug should be dropped
	err = b.add(ctx, entry{
		labels: model.LabelSet{
			model.LabelName("job"): model.LabelValue("test"),
			model.LabelName("env"): model.LabelValue("debug"),
		},
		entry: logproto.Entry{
			Line:      "debug log",
			Timestamp: time.Now(),
		},
	})
	require.NoError(t, err)

	// Entry with env=production should be kept
	err = b.add(ctx, entry{
		labels: model.LabelSet{
			model.LabelName("job"): model.LabelValue("test"),
			model.LabelName("env"): model.LabelValue("production"),
		},
		entry: logproto.Entry{
			Line:      "production log",
			Timestamp: time.Now(),
		},
	})
	require.NoError(t, err)

	require.Len(t, b.streams, 1)
	for _, stream := range b.streams {
		require.Len(t, stream.Entries, 1)
		require.Equal(t, "production log", stream.Entries[0].Line)
	}
}

func TestBatchAddDropMultiplePatterns(t *testing.T) {
	setupBatchTestGlobals(t)

	// Drop lines matching "END RequestId:" OR "START RequestId:"
	configs, err := parseRelabelConfigs(`[{"source_labels":["__log_line__"],"regex":"(END|START) RequestId:.*","action":"drop"}]`)
	require.NoError(t, err)
	relabelConfigs = configs

	b := &batch{
		streams:   map[string]*logproto.Stream{},
		processor: &LokiStages{},
	}
	ctx := context.Background()
	labels := model.LabelSet{
		model.LabelName("job"): model.LabelValue("test"),
	}

	lines := []string{
		"START RequestId: abc-123",
		"Processing request...",
		"END RequestId: abc-123",
		"REPORT RequestId: abc-123 Duration: 50ms",
	}

	for _, line := range lines {
		err = b.add(ctx, entry{labels: labels.Clone(), entry: logproto.Entry{
			Line:      line,
			Timestamp: time.Now(),
		}})
		require.NoError(t, err)
	}

	require.Len(t, b.streams, 1)
	for _, stream := range b.streams {
		require.Len(t, stream.Entries, 2)
		require.Equal(t, "Processing request...", stream.Entries[0].Line)
		require.Equal(t, "REPORT RequestId: abc-123 Duration: 50ms", stream.Entries[1].Line)
	}
}
