package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"
)

// rawEvent reads a fixture and unmarshals it into the map[string]interface{}
// shape the Lambda runtime hands to handler / checkEventType.
func rawEvent(t *testing.T, path string) map[string]any {
	t.Helper()
	bs, err := os.ReadFile(path)
	require.NoError(t, err)
	ev := make(map[string]any)
	require.NoError(t, json.Unmarshal(bs, &ev))
	return ev
}

func Test_checkEventType(t *testing.T) {
	// The fixtures under testdata/events are copied verbatim from the
	// aws-lambda-go SDK's own testdata, except s3-test-event.json and
	// s3-event-generated-tags.json which model the S3 test notification and the
	// eventVersion 2.5 event (with the awsGeneratedTags field) respectively.
	tests := []struct {
		name string
		file string
		want any
	}{
		{
			name: "s3 event",
			file: "../testdata/events/s3-event.json",
			want: &events.S3Event{},
		},
		{
			name: "sqs event",
			file: "../testdata/events/sqs-event.json",
			want: &events.SQSEvent{},
		},
		{
			name: "sns event",
			file: "../testdata/events/sns-event.json",
			want: &events.SNSEvent{},
		},
		{
			name: "kinesis event",
			file: "../testdata/events/kinesis-event.json",
			want: &events.KinesisEvent{},
		},
		{
			name: "cloudwatch logs event",
			file: "../testdata/events/cloudwatch-logs-event.json",
			want: &events.CloudwatchLogsEvent{},
		},
		{
			name: "eventbridge event",
			file: "../testdata/eventbridge-s3-event.json",
			want: &events.CloudWatchEvent{},
		},
		{
			name: "s3 test event",
			file: "../testdata/events/s3-test-event.json",
			want: &events.S3TestEvent{},
		},
		{
			// Regression for https://github.com/grafana/lambda-promtail/issues/193:
			// the new awsGeneratedTags field (eventVersion 2.5) must not break
			// detection now that we no longer reject unknown fields.
			name: "s3 event with awsGeneratedTags",
			file: "../testdata/events/s3-event-generated-tags.json",
			want: &events.S3Event{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checkEventType(rawEvent(t, tt.file))
			require.NoError(t, err)
			require.IsType(t, tt.want, got)
		})
	}
}

// Test_checkEventType_ignoresUnknownFields asserts the specific behaviour from
// the S3 docs: consumers should ignore fields they do not recognise. The event
// still decodes into an S3Event and the known fields survive.
func Test_checkEventType_ignoresUnknownFields(t *testing.T) {
	got, err := checkEventType(rawEvent(t, "../testdata/events/s3-event-generated-tags.json"))
	require.NoError(t, err)

	s3Event, ok := got.(*events.S3Event)
	require.True(t, ok)
	require.Len(t, s3Event.Records, 1)

	record := s3Event.Records[0]
	require.Equal(t, "2.5", record.EventVersion)
	require.Equal(t, eventSourceS3, record.EventSource)
	require.Equal(t, "aws-landing-zone-logs-000000000000-eu-central-1", record.S3.Bucket.Name)
}

func Test_checkEventType_unknownEvent(t *testing.T) {
	_, err := checkEventType(map[string]any{"foo": "bar"})
	require.Error(t, err)
}

func Test_checkEventType_unknownRecordSource(t *testing.T) {
	ev := map[string]any{
		"Records": []any{
			map[string]any{"eventSource": "aws:dynamodb"},
		},
	}
	_, err := checkEventType(ev)
	require.Error(t, err)
}
