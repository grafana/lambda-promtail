# devops-lambda-promtail — Ferryhopper

Ferryhopper fork of [grafana/lambda-promtail](https://github.com/grafana/lambda-promtail).

Reads log files from S3 and pushes them to Grafana Cloud Loki. Extended with ALB-specific features for the Ferryhopper observability pipeline.

## Pipeline

```
ALB → S3 (aws-lb-access-logs/) → Lambda (this repo) → Grafana Cloud Loki
```

The Lambda is triggered by S3 `ObjectCreated` events. For each new log file it reads every line, transforms it, and pushes to Loki via the push API.

## Ferryhopper additions

### 1. Log filtering before ingestion

Unwanted log lines (e.g. 200 responses) are dropped inside the Lambda before the HTTP push to Loki, reducing ingestion volume.

Controlled via the `loki_stage_configs` Terraform variable, which maps to the `LOKI_STAGE_CONFIGS` env var. Accepts a JSON array of [Loki pipeline stages](https://grafana.com/docs/loki/latest/send-data/promtail/stages/).

Example — drop all 200 responses:
```hcl
loki_stage_configs = "[{\"drop\":{\"expression\":\"\\\"apache_status\\\":\\\"200\\\"\"}}]"
```

### 2. Dynamic labels from ALB log fields

Each log line gets `service` and `env` labels extracted at ingestion time, enabling per-service filtering in Grafana without post-ingest parsing.

Extraction logic (in order of priority):
1. **Target group ARN** (field 18) — regex `targetgroup/...-<service>-<env>/` → e.g. `service=crs env=uat`
2. **Hostname** (field 20, fallback) — regex `<service>.<env>.ferryhopper.com` → e.g. `service=ferryhapi env=uat`

In Grafana Explore you can then query:
```
{service="crs", env="uat"}
```

### 3. JSON output format

Raw ALB log lines are rewritten as JSON objects before ingestion, using field names compatible with the existing `jsonext_ups` Nginx log format. This enables shared Grafana queries across ALB and Nginx logs.

Field mapping:

| ALB field | JSON key |
|---|---|
| client IP | `apache_rem_addr` |
| request method | `apache_req_method` |
| request URI | `apache_req_uri` |
| elb status code | `apache_status` |
| sent bytes | `apache_resp_blen` |
| user agent | `apache_user_agent` |
| target processing time | `apachex_URT` |
| domain name | `apachex_host` |
| target group ARN | `alb_target_group` |
| ssl protocol | `alb_ssl_protocol` |
| elb name | `alb_name` |
| timestamp | `timestamp` |

Query example:
```
{__aws_log_type="s3_lb"} | json | apache_status =~ "4..|5.."
```

## Terraform variables

| Variable | Default | Description |
|---|---|---|
| `extra_labels` | `"env,staging,service,alb"` | Comma-separated key/value pairs attached as Loki labels |
| `batch_size` | — | Max batch size before flushing to Loki |
| `loki_stage_configs` | `""` | JSON array of Loki pipeline stages for filtering/transformation |
| `password` | sensitive | Grafana Cloud Loki basic auth token |

Set `OMIT_EXTRA_LABELS_PREFIX=true` (hardcoded in `mon-alb-logs/main.tf`) so `extra_labels` produces bare `env=staging` instead of `__extra_env=staging`.

## Release

Releases are managed with [release-please](https://github.com/googleapis/release-please). Merge a release PR on `main` to create a tag. The tag triggers the build workflow which:

1. Builds the Go binary (`GOOS=linux CGO_ENABLED=0 go build -o ./bootstrap ./...`)
2. Zips it as `lambda-promtail-<version>.zip`
3. Uploads to `s3://fh-lambda-artifacts-staging/`
4. Creates a GitHub Release with the zip attached

To trigger a release manually, push a `v*` tag:
```bash
make release          # bumps patch: v0.1.0 → v0.1.1
make release BUMP=minor
git push origin <tag>
```

## Local development

Requires: Docker, Go, AWS CLI.

**Start local Loki + Grafana:**
```bash
docker compose -f dev/docker-compose.yml up -d
```

- Loki: http://localhost:3100
- Grafana: http://localhost:3005

**Push a real ALB log file to local Loki:**
```bash
go run dev/push-alb-logs.go /path/to/albaccesslog.log.gz
```

Download a log file from S3 first:
```bash
aws s3 cp s3://aws-lb-access-logs/fh-main-staging/<filename>.log.gz /tmp/alb-test.log.gz
```

**Run unit tests:**
```bash
go test ./pkg/ -v -run "Test_parseALBDynamicLabels|Test_albLogLineToJSON"
```

**Stop the stack:**
```bash
docker compose -f dev/docker-compose.yml down
```

## Requirements

- Go 1.21+
- Terraform
- AWS CLI with access to `fh-lambda-artifacts-staging` S3 bucket
- GitHub secrets: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` (for release workflow)
