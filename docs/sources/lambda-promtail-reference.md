---
title: Lambda Promtail reference
menuTitle: Reference
description: Reference for Lambda Promtail environment variables, propagated labels, relabeling, and limitations.
weight: 300
---

# Lambda Promtail reference

This page describes the environment variables, propagated labels, relabeling, and limitations for Lambda Promtail.
The Terraform and CloudFormation deployments set most of these values for you. For deployment steps, refer to [Deploy with Terraform](lambda-promtail-deploy-terraform.md) and [Deploy with CloudFormation](lambda-promtail-deploy-cloudformation.md).

## Environment variables

Lambda Promtail reads its configuration from the following environment variables.

| Variable | Default | Description |
| --- | --- | --- |
| `WRITE_ADDRESS` | none, required | The Loki write API compatible endpoint to write logs to, in the form `https://<hostname>/loki/api/v1/push`. |
| `USERNAME` | empty | The basic authentication username. If set, you must also set `PASSWORD`. Accepts a value or the ARN of an AWS Secrets Manager secret or SSM parameter. |
| `PASSWORD` | empty | The basic authentication password. If set, you must also set `USERNAME`. Accepts a value or an ARN. |
| `BEARER_TOKEN` | empty | A bearer token for the `Authorization` header. You can't set it together with `USERNAME`. Accepts a value or an ARN. |
| `TENANT_ID` | empty | The tenant ID, sent as the `X-Scope-OrgID` header. |
| `KEEP_STREAM` | `false` | Set to `true` to keep the CloudWatch log stream value as the `__aws_cloudwatch_log_stream` label. |
| `BATCH_SIZE` | `131072` | The batch size in bytes at which the function flushes logs. The default is 128 KB. |
| `EXTRA_LABELS` | empty | A comma-separated list of `name,value` pairs to add to every entry. By default, each label name is prefixed with `__extra_`. |
| `OMIT_EXTRA_LABELS_PREFIX` | `false` | Set to `true` to omit the `__extra_` prefix from the labels defined in `EXTRA_LABELS`. |
| `DROP_LABELS` | empty | A comma-separated list of label names to drop from every entry. |
| `RELABEL_CONFIGS` | empty | A JSON array of relabel rules in Prometheus `relabel_configs` format. Refer to [Relabeling configuration](#relabeling-configuration). |
| `LOKI_STAGE_CONFIGS` | empty | A JSON array of Loki pipeline stages to apply to each entry. Refer to [Pipeline stages](#pipeline-stages). |
| `PIPELINE_TIMEOUT` | `1s` | The timeout for processing a single log line through the pipeline stages, as a Go duration string. |
| `SKIP_TLS_VERIFY` | `false` | Set to `true` to skip TLS certificate verification. Use for development only. |
| `PRINT_LOG_LINE` | `true` | Set to `false` to stop the function from printing each parsed log line before forwarding it. |
| `LOG_LEVEL` | `info` | The log level for the function's own logs. |

{{< admonition type="note" >}}
The Terraform and CloudFormation templates don't set `LOKI_STAGE_CONFIGS`, `PIPELINE_TIMEOUT`, or `LOG_LEVEL`.
To use these, add them to the function's environment configuration.
{{< /admonition >}}

## Propagated labels

Incoming logs are assigned special labels that you can use in relabeling or in later [pipeline stages](#pipeline-stages):

| Label | Description |
| --- | --- |
| `__aws_log_type` | The source of the log: `cloudwatch`, `kinesis`, or one of the S3-based log types, such as `s3_lb` or `s3_vpc_flow`. |
| `__aws_cloudwatch_log_group` | The CloudWatch log group for this log. |
| `__aws_cloudwatch_log_stream` | The CloudWatch log stream for this log. Present only when `KEEP_STREAM` is `true`. |
| `__aws_cloudwatch_owner` | The AWS ID of the owner of the event. |
| `__aws_kinesis_event_source_arn` | The Kinesis event source ARN. |
| `__aws_<log_type>` | For S3-based logs, the source identifier extracted from the object key, for example the load balancer name for `s3_lb`. |
| `__aws_<log_type>_owner` | For S3-based logs, the account ID of the log owner. |

For S3-based logs, `<log_type>` is one of the following values, which is also used as the value of `__aws_log_type`:

`s3_vpc_flow`, `s3_lb`, `s3_cloudtrail`, `s3_cloudfront`, `s3_waf`, `s3_guardduty`, `s3_msk`, `s3_access`.

For example, an Application Load Balancer log receives the labels `__aws_log_type="s3_lb"`, `__aws_s3_lb` for the load balancer name, and `__aws_s3_lb_owner` for the account ID.

## Relabeling configuration

Lambda Promtail supports Prometheus-style relabeling through the `RELABEL_CONFIGS` environment variable.
Use relabeling to modify, keep, or drop labels before the function sends logs to Loki.
Provide the configuration as a JSON array of relabel rules.
Relabeling follows the same principles as Prometheus relabeling. For a detailed explanation, refer to [How relabeling in Prometheus works](https://grafana.com/blog/2022/03/21/how-relabeling-in-prometheus-works/).

### Example configurations

Rename a label and capture regular expression groups:

```json
[
  {
    "source_labels": ["__aws_log_type"],
    "target_label": "log_type",
    "action": "replace",
    "regex": "(.*)",
    "replacement": "${1}"
  }
]
```

Keep only specific log types:

```json
[
  {
    "source_labels": ["__aws_log_type"],
    "regex": "s3_.*",
    "action": "keep"
  }
]
```

Drop internal AWS labels:

```json
[
  {
    "regex": "__aws_.*",
    "action": "labeldrop"
  }
]
```

Combine multiple rules:

```json
[
  {
    "source_labels": ["__aws_log_type"],
    "target_label": "log_type",
    "action": "replace",
    "regex": "(.*)",
    "replacement": "${1}"
  },
  {
    "source_labels": ["__aws_s3_lb"],
    "target_label": "loadbalancer",
    "action": "replace"
  },
  {
    "regex": "__aws_.*",
    "action": "labeldrop"
  }
]
```

### Supported actions

Relabeling supports the same actions as Prometheus:

- `replace`: Replace a label value with a new value using regular expression capture groups.
- `keep`: Keep entries where labels match the regular expression.
- `drop`: Drop entries where labels match the regular expression.
- `hashmod`: Set a label to the modulus of a hash of labels, which is useful for sharding.
- `labelmap`: Copy labels to other labels based on regular expression matching.
- `labeldrop`: Remove labels that match the regular expression.
- `labelkeep`: Keep only labels that match the regular expression.
- `lowercase`: Convert label values to lowercase.
- `uppercase`: Convert label values to uppercase.

### Configuration fields

Each relabel rule supports the following fields. All fields are optional except `action`.

- `source_labels`: A list of label names to use as input for the action.
- `separator`: A string that joins source label values. The default is `;`.
- `target_label`: The label to modify. It's required for the `replace` and `hashmod` actions.
- `regex`: A regular expression to match against. The default is `(.+)` for most actions.
- `replacement`: The replacement pattern for the matched regular expression. It supports capture groups such as `${1}` and `${2}`.
- `modulus`: The modulus for the `hashmod` action.
- `action`: One of the supported actions.

### Relabel order and behavior

- Relabeling runs after the function merges the labels from `EXTRA_LABELS` and drops the labels specified by `DROP_LABELS`.
- If relabeling removes all labels from an entry, the function drops the entry.
- Rules are processed in order, and each rule can affect the input of later rules.
- Regular expressions in the `regex` field support full RE2 syntax.
- For the `replace` action, if the `regex` doesn't match, the target label remains unchanged.

## Pipeline stages

Set the `LOKI_STAGE_CONFIGS` environment variable to a JSON array of [Loki pipeline stages](https://grafana.com/docs/loki/<LOKI_VERSION>/send-data/promtail/pipelines/) to transform entries before the function forwards them.
Each entry is processed synchronously.
If a stage doesn't finish within `PIPELINE_TIMEOUT`, the function drops the entry.

## Example Grafana Alloy configuration

Instead of writing directly to Loki, you can forward logs from Lambda Promtail to a [Grafana Alloy](https://grafana.com/docs/alloy/latest/) collector, which then writes to Loki.

{{< admonition type="note" >}}
Promtail is deprecated and at end of life.
Use [Grafana Alloy](https://grafana.com/docs/alloy/latest/) as the collector between Lambda Promtail and Loki.
Alloy is compatible with the Loki push API through its [`loki.source.api`](https://grafana.com/docs/alloy/latest/reference/components/loki/loki.source.api/) component.
{{< /admonition >}}

The following Alloy configuration receives logs on the Loki push API endpoint, maps the special `__aws_*` labels to Loki labels, and forwards the entries to Loki.
Set the Lambda Promtail `WRITE_ADDRESS` to the Alloy endpoint, for example `http://<alloy-host>:3500/loki/api/v1/push`.

```alloy
// Receive logs from Lambda Promtail on the Loki push API endpoint.
loki.source.api "lambda_promtail" {
  http {
    listen_address = "0.0.0.0"
    listen_port    = 3500
  }

  forward_to = [loki.write.default.receiver]

  // Add a static label to indicate that the Lambda Promtail workflow processed these logs.
  labels = {
    source = "lambda-promtail",
  }

  relabel_rules = loki.relabel.lambda_promtail.rules
}

// Map the special __aws_* labels to labels for use in Loki.
loki.relabel "lambda_promtail" {
  forward_to = []

  rule {
    source_labels = ["__aws_log_type"]
    target_label  = "log_type"
  }

  // Map the CloudWatch log group into a label called log_group.
  rule {
    source_labels = ["__aws_cloudwatch_log_group"]
    target_label  = "log_group"
  }

  // Map the load balancer name into a label called loadbalancer_name.
  rule {
    source_labels = ["__aws_s3_lb"]
    target_label  = "loadbalancer_name"
  }
}

// Forward received logs to Loki.
loki.write "default" {
  endpoint {
    url = "http://ip_or_hostname_where_Loki_runs:3100/loki/api/v1/push"
  }
}
```

## Limitations

The following limitations describe the constraints and trade-offs of running Lambda Promtail, such as how retries and dropped logs are handled, event size caps, and behavior when you forward through a collector.
Review them before you deploy to understand where you might lose logs or need to adjust your architecture, and to set expectations for reliability and scale.

### Retries and dropped logs

Lambda Promtail applies retries at several layers:

- **Sending a batch to the write endpoint**: When a batch fails with an HTTP 429, an HTTP 5xx, or a connection-level error, Lambda Promtail retries the send. The retry count is hard-coded to 10 attempts, waiting an exponentially increasing delay between attempts, from 100 milliseconds up to 30 seconds. If every attempt fails, the function drops the batch. Errors other than 429, 5xx, and connection-level errors aren't retried.
- **Lambda invocation**: AWS retries the function invocation itself on failure. The provided Terraform sets a maximum of 2 invocation retries with `maximum_retry_attempts`.
- **SQS redrive**: If you trigger the function through SQS, a message that fails to process returns to the queue and moves to the dead-letter queue after it reaches the maximum receive count. The provided Terraform sets this count to 5.

### CloudWatch event size

Amazon CloudWatch [quotas](https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/cloudwatch_limits_cwl.html) limit the event size to 256 KB. This quota can't be changed.

### Batch behavior when writing to a collector

This limitation is relevant only when Lambda Promtail writes to a collector, such as Grafana Alloy, instead of directly to Loki.
Because the collector batches writes to Loki for performance, it can receive a log, return a successful `204` status code, and then be terminated before it writes upstream to Loki.
This is rare, but it's a trade-off of forwarding through a collector.

### Availability

For availability, run a set of Grafana Alloy collectors behind a load balancer.

### Template and deployment customization

The provided Terraform and CloudFormation files cover the default use cases.
More complex deployments, such as adding VPC configuration or subscribing to many CloudWatch log groups, require you to modify and extend the files.
The Terraform configuration is more flexible than the CloudFormation templates because it accepts arrays of log group and bucket names and supports VPC configuration.

### Collectors between Lambda Promtail and Loki

{{< admonition type="note" >}}
This section is relevant only if you run a collector, such as Grafana Alloy, between Lambda Promtail and Loki to work around out-of-order errors.
Current versions of Loki removed the ordering constraint, so this is no longer required for most deployments.
{{< /admonition >}}

Forwarding through a collector moves the worst-case stream cardinality from the number of log streams to the number of log groups multiplied by the number of collectors.
When you run a set of collectors behind a load balancer, assign each collector a unique label so that logs for the same log group don't cause out-of-order errors.
In Grafana Alloy, add a unique label with the `external_labels` argument of the [`loki.write`](https://grafana.com/docs/alloy/latest/reference/components/loki/loki.write/) component, for example `external_labels = { collector = constants.hostname }`.
Run a small number of collectors behind a load balancer according to your throughput and redundancy needs.

If you haven't configured Loki to [accept out-of-order writes](https://grafana.com/docs/loki/<LOKI_VERSION>/configure/#accept-out-of-order-writes), the unique label is required.
