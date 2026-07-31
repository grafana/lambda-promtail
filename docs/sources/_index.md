---
title: Lambda Promtail
menuTitle: Lambda Promtail
description: Learn how Lambda Promtail forwards AWS logs to Grafana Loki using an AWS Lambda function.
aliases:
- ../clients/lambda-promtail/
weight: 700
---

# Lambda Promtail

Lambda Promtail is an [AWS Lambda](https://aws.amazon.com/lambda/) function that forwards logs from AWS services to [Grafana Loki](/docs/loki/latest/) or to any Loki-push-API-compatible endpoint (Grafana Cloud or Grafana Alloy).
It receives events from AWS log sources, converts them into Loki log entries, and sends them to a Loki write endpoint using the [Loki push API](/docs/loki/latest/reference/api/#push-log-entries-to-loki).

The [lambda-promtail](https://github.com/grafana/lambda-promtail) project is maintained as its own repository, independently versioned from Loki, and provides Terraform and CloudFormation definitions you can deploy directly into your AWS account.

{{< admonition type="note" >}}
If you use Grafana Cloud and want a guided setup with generated Terraform or CloudFormation, API credentials, and your Loki write endpoint filled in for you, use the Cloud Provider Observability [Logs with Lambda](/docs/grafana-cloud/monitor-infrastructure/monitor-cloud-provider/aws/logs/cloudwatch-logs/) workflow instead.
This page covers self-managed deployment: customizing the Terraform or CloudFormation files directly, building the function from source, and advanced patterns such as Amazon Kinesis and relabeling.
{{< /admonition >}}

## How Lambda Promtail works

An AWS log source invokes the Lambda Promtail function with an event.
Lambda Promtail detects the event type, parses the log records, attaches labels, and forwards the entries to the write address that you configure.

The write address is any endpoint compatible with the Loki write API. This can be:

- A Loki instance, such as a self-managed Loki cluster or Grafana Cloud Loki.
- A [Grafana Alloy](/docs/alloy/latest/) collector that receives logs through the Loki push API and forwards them to Loki.

{{< admonition type="note" >}}
The name Lambda Promtail refers to this Lambda function and is unrelated to the standalone Promtail agent, which is deprecated and end-of-life (EOL) as of March 2, 2026.
Where you need a collector between Lambda Promtail and Loki, use Grafana Alloy.
{{< /admonition >}}

## Supported log sources

Lambda Promtail processes events from the following sources:

- **Amazon CloudWatch Logs**: Subscribe a CloudWatch log group to the function with a subscription filter.
- **Amazon Kinesis Data Streams**: Map a Kinesis data stream as an event source, for example to receive CloudFront real-time logs.
- **Amazon S3**: Trigger the function when objects are created in a bucket, either through S3 bucket notifications or through Amazon EventBridge. Lambda Promtail parses the following S3-based log types from the object key:
  - VPC flow logs
  - Application and Network Load Balancer access logs
  - CloudTrail logs
  - CloudFront access logs
  - AWS WAF logs
  - GuardDuty findings
  - Amazon MSK (Kafka) broker logs
  - S3 server access logs
- **Amazon SQS** and **Amazon SNS**: Receive events indirectly. Lambda Promtail extracts the nested source events from the message body and processes them as if they came directly from the source service.

## Deploy Lambda Promtail

You can deploy Lambda Promtail with either Terraform or CloudFormation:

- [Deploy with Terraform](/docs/loki/latest/send-data/lambda-promtail/lambda-promtail-deploy-terraform): Recommended for most deployments. The Terraform configuration accepts arrays of log groups, buckets, and Kinesis streams, and can build the function from source or deploy a prebuilt ZIP archive.
- [Deploy with CloudFormation](/docs/loki/latest/send-data/lambda-promtail/lambda-promtail-deploy-cloudformation): Use the provided templates for CloudWatch, S3 with EventBridge, and Application Load Balancer logs.

For all configuration options, propagated labels, and relabeling, refer to the [Lambda Promtail reference](/docs/loki/latest/send-data/lambda-promtail/lambda-promtail-reference).

Grafana publishes a prebuilt ZIP archive of the function with each [release](https://github.com/grafana/lambda-promtail/releases), which you can deploy with Terraform or reference from CloudFormation.
You can also clone the [lambda-promtail repository](https://github.com/grafana/lambda-promtail), modify the Go code, and build the function yourself.

## Use cases

The following use cases show common scenarios where Lambda Promtail helps you collect AWS logs, along with the approach and trade-offs for each one.
Use them to decide whether Lambda Promtail fits your needs and to choose the deployment pattern that matches your log sources and reliability requirements.

### Monitor ephemeral jobs

Lambda Promtail is an effective way to monitor ephemeral jobs, such as those that run on AWS Lambda, which are otherwise hard to monitor with one of the other Loki [clients](../).

Ephemeral jobs can violate cardinality best practices.
Under high request load, an AWS Lambda function can increase in concurrency and create many log streams in CloudWatch.
For this reason, Lambda Promtail doesn't keep the log stream value as a label by default.
This is possible because current versions of Loki no longer have an ingestion ordering constraint on logs within a single stream.

### Test Loki with existing CloudWatch logs

If you use CloudWatch and want to try Loki in a low-risk way, Lambda Promtail lets you pipe CloudWatch logs to Loki regardless of the event source, such as Amazon EC2, Kubernetes, Lambda, or Amazon ECS, without deploying log collectors across your infrastructure.

For long-term use, running [Grafana Alloy](/docs/alloy/latest/) as a collector on your infrastructure is the recommended strategy for flexibility, reliability, performance, and cost.

{{< admonition type="note" >}}
Forwarding logs from CloudWatch to Loki means that you still pay for CloudWatch.
{{< /admonition >}}

### Trigger Lambda Promtail through SQS

For AWS services that can send messages to Amazon SQS, such as S3 with an S3 notification to SQS, you can process events through an [SQS queue with a Lambda trigger](https://docs.aws.amazon.com/lambda/latest/dg/with-sqs.html) instead of configuring the source service to trigger the function directly.
Lambda Promtail retrieves the nested events from the body of the SQS messages and processes them as if they came directly from the source service.
The same applies to Amazon SNS notifications.

{{< admonition type="note" >}}
The nested payload in each SQS or SNS message must be an event type that Lambda Promtail recognizes, such as an S3 or CloudWatch Logs event.
For EventBridge-sourced events, Lambda Promtail only processes S3 `Object Created` events. Other EventBridge event types are rejected with the error `event bridge event type not supported`.
{{< /admonition >}}

### Recover logs on failure with an SQS dead-letter queue

When you trigger Lambda Promtail through SQS, you can handle on-failure recovery with a secondary SQS queue as a dead-letter queue (DLQ).
Configure Lambda so that unsuccessfully processed messages are sent to the DLQ.
After you fix the issue, you can reprocess the messages by moving them from the DLQ back to the source queue.
For more information, refer to the [Amazon SQS dead-letter queue documentation](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-configure-dead-letter-queue-redrive.html).

### Forward S3 logs with EventBridge

Several AWS services use S3 as their log destination, such as Application Load Balancer, VPC flow logs, and CloudFront access logs.
To forward these logs, you configure S3 to trigger Lambda Promtail when objects are created.

When you use CloudFormation to define your infrastructure, there's a [known issue](https://github.com/aws-cloudformation/cloudformation-coverage-roadmap/issues/79) when you configure an `AWS::S3::BucketNotification` and the resource that the notification triggers in the same stack.
To work around this issue, use [S3 event notifications with EventBridge](https://aws.amazon.com/blogs/aws/new-use-amazon-s3-event-notifications-with-amazon-eventbridge/).
When an object is created in an S3 bucket, S3 sends an event to an EventBridge bus, and you create a rule that routes the event to Lambda Promtail.

The following diagram shows how logs are written from the source service into an S3 bucket.
From there, the S3 bucket sends an `Object Created` notification to the EventBridge `default` bus, where a rule triggers Lambda Promtail.

{{< figure src="https://grafana.com/media/docs/loki/lambda-promtail-with-eventbridge.png" alt="Diagram showing how logs are written from the source service into an S3 bucket and routed to Lambda Promtail through EventBridge" >}}

For deployment steps, refer to [Deploy with CloudFormation](/docs/loki/latest/send-data/lambda-promtail/lambda-promtail-deploy-cloudformation).
