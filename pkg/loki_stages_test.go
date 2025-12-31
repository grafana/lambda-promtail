package main

import (
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/loki/pkg/push"
	"github.com/grafana/loki/v3/clients/pkg/logentry/stages"
	"github.com/grafana/loki/v3/clients/pkg/promtail/api"
	"github.com/grafana/loki/v3/pkg/logproto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParsePipelineConfigs validates that the parser is able to handle common stage configurations
func TestParsePipelineConfigs(t *testing.T) {
	logger := log.NewNopLogger()
	registerer := prometheus.NewRegistry()

	tests := []struct {
		name         string
		configJSON   string
		logger       log.Logger
		registerer   prometheus.Registerer
		wantErr      bool
		expectedSize int
		errContains  string
	}{
		{
			name:         "should return pipeline with empty stages when configJSON is empty",
			configJSON:   "",
			logger:       logger,
			registerer:   registerer,
			wantErr:      false,
			expectedSize: 0,
		},
		{
			name:        "should return error when configJSON is whitespace",
			configJSON:  "   ",
			logger:      logger,
			registerer:  registerer,
			wantErr:     true,
			errContains: "failed to parse LOKI_STAGE_CONFIGS",
		},
		{
			name: "should parse valid JSON array with multiple pipeline stages",
			configJSON: `[
			{
				"json": {
					"expressions": {
						"output": "input"
					}
				}
			},
			{
				"labels": {
					"level": "info"
				}
			}
		]`,
			logger:       logger,
			registerer:   registerer,
			wantErr:      false,
			expectedSize: 2,
		},
		{
			name: "should parse valid JSON array with single stage",
			configJSON: `[
			{
				"labels": {
					"level": "info"
				}
			}
		]`,
			logger:       logger,
			registerer:   registerer,
			wantErr:      false,
			expectedSize: 1,
		},
		{
			name: "should handle an invalid stage name",
			configJSON: `[
			{
				"invalid": {
					"expressions": {
						"output": "input"
					}
				}
			}
		]`,
			logger:     logger,
			registerer: registerer,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := ParsePipelineConfigs(tt.configJSON, tt.logger, tt.registerer)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, pipeline)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, pipeline)
				require.Equal(t, tt.expectedSize, pipeline.Size())
			}
		})
	}
}

