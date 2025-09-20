output "lambda_function_arn" {
  description = "The ARN of the lambda-promtail function."
  value       = aws_lambda_function.this.arn
}
