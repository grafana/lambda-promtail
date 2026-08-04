---
title: Deploy with CloudFormation
menuTitle: Deploy with CloudFormation
description: Deploy the Lambda Promtail function to AWS with the provided CloudFormation templates.
weight: 200
---

# Deploy with CloudFormation

Deploy Lambda Promtail with the [CloudFormation](https://aws.amazon.com/cloudformation/) templates in the [lambda-promtail repository](https://github.com/grafana/lambda-promtail).

The repository provides the following templates:

- [`lambda-promtail.yaml`](https://github.com/grafana/lambda-promtail/blob/main/lambda-promtail.yaml): Forwards Amazon CloudWatch logs. It subscribes one CloudWatch log group to the function.
- [`aws-eventbridge-logs.yaml`](https://github.com/grafana/lambda-promtail/blob/main/aws-eventbridge-logs.yaml): Forwards S3-based logs using EventBridge. Use this template to work around the S3 bucket notification limitation described in [Forward S3 logs with EventBridge](/docs/loki/latest/send-data/lambda-promtail/#forward-s3-logs-with-eventbridge).
- [`aws-alb-logs.yaml`](https://github.com/grafana/lambda-promtail/blob/main/aws-alb-logs.yaml): Forwards Application Load Balancer access logs from S3 using EventBridge.

If you define your infrastructure with CloudFormation and forward S3-based logs, use the EventBridge template for easier deployment.

## Before you begin

Make sure that you have the following:

- The [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) configured with permissions to create the required resources.
- A Loki write endpoint, such as a Grafana Cloud Loki URL or a self-managed Loki cluster.
- The function code available to CloudFormation. The templates load the function from an S3 bucket, so download the `lambda-promtail.zip` archive from the [releases page](https://github.com/grafana/lambda-promtail/releases), upload it to a bucket, and provide the `S3BucketName` and `S3KeyName` parameters.

## Deploy the CloudWatch template

Use `aws cloudformation create-stack` with the `lambda-promtail.yaml` template.
Set the CloudWatch log group to subscribe with the `LogGroupToSubscribe` parameter.

Select a highlighted placeholder in the following commands to enter your own value, such as `WriteAddress`, which is your Loki write endpoint.
The value you enter fills in automatically across all the examples on this page.

```bash
aws cloudformation create-stack \
  --stack-name lambda-promtail \
  --template-body file://lambda-promtail.yaml \
  --capabilities CAPABILITY_IAM CAPABILITY_NAMED_IAM \
  --region us-east-2 \
  --parameters \
    ParameterKey=WriteAddress,ParameterValue=https://@@@LOKI_ENDPOINT@@@/loki/api/v1/push \
    ParameterKey=Username,ParameterValue=@@@USERNAME@@@ \
    ParameterKey=Password,ParameterValue=@@@PASSWORD@@@ \
    ParameterKey=S3BucketName,ParameterValue=@@@S3_BUCKET_NAME@@@ \
    ParameterKey=LogGroupToSubscribe,ParameterValue=@@@LOG_GROUP_NAME@@@
```

To subscribe more than one CloudWatch log group, copy the `MainLambdaPromtailSubscriptionFilter` resource in the template and modify it for each log group:

```yaml
MainLambdaPromtailSubscriptionFilter:
  Type: AWS::Logs::SubscriptionFilter
  DependsOn: LambdaPromtailPermissions
  Properties:
    DestinationArn: !GetAtt LambdaPromtailFunction.Arn
    FilterPattern: ""
    LogGroupName: "@@@ADDITIONAL_LOG_GROUP_NAME@@@"
```

## Deploy the EventBridge template for S3 logs

Use the `aws-eventbridge-logs.yaml` template to forward S3-based logs, such as ALB, VPC flow, or CloudFront access logs.
Set the source bucket with the `EventSourceS3Bucket` parameter.

```bash
aws cloudformation create-stack \
  --stack-name lambda-promtail-stack \
  --template-body file://aws-eventbridge-logs.yaml \
  --capabilities CAPABILITY_IAM CAPABILITY_NAMED_IAM \
  --region us-east-2 \
  --parameters \
    ParameterKey=WriteAddress,ParameterValue=https://@@@LOKI_ENDPOINT@@@/loki/api/v1/push \
    ParameterKey=Username,ParameterValue=@@@USERNAME@@@ \
    ParameterKey=Password,ParameterValue=@@@PASSWORD@@@ \
    ParameterKey=BearerToken,ParameterValue=@@@BEARER_TOKEN@@@ \
    ParameterKey=ExtraLabels,ParameterValue="name1,value1,name2,value2" \
    ParameterKey=TenantID,ParameterValue=@@@TENANT_ID@@@ \
    ParameterKey=SkipTlsVerify,ParameterValue="false" \
    ParameterKey=S3BucketName,ParameterValue=@@@S3_BUCKET_NAME@@@ \
    ParameterKey=EventSourceS3Bucket,ParameterValue=@@@LOG_SOURCE_BUCKET@@@
```

## Set optional parameters

The templates support the following optional parameters.
Availability depends on the template, so check the `Parameters` section of the template that you use.

- `KeepStream`: Set to `true` to keep the log stream label.
- `ExtraLabels`: A comma-separated list in the format `name1,value1,name2,value2` to add extra labels.
- `OmitExtraLabelsPrefix`: Set to `true` to omit the `__extra_` prefix from extra labels.
- `TenantID`: A tenant ID to add when writing logs.
- `SkipTlsVerify`: Set to `true` for development only, to skip TLS certificate verification.
- `ReservedConcurrency`: The maximum number of concurrent executions to reserve for the function.

For details about each option, refer to the [Lambda Promtail reference](/docs/loki/latest/send-data/lambda-promtail/lambda-promtail-reference/).

## Update a stack

To modify an existing stack, use the [`update-stack`](https://docs.aws.amazon.com/cli/latest/reference/cloudformation/update-stack.html) command with your modified template and parameters.
