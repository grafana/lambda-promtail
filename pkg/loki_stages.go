package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/loki/v3/clients/pkg/logentry/stages"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	JobName = "LAMBDA-PROMTAIL"
)

type LokiStages struct {
	lokiStages []stages.Stage
	logger     log.Logger
}

// ParsePipelineConfigs parses the LOKI_STAGE_CONFIGS environment variable
func ParsePipelineConfigs(configJSON string, log log.Logger, registerer prometheus.Registerer) (*LokiStages, error) {
	if configJSON == "" {
		return NewLokiStages(log, []map[string]any{}, &JobName, registerer)
	}

	var pipelineStages []map[string]any
	if err := json.Unmarshal([]byte(configJSON), &pipelineStages); err != nil {
		return nil, fmt.Errorf("failed to parse LOKI_STAGE_CONFIGS: %w", err)
	}

	return NewLokiStages(log, pipelineStages, &JobName, registerer)
}

// NewLokiStages creates a new instance of LokiStages
func NewLokiStages(logger log.Logger, stgs []map[string]any, jobName *string, registerer prometheus.Registerer) (*LokiStages, error) {
	st := []stages.Stage{}
	for _, s := range stgs {
		for name, config := range s {
			newStage, err := stages.New(logger, jobName, name, config, registerer)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid %s stage config", name)
			}
			st = append(st, newStage)
		}
	}
	return &LokiStages{
		lokiStages: st,
		logger:     logger,
	}, nil
}

// Process processes a single entry synchronously
func (s *LokiStages) Process(entry stages.Entry) stages.Entry {
	timeout := 1 * time.Second
	if pipelineTimeout != 0 {
		timeout = pipelineTimeout
	}
	for _, m := range s.lokiStages {
		processor, ok := m.(stages.Processor)
		if ok {
			processor.Process(entry.Labels, entry.Extracted, &entry.Timestamp, &entry.Line)
		} else {
			// Use asynchronous Run method for stages that don't implement Processor
			inputChan := make(chan stages.Entry, 1)
			outputChan := m.Run(inputChan)

			// Send entry to the input channel
			inputChan <- entry
			close(inputChan)

			// Wait for processed entry with timeout
			select {
			case processedEntry, ok := <-outputChan:
				if !ok {
					// Channel closed, entry was dropped
					return stages.Entry{}
				}
				entry = processedEntry
			case <-time.After(timeout):
				level.Warn(s.logger).Log("err", "timed out whilst processing log line") // nolint:errcheck
				return stages.Entry{}
			}
		}
	}
	return entry
}

// Size outputs the number of configured stages
func (s *LokiStages) Size() int {
	return len(s.lokiStages)
}
