// Push a real ALB log .gz file to a local Loki instance for testing.
// Usage: go run dev/push-alb-logs.go <path-to-alb-log.gz>
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const lokiURL = "http://localhost:3100/loki/api/v1/push"

type albLogEntry struct {
	Timestamp   string `json:"timestamp"`
	ALBName     string `json:"alb_name"`
	RemAddr     string `json:"apache_rem_addr"`
	ReqMethod   string `json:"apache_req_method"`
	ReqURI      string `json:"apache_req_uri"`
	Status      string `json:"apache_status"`
	RespBlen    string `json:"apache_resp_blen"`
	UserAgent   string `json:"apache_user_agent"`
	TargetGroup string `json:"alb_target_group"`
	Host        string `json:"apachex_host"`
	UpstreamRT  string `json:"apachex_URT"`
	SSLProtocol string `json:"alb_ssl_protocol"`
}

func albLogLineToJSON(logLine string) (string, error) {
	fields := strings.Fields(logLine)
	if len(fields) < 18 {
		return logLine, nil
	}
	reqMethod := strings.TrimPrefix(fields[12], "\"")
	reqURI := fields[13]
	userAgent := strings.Trim(fields[15], "\"")
	sslProtocol := fields[17]
	targetGroup := fields[18]
	host := strings.Trim(fields[20], "\"")
	entry := albLogEntry{
		Timestamp:   fields[1],
		ALBName:     fields[2],
		RemAddr:     strings.SplitN(fields[3], ":", 2)[0],
		ReqMethod:   reqMethod,
		ReqURI:      reqURI,
		Status:      fields[8],
		RespBlen:    fields[11],
		UserAgent:   userAgent,
		TargetGroup: targetGroup,
		Host:        host,
		UpstreamRT:  fields[6],
		SSLProtocol: sslProtocol,
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return logLine, err
	}
	return string(b), nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run dev/push-alb-logs.go <file.log.gz>")
		os.Exit(1)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		panic(err)
	}
	defer gz.Close()

	raw, err := io.ReadAll(gz)
	if err != nil {
		panic(err)
	}

	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	fmt.Printf("Pushing %d lines to %s (dropping 200/204/301/302)\n", len(lines), lokiURL)

	type lokiValue [2]string
	type lokiStream struct {
		Stream map[string]string `json:"stream"`
		Values []lokiValue       `json:"values"`
	}
	type lokiPayload struct {
		Streams []lokiStream `json:"streams"`
	}

	// Status codes to drop before ingestion (mirrors LOKI_STAGE_CONFIGS drop stage)
	dropStatuses := map[string]bool{"200": true, "204": true, "301": true, "302": true}

	var skipped int
	var values []lokiValue
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Drop by ALB status code field (field 8) before transformation
		fields := strings.Fields(line)
		if len(fields) > 8 && dropStatuses[fields[8]] {
			skipped++
			continue
		}
		jsonLine, err := albLogLineToJSON(line)
		if err != nil {
			jsonLine = line
		}
		ts := strconv.FormatInt(time.Now().UnixNano(), 10)
		values = append(values, lokiValue{ts, jsonLine})
	}

	payload := lokiPayload{
		Streams: []lokiStream{
			{
				Stream: map[string]string{
					"__aws_log_type": "s3_lb",
					"env":            "local",
					"service":        "alb",
				},
				Values: values,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}

	resp, err := http.Post(lokiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("Skipped %d lines (200/204/301/302)\n", skipped)
	fmt.Printf("Loki response: %d %s\n", resp.StatusCode, string(respBody))
}
