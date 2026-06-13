package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/loki/v3/pkg/logproto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

// TestDropStageViaLokiStageConfigs proves that a drop stage configured purely
// through the LOKI_STAGE_CONFIGS env var filters out matching log lines before
// they reach a Loki stream — the deployable, env-var-driven content filtering
// path this project relies on instead of the removed __log_line__ relabel hack.
func TestDropStageViaLokiStageConfigs(t *testing.T) {
	origBatch := batchSize
	t.Cleanup(func() { batchSize = origBatch })
	batchSize = 131072

	// Exactly how the Lambda reads it in main.go (os.Getenv("LOKI_STAGE_CONFIGS")).
	t.Setenv("LOKI_STAGE_CONFIGS", `[{"drop":{"expression":"^END RequestId:.*"}}]`)

	pipeline, err := ParsePipelineConfigs(os.Getenv("LOKI_STAGE_CONFIGS"), log.NewNopLogger(), prometheus.NewRegistry())
	require.NoError(t, err)

	b, err := newBatch(context.Background(), nil, pipeline)
	require.NoError(t, err)

	labels := model.LabelSet{model.LabelName("job"): model.LabelValue("test")}

	require.NoError(t, b.add(context.Background(), entry{labels: labels.Clone(), entry: logproto.Entry{
		Line: "END RequestId: abc-123", Timestamp: time.Now(),
	}}))
	require.NoError(t, b.add(context.Background(), entry{labels: labels.Clone(), entry: logproto.Entry{
		Line: "Hello World", Timestamp: time.Now(),
	}}))

	total := 0
	for _, s := range b.streams {
		total += len(s.Entries)
		for _, e := range s.Entries {
			require.NotContains(t, e.Line, "END RequestId:")
		}
	}
	require.Equal(t, 1, total, "noisy line must be dropped, clean line kept")
}
