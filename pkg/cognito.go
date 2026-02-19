package main

import (
	"encoding/json"
	"time"

	"github.com/grafana/loki/v3/pkg/logproto"
)

// Parses a Cognito Record and returns a logproto.Entry
func parseCognitoRecord(record Record) (logproto.Entry, error) {
	timestamp := time.Now()
	if record.Error != nil {
		return logproto.Entry{}, record.Error
	}
	document, err := json.Marshal(record.Content)
	if err != nil {
		return logproto.Entry{}, err
	}
	if val, ok := record.Content["eventTimestamp"]; ok {
		sec, nsec, err := getUnixSecNsec(val.(string))
		if err != nil {
			return logproto.Entry{}, err
		}

		timestamp = time.Unix(sec, nsec).UTC()
	}
	return logproto.Entry{
		Line:      string(document),
		Timestamp: timestamp,
	}, nil
}
