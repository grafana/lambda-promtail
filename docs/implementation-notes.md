# lambda-promtail Ferryhopper Implementation Notes

## Architecture

### Pipeline

```
ALB → S3 (aws-lb-access-logs/fh-main-staging/)
  → S3 ObjectCreated event
  → Lambda (this fork)
  → Grafana Cloud Loki (logs-prod-012.grafana.net)
```

The Lambda is triggered per file. For each `.log.gz` file it:
1. Downloads and decompresses it
2. Scans line by line
3. For each line: extracts timestamp, applies labels, transforms the line
4. Batches entries and pushes to Loki via HTTP

---

## Features implemented

### 1. Log filtering before ingestion (`LOKI_STAGE_CONFIGS`)

**Task:** feat/filter logs during ingestion (eg do not ingest 200 codes) — expose filtering variable as a var for the lambda

**What was already in the code:** `pkg/main.go` already reads `os.Getenv("LOKI_STAGE_CONFIGS")` and passes it to `ParsePipelineConfigs`. The drop stage runs inside the Lambda — lines matching the expression are dropped before the HTTP push to Loki.

**What was added:** The variable was not exposed in Terraform. Now added to `variables.tf` (line 153) and `main.tf` (line 252).

**`variables.tf`:**
```hcl
variable "loki_stage_configs" {
  type        = string
  description = "JSON array of Loki pipeline stage configs. Use to filter or transform log lines before ingestion."
  default     = ""
}
```

**`main.tf`:**
```hcl
LOKI_STAGE_CONFIGS = var.loki_stage_configs
```

**How to use** (in `mon-alb-logs/main.tf` once deployed):
```hcl
loki_stage_configs = "[{\"drop\":{\"expression\":\"\\\"apache_status\\\":\\\"200\\\"\"}}]"
```

---

### 2. Dynamic labels (`service`, `env`)

**Task:** feat/dynamic labels service env — extract from hostname / target group ARN

**The problem:** All ALB logs were landing under `service=alb` — no way to filter by individual service in Grafana.

**What was added:** `parseALBDynamicLabels()` in `pkg/s3.go`. For each line it extracts `service` and `env` from:

1. **Target group ARN** (field 18 in naive whitespace split) — regex `targetgroup/(?:[^/]+-)?([a-z0-9]+)-([a-z0-9]+)/`
   - Example: `arn:...:targetgroup/ecs-fh-pro-crs-uat/...` → `service=crs, env=uat`
2. **Hostname fallback** (field 20) — regex `^([a-z0-9-]+)\.([a-z0-9]+)\.ferryhopper\.com`
   - Example: `crs-internal.uat.ferryhopper.com` → `service=crs, env=uat`

Wired into the scanner loop in `parseS3Log`:
```go
lineLS := ls
if labels["type"] == LbLogType {
    dynLabels := parseALBDynamicLabels(logLine)
    lineLS = ls.Merge(dynLabels)
    ...
}
```

`lineLS` is a per-line copy — `ls` (the shared base label set) is never mutated, so labels don't bleed across lines.

**In Grafana after deploy:**
```
{service="crs", env="uat"}
{service="worker-standard", env="uat"}
```

---

### 3. JSON output format (`jsonext_ups` compatible)

**Task:** feat/output format — write as json logs compatible with apache combined (File Web Log Formats)

**The problem:** Raw ALB log lines are space-separated strings. The existing `jsonext_ups` Nginx format uses `apache_`-prefixed JSON field names. To share Grafana dashboards and queries between ALB and Nginx logs, ALB lines need to be rewritten as JSON with the same field names.

**What was added:** `albLogLineToJSON()` in `pkg/s3.go`. Splits the raw line, maps fields to a struct with `jsonext_ups`-compatible names, marshals to JSON.

**Key detail about field positions:** AWS documentation numbers fields 0-indexed, but `strings.Fields()` splits naively — the quoted request field `"POST https://... HTTP/1.1"` spans 3 tokens (fields 12, 13, 14), shifting everything after by +2. So `target_group_arn` is at position 18 (not 16 per docs), and `domain_name` at 20 (not 18).

