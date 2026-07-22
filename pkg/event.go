package main

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
)

// AWS records their event source in the `eventSource` (or, for SNS, the
// PascalCase `EventSource`) field of each record. aws-lambda-go does not export
// these as constants, so we define the ones we route on here. The values are
// documented in each service's event structure reference, e.g. the S3 event
// notification content structure:
// https://docs.aws.amazon.com/AmazonS3/latest/userguide/notification-content-structure.html
const (
	eventSourceS3      = "aws:s3"
	eventSourceSQS     = "aws:sqs"
	eventSourceSNS     = "aws:sns"
	eventSourceKinesis = "aws:kinesis"
)

// checkEventType inspects a raw Lambda event and returns a pointer to the
// concrete aws-lambda-go event type it represents.
func checkEventType(ev map[string]any) (any, error) {
	target, err := eventTarget(ev)
	if err != nil {
		return nil, err
	}

	j, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(j, target); err != nil {
		return nil, err
	}
	return target, nil
}

// eventTarget selects the concrete event struct to decode into based on the
// event's discriminating fields.
func eventTarget(ev map[string]any) (any, error) {
	switch {
	case hasKey(ev, "awslogs"):
		return &events.CloudwatchLogsEvent{}, nil
	case hasKey(ev, "detail-type") || hasKey(ev, "detail"):
		return &events.CloudWatchEvent{}, nil
	case hasKey(ev, "Records"):
		return recordEventTarget(ev)
	case isS3TestEvent(ev):
		return &events.S3TestEvent{}, nil
	}
	return nil, fmt.Errorf("unknown event type: %v", ev)
}

// recordEventTarget selects the event struct for the Records-based envelopes
// (S3, SQS, SNS, Kinesis), which are indistinguishable at the top level and are
// told apart by the event source of their first record.
func recordEventTarget(ev map[string]any) (any, error) {
	source := firstRecordEventSource(ev)
	switch source {
	case eventSourceS3:
		return &events.S3Event{}, nil
	case eventSourceSQS:
		return &events.SQSEvent{}, nil
	case eventSourceSNS:
		return &events.SNSEvent{}, nil
	case eventSourceKinesis:
		return &events.KinesisEvent{}, nil
	}
	return nil, fmt.Errorf("unknown record event source: %q", source)
}

// firstRecordEventSource returns the event source of the first record, checking
// both the camelCase `eventSource` (S3, SQS, Kinesis) and PascalCase
// `EventSource` (SNS) spellings.
func firstRecordEventSource(ev map[string]any) string {
	records, ok := ev["Records"].([]any)
	if !ok || len(records) == 0 {
		return ""
	}
	record, ok := records[0].(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"eventSource", "EventSource"} {
		if v, ok := record[key].(string); ok {
			return v
		}
	}
	return ""
}

// isS3TestEvent reports whether the event is the test notification S3 sends when
// a bucket notification is first configured. See:
// https://docs.aws.amazon.com/AmazonS3/latest/userguide/notification-content-structure.html
func isS3TestEvent(ev map[string]any) bool {
	event, _ := ev["Event"].(string)
	return event == "s3:TestEvent"
}

func hasKey(ev map[string]any, key string) bool {
	_, ok := ev[key]
	return ok
}
