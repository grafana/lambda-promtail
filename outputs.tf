output "function_arn" {
  description = "The ARN of the Lambda function"
  value       = aws_lambda_function.this.arn
}

output "function_name" {
  description = "The name of the Lambda function"
  value       = aws_lambda_function.this.function_name
}

output "function_role_arn" {
  description = "The ARN of the IAM role for the Lambda function"
  value       = aws_iam_role.this.arn
}

output "function_role_name" {
  description = "The name of the IAM role for the Lambda function"
  value       = aws_iam_role.this.name
}

output "log_group_name" {
  description = "The name of the CloudWatch log group for the Lambda function"
  value       = aws_cloudwatch_log_group.this.name
}

output "log_group_arn" {
  description = "The ARN of the CloudWatch log group for the Lambda function"
  value       = aws_cloudwatch_log_group.this.arn
}

output "kinesis_stream_name" {
  description = "The name of the Kinesis stream for the Lambda function"
  value       = aws_kinesis_stream.this.name
}

output "kinesis_stream_arn" {
  description = "The ARN of the Kinesis stream for the Lambda function"
  value       = aws_kinesis_stream.this.arn
}

output "sqs_queue_name" {
  description = "The name of the SQS queue for the Lambda function"
  value       = aws_sqs_queue.main[0].name
}

output "sqs_queue_arn" {
  description = "The ARN of the SQS queue for the Lambda function"
  value       = aws_sqs_queue.main[0].arn
}