**Field mapping (`albLogEntry` struct):**

| ALB raw field | Position | JSON key |
|---|---|---|
| client:port | 3 | `apache_rem_addr` |
| request method | 12 (token 1 of quoted request) | `apache_req_method` |
| request URI | 13 (token 2) | `apache_req_uri` |
| elb_status_code | 8 | `apache_status` |
| sent_bytes | 11 | `apache_resp_blen` |
| user_agent | 15 | `apache_user_agent` |
| target_processing_time | 6 | `apachex_URT` |
| domain_name | 20 | `apachex_host` |
| target_group_arn | 18 | `alb_target_group` |
| ssl_protocol | 17 | `alb_ssl_protocol` |
| elb name | 2 | `alb_name` |
| timestamp | 1 | `timestamp` |

**In Grafana after deploy:**
```
{__aws_log_type="s3_lb"} | json | apache_status =~ "4..|5.."
{__aws_log_type="s3_lb"} | json | apachex_host =~ ".*crs.*"
```

---

### 4. Release pipeline

**Task:** Release management config + Build management (build on release and publish to gh release artifact)

Two workflows work in sequence:

1. **`.github/workflows/release-please.yml`** — watches `main`, creates/updates a Release PR with bumped version and changelog. Merging the PR creates the git tag.

2. **`.github/workflows/release.yml`** — triggers on `v*` tag. Builds `bootstrap` binary (`GOOS=linux CGO_ENABLED=0 go build -o ./bootstrap ./...`), zips it, uploads to `s3://fh-lambda-artifacts-staging/lambda-promtail-<version>.zip`, creates GitHub Release with zip attached.

---

### 5. Local dev environment

**Task:** Create local dev environment — get logs from a local file, post to a local stage grafana

- `dev/docker-compose.yml` — Loki on port 3100, Grafana on port 3005
- `dev/push-alb-logs.go` — reads a real `.log.gz`, applies JSON transformation and status filtering, pushes to local Loki

**Usage:**
```bash
# Start stack
docker compose -f dev/docker-compose.yml up -d

# Download a real log file from S3
aws s3 cp s3://aws-lb-access-logs/fh-main-staging/<filename>.log.gz /tmp/alb-test.log.gz

# Push to local Loki
go run dev/push-alb-logs.go /tmp/alb-test.log.gz

# Open Grafana
# http://localhost:3005
# Query: {__aws_log_type="s3_lb"} | json

# Stop stack
docker compose -f dev/docker-compose.yml down
```

---

## What needs to change to deploy on staging

### Step 1 — Merge and tag the fork

Branch `customize-grafana-lp` → merge to `main` on `ferryhopper/devops-lambda-promtail` → merge the release-please PR → tag `v0.1.0` is created → `release.yml` uploads `lambda-promtail-v0.1.0.zip` to `s3://fh-lambda-artifacts-staging/`.

**Prerequisite:** GitHub secrets `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` must be set on `ferryhopper/devops-lambda-promtail`.

### Step 2 — Update `auto-terraform/mon-alb-logs/main.tf`

Three changes required:

**a) Replace Grafana binary source with fork artifact**

Remove the `data "http" "latest_release"` block and `aws_s3_object_copy` resource. Replace with:
```hcl
locals {
  lp_version = "v0.1.0"
  lp_s3_key  = "lambda-promtail-${local.lp_version}.zip"
}
```
The zip is already in `fh-lambda-artifacts-staging` from the release workflow — no copy needed.

**b) Fix the handler name**

Current (line 84):
```hcl
handler = "main"
```
Change to:
```hcl
handler = "bootstrap"
```
The build produces a binary named `bootstrap` (Lambda custom runtime convention).

**c) Add `LOKI_STAGE_CONFIGS` to drop 200s**

Add to the Lambda `environment.variables` block:
```hcl
LOKI_STAGE_CONFIGS = "[{\"drop\":{\"expression\":\"\\\"apache_status\\\":\\\"200\\\"\"}}]"
```
