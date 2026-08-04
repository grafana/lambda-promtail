---
title: Deploy with Terraform
menuTitle: Deploy with Terraform
description: Deploy the Lambda Promtail function to AWS with the provided Terraform configuration.
weight: 100
---

# Deploy with Terraform

Deploy Lambda Promtail with the [Terraform](https://www.terraform.io/) configuration in the [lambda-promtail repository](https://github.com/grafana/lambda-promtail).
The main configuration is in [`main.tf`](https://github.com/grafana/lambda-promtail/blob/main/main.tf), and the input variables are defined in [`variables.tf`](https://github.com/grafana/lambda-promtail/blob/main/variables.tf).

The Terraform configuration accepts arrays of Amazon CloudWatch log group names, S3 bucket names, and Amazon Kinesis Data stream names, and can also configure VPC subnets and security groups.

## Before you begin

Make sure that you have the following:

- The [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) configured with permissions to create the required resources.
- [Terraform](https://developer.hashicorp.com/terraform/install) installed.
- A Loki write endpoint, such as a Grafana Cloud Loki URL or a self-managed Loki cluster.

By default, Terraform builds the Go binary from source and packages it as a ZIP archive, so you also need [Go](https://go.dev/doc/install) installed.
To deploy a prebuilt binary instead, download the ZIP archive from the [releases page](https://github.com/grafana/lambda-promtail/releases), upload it to an S3 bucket, and set the `lambda_promtail_binary_bucket` and `lambda_promtail_binary_key` variables.

## Required values

You must set the following value for every deployment:

- `write_address`: A Loki write API compatible endpoint, either Loki or a Grafana Alloy collector.

Depending on your endpoint and event sources, you typically also set:

- `username` and `password`: Basic authentication credentials, required if the write address is a Loki endpoint with authentication.
- `bearer_token`: A bearer token, if your endpoint requires one. You can't set a bearer token together with a username and password.
- One or more of `log_group_names`, `bucket_names`, or `kinesis_stream_name` to define the event sources.

The Terraform configuration passes these values to the function as environment variables.
For the complete list of options, refer to the [Lambda Promtail reference](/docs/loki/latest/send-data/lambda-promtail/lambda-promtail-reference/).

## Set the AWS region

The provider block in `main.tf` uses the AWS provider's default region resolution.
To pin a region, add a `provider` block:

```hcl
provider "aws" {
  region = "us-east-2"
}
```

## Deploy the function

Use `terraform apply` with the variables for your event sources.
Select a highlighted placeholder in the following commands to enter your own value, such as `write_address`, which is your Loki write endpoint.
The value you enter fills in automatically across all the examples on this page.
Grafana Cloud users can find the write endpoint in the Loki details of their stack, for example `https://logs-prod-us-central1.grafana.net/loki/api/v1/push`.

The `log_group_names`, `bucket_names`, and `kinesis_stream_name` variables each accept a list, so you can pass more than one comma-separated value, for example `["log-group-1", "log-group-2"]`.

To forward logs from CloudWatch log groups, run the following command after updating with your values:

```bash
terraform apply \
  -var "write_address=https://@@@LOKI_ENDPOINT@@@/loki/api/v1/push" \
  -var "username=@@@USERNAME@@@" \
  -var "password=@@@PASSWORD@@@" \
  -var 'log_group_names=["@@@LOG_GROUP_NAME@@@"]' \
  -var 'bucket_names=["@@@LOG_SOURCE_BUCKET@@@"]'
```

To deploy a prebuilt binary from S3 instead of building from source:

```bash
terraform apply \
  -var "write_address=https://@@@LOKI_ENDPOINT@@@/loki/api/v1/push" \
  -var "username=@@@USERNAME@@@" \
  -var "password=@@@PASSWORD@@@" \
  -var "lambda_promtail_binary_bucket=@@@S3_BUCKET_NAME@@@" \
  -var "lambda_promtail_binary_key=lambda-promtail.zip"
```

To forward logs from Kinesis data streams, for example Amazon CloudFront real-time logs:

```bash
terraform apply \
  -var "write_address=https://@@@LOKI_ENDPOINT@@@/loki/api/v1/push" \
  -var "username=@@@USERNAME@@@" \
  -var "password=@@@PASSWORD@@@" \
  -var 'kinesis_stream_name=["@@@KINESIS_STREAM_NAME@@@"]'
```

## Set optional variables

Add any of the following variables to `terraform apply` to change the default behavior:

- Keep the log stream label: `-var "keep_stream=true"`
- Add extra labels: `-var 'extra_labels="name1,value1,name2,value2"'`
- Drop labels: `-var 'drop_labels="name1,name2"'`
- Apply relabeling rules: `-var 'relabel_configs=[{"source_labels":["__aws_log_type"],"target_label":"log_type","action":"replace"}]'`
- Set a tenant ID: `-var "tenant_id=<value>"`
- Skip TLS verification for development only: `-var 'skip_tls_verify="true"'`
- Change the batch size in bytes: `-var 'batch_size=131072'`

For details about each option, refer to the [Lambda Promtail reference](/docs/loki/latest/send-data/lambda-promtail/lambda-promtail-reference/).

## Store credentials in AWS Secrets Manager or SSM

Instead of passing credentials in plain text, you can store them in AWS Secrets Manager or AWS Systems Manager (SSM) Parameter Store and reference them by Amazon ARN.
When you set an ARN, Terraform grants the function permission to read the secret, and the function resolves the value at runtime.

Use the corresponding variables:

- Secrets Manager: `username_secret_arn`, `password_secret_arn`, `bearer_token_secret_arn`
- SSM Parameter Store: `username_parameter_arn`, `password_parameter_arn`, `bearer_token_parameter_arn`

## Configure a VPC

To run the function in a VPC, set the `lambda_vpc_subnets` and `lambda_vpc_security_groups` variables.
Every subnet must be able to reach the write address.

## CloudWatch subscription filters

The Terraform configuration creates a subscription filter for each name in `log_group_names` with an empty filter pattern, so it forwards all log events.
It doesn't accept filter patterns for regular expression filtering on log contents.

To filter log contents, extend the Terraform configuration to set a `filter_pattern` for a specific log group, or forward logs to a [Grafana Alloy](/docs/alloy/latest/) collector and filter them with the [`loki.process`](/docs/alloy/latest/reference/components/loki/loki.process/) component.

## Trigger the function through SQS

To trigger the function through an SQS queue instead of directly, set `sqs_enabled=true`.
This creates an SQS queue and a dead-letter queue for on-failure recovery.
For more information, refer to [Recover logs on failure with an SQS dead-letter queue](/docs/loki/latest/send-data/lambda-promtail/#recover-logs-on-failure-with-an-sqs-dead-letter-queue).