// TestLokiStages_Process validates the Process function executes correctly with relevant configured stages
func TestLokiStages_Process(t *testing.T) {
	logger := log.NewNopLogger()
	registerer := prometheus.NewRegistry()

	tests := []struct {
		name           string
		configJSON     string
		inputEntry     stages.Entry
		validateOutput func(t *testing.T, output stages.Entry)
		expectDropped  bool
	}{
		{
			name:       "no stage makes no changes to the input",
			configJSON: ``,
			inputEntry: stages.Entry{
				Extracted: map[string]interface{}{},
				Entry: api.Entry{
					Labels: model.LabelSet{},
					Entry: logproto.Entry{
						Timestamp: time.Now(),
						Line:      `{"level":"info","msg":"test message"}`,
					},
				},
			},
			validateOutput: func(t *testing.T, output stages.Entry) {
				assert.Equal(t, `{"level":"info","msg":"test message"}`, output.Line)
				assert.Equal(t, output.Labels, model.LabelSet{})
				assert.Equal(t, output.Extracted, map[string]interface{}{})
			},
		},
		{
			name: "json stage extracts fields from JSON line",
			configJSON: `[
				{
					"json": {
						"expressions": {
							"level": "level",
							"message": "msg"
						}
					}
				}
			]`,
			inputEntry: stages.Entry{
				Extracted: map[string]interface{}{},
				Entry: api.Entry{
					Labels: model.LabelSet{},
					Entry: logproto.Entry{
						Timestamp: time.Now(),
						Line:      `{"level":"info","msg":"test message"}`,
					},
				},
			},
			validateOutput: func(t *testing.T, output stages.Entry) {
				assert.Equal(t, "info", output.Extracted["level"])
				assert.Equal(t, "test message", output.Extracted["message"])
			},
		},
		{
			name: "labels stage adds labels from extracted fields",
			configJSON: `[
				{
					"labels": {
						"level": "level",
						"service": "service_name"
					}
				}
			]`,
			inputEntry: stages.Entry{
				Extracted: map[string]interface{}{
					"level":        "error",
					"service_name": "api",
				},
				Entry: api.Entry{
					Labels: model.LabelSet{},
					Entry: logproto.Entry{
						Timestamp: time.Now(),
						Line:      "test log line",
					},
				},
			},
			validateOutput: func(t *testing.T, output stages.Entry) {
				labels := output.Entry.Labels
				assert.Equal(t, model.LabelValue("error"), labels["level"])
				assert.Equal(t, model.LabelValue("api"), labels["service"])
			},
		},
		{
			name: "structured_metadata stage adds structured metadata from log lines",
			configJSON: `[
			    {
					"json": {
						"expressions": {
							"level": "level",
							"message": "msg"
						}
					}
				},
				{
					"structured_metadata": {
						"level": null,
						"msg": "message"
					}
				}
			]`,
			inputEntry: stages.Entry{
				Extracted: map[string]interface{}{
					"env":        "production",
					"aws_region": "us-east-1",
				},
				Entry: api.Entry{
					Labels: model.LabelSet{},
					Entry: logproto.Entry{
						Timestamp: time.Now(),
						Line:      `{"level":"critical","msg":"a_message"}`,
					},
				},
			},
			validateOutput: func(t *testing.T, output stages.Entry) {
				assert.NotNil(t, output)
				assert.NotEmpty(t, output.Line)
				assert.Contains(t, output.StructuredMetadata, push.LabelAdapter{Name: "level", Value: "critical"})
				assert.Contains(t, output.StructuredMetadata, push.LabelAdapter{Name: "msg", Value: "a_message"})
			},
		},
		{
			name: "regex stage extracts fields using regex",
			configJSON: `[
				{
					"regex": {
						"expression": "^(?P<timestamp>\\d{4}-\\d{2}-\\d{2}) (?P<level>\\w+) (?P<message>.*)$"
					}
				}
			]`,
			inputEntry: stages.Entry{
				Extracted: map[string]interface{}{},
				Entry: api.Entry{
					Labels: model.LabelSet{},
					Entry: logproto.Entry{
						Timestamp: time.Now(),
						Line:      "2024-01-15 ERROR Something went wrong",
					},
				},
			},
			validateOutput: func(t *testing.T, output stages.Entry) {
				assert.Equal(t, "2024-01-15", output.Extracted["timestamp"])
				assert.Equal(t, "ERROR", output.Extracted["level"])
				assert.Equal(t, "Something went wrong", output.Extracted["message"])
			},
		},
		{
			name: "combined stages: json then labels",
			configJSON: `[
				{
					"json": {
						"expressions": {
							"severity": "level",
							"app": "application"
						}
					}
				},
				{
					"labels": {
						"severity": "severity",
						"app": "app"
					}
				}
			]`,
			inputEntry: stages.Entry{
				Extracted: map[string]interface{}{},
				Entry: api.Entry{
					Labels: model.LabelSet{},
					Entry: logproto.Entry{
						Timestamp: time.Now(),
						Line:      `{"level":"warning","application":"webapp"}`,
					},
				},
			},
			validateOutput: func(t *testing.T, output stages.Entry) {
				assert.Equal(t, "warning", output.Extracted["severity"])
				assert.Equal(t, "webapp", output.Extracted["app"])
				labels := output.Entry.Labels
				assert.Equal(t, model.LabelValue("warning"), labels["severity"])
				assert.Equal(t, model.LabelValue("webapp"), labels["app"])
			},
		},
		{
			name: "drop stage filters out entries longer than threshold",
			configJSON: `[
				{
					"drop": {
						"longer_than": 50
					}
				}
			]`,
			inputEntry: stages.Entry{
				Extracted: map[string]interface{}{},
				Entry: api.Entry{
					Labels: model.LabelSet{},
					Entry: logproto.Entry{
						Timestamp: time.Now(),
						Line:      "This is a very long log line that exceeds the 50 character threshold and should be dropped",
					},
				},
			},
			validateOutput: nil,
			expectDropped:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := ParsePipelineConfigs(tt.configJSON, logger, registerer)
			require.NoError(t, err)
			require.NotNil(t, pipeline)

			outputEntry := pipeline.Process(tt.inputEntry)

			// Validate output if not dropped
			if tt.expectDropped {
				require.Equal(t, stages.Entry{}, outputEntry)
			}
			if tt.validateOutput != nil {
				tt.validateOutput(t, outputEntry)
			}
		})
	}
}
