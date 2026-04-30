package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/prometheus/common/model"

	"github.com/grafana/loki/v3/pkg/logproto"
)

func fetchTagsForLogGroup(logGroupName string) (map[string]string, error) {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("eu-central-1"))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS configuration: %w", err)
	}

	client := cloudwatchlogs.NewFromConfig(cfg)

	input := &cloudwatchlogs.ListTagsLogGroupInput{
		LogGroupName: aws.String(logGroupName),
	}

	output, err := client.ListTagsLogGroup(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tags for log group %s: %w", logGroupName, err)
	}

	return output.Tags, nil
}

func parseCWEvent(ctx context.Context, b *batch, ev *events.CloudwatchLogsEvent) error {
	data, err := ev.AWSLogs.Parse()
	if err != nil {
		return err
	}

	labels := model.LabelSet{
		model.LabelName("cloudwatch_log_group"): model.LabelValue(data.LogGroup),
	}

	if keepStream {
		labels[model.LabelName("__aws_cloudwatch_log_stream")] = model.LabelValue(data.LogStream)
	}

	tags, err := fetchTagsForLogGroup(data.LogGroup)
	if err == nil {
		project, projectExists := tags["Project"]
		environment, environmentExists := tags["Environment"]
		if projectExists && environmentExists {
			labels[model.LabelName("namespace")] = model.LabelValue(fmt.Sprintf("%s-%s", project, environment))
		}
	}

	if strings.HasPrefix(data.LogGroup, "/aws/lambda/") {
		functionName := strings.TrimPrefix(data.LogGroup, "/aws/lambda/")
		labels[model.LabelName("app")] = model.LabelValue(functionName)
	} else {
		labels[model.LabelName("app")] = model.LabelValue("cloudwatch")
	}

	labels = applyLabels(labels)

	for _, event := range data.LogEvents {
		timestamp := time.UnixMilli(event.Timestamp)

		if len(event.Message) > 0 {
			fields := strings.Fields(event.Message)
			if len(fields) > 2 {
				logLevel := strings.ToLower(fields[2])
				if logLevel == "info" || logLevel == "warn" || logLevel == "error" {
					labels[model.LabelName("level")] = model.LabelValue(logLevel)
				}
			}
		}

		if err := b.add(ctx, entry{labels, logproto.Entry{
			Line:      event.Message,
			Timestamp: timestamp,
		}}); err != nil {
			return err
		}
	}

	return nil
}

func processCWEvent(ctx context.Context, ev *events.CloudwatchLogsEvent, pClient Client, processingPipeline *LokiStages) error {
	batch, err := newBatch(ctx, pClient, processingPipeline)
	if err != nil {
		return err
	}

	err = parseCWEvent(ctx, batch, ev)
	if err != nil {
		return fmt.Errorf("error parsing log event: %s", err)
	}

	err = pClient.sendToPromtail(ctx, batch)
	if err != nil {
		return err
	}

	return nil
}
