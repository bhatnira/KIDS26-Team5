# Demo: Register a Nextflow Pipeline in AnTelOpe

AnTelOpe manages Nextflow pipelines by **registering** them (storing the git
repo + tag), then **launching** jobs from the browser via a schema-driven form.
This guide walks through registering a small public pipeline, verifying the
result, and understanding what happens behind the scenes.

## Prerequisites

| Requirement | How to satisfy |
|---|---|
| Docker Compose stack running | `cd src/docker && docker compose up -d --build` ([details](../docker/README.md)) |
| Host-side Nomad dev agent | `nomad agent -dev -bind 0.0.0.0` (the stack cannot reach `127.0.0.1`-bound agents) |
| `curl` or browser access | `http://127.0.0.1:8086` (UI) or API at `http://127.0.0.1:8086/api/v1/` |

## Demo pipeline

[nf-core/testpipeline](https://github.com/nf-core/testpipeline) is nf-core's
official minimal tutorial pipeline. It is small, public, has a
`nextflow_schema.json` (for the Launch form), and real version tags.

| Field | Value |
|---|---|
| Repository | `https://github.com/nf-core/testpipeline` |
| Version (tag) | `1.0.0` |
| Nomad job ID | `nf-core-testpipeline-1.0.0` (auto-generated as `<name>-<version>`) |

## Option A — Register via the API (curl)

### 1. Authenticate

```bash
TOKEN=$(curl -s -X POST http://127.0.0.1:8086/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@antelope.dev","password":"password"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['accessToken'])")
```

The default super-user is seeded at first boot from `config.yaml` +
`ANTELOPE_SYSTEM_SUPER_USER_PASSWORD` (defaults to `password` in
`docker-compose.yaml`).

### 2. Register the pipeline

```bash
curl -s -X POST http://127.0.0.1:8086/api/v1/pipeline/add \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "nf-core-testpipeline",
    "version": "1.0.0",
    "repository": "https://github.com/nf-core/testpipeline",
    "author": "Nick Bruggemans",
    "description": "Small public nf-core demo pipeline for the Antelope quick start"
  }'
```

Expected response:

```json
{"code":2000,"data":null,"msg":"ok"}
```

If the tag `1.0.0` doesn't exist at the repo the request is rejected before
any database write (`code 4220`, "pipeline not exist"); if the same
`(name, version)` is already registered you get `code 4220`,
"pipeline is already registered".

### 3. Verify

```bash
# Check pipeline status (should show "ready")
curl -s http://127.0.0.1:8086/api/v1/pipeline/list_all \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool

# Check the Nomad job exists
curl -s http://127.0.0.1:4646/v1/job/nf-core-testpipeline-1.0.0 | python3 -m json.tool

# Fetch the Launch schema (requires tag to exist at the repo)
curl -s "http://127.0.0.1:8086/api/v1/pipeline/schema?repository=https%3A%2F%2Fgithub.com%2Fnf-core%2Ftestpipeline&version=1.0.0" \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool | head -30
```

## Option B — Register via the UI

1. Open <http://127.0.0.1:8086> and log in with `admin@antelope.dev` /
   `password`.
2. Navigate to **Application → Nextflow → Pipeline** (the pipeline list page).
3. Click **New Pipeline** (or the + button) to open the add modal.
4. Fill in:
   | Field | Value |
   |---|---|
   | Name | `nf-core-testpipeline` |
   | Version | `1.0.0` |
   | Repository | `https://github.com/nf-core/testpipeline` |
   | Author | `Nick Bruggemans` |
   | Description | `Small public nf-core demo pipeline for the Antelope quick start` |
5. Click **Confirm**. The pipeline appears in the list with status `ready`
   (the background Nomad job registration completes within a few seconds).

## What happens behind the scenes

```
   UI / API                AnTelOpe backend                 Nomad                  GitHub
      │                        │                              │                     │
      │  POST /pipeline/add    │                              │                     │
      │ ──────────────────────>│                              │                     │
      │                        │ git ls-remote (tag 1.0.0) ─────────────────────────>
      │                        │ <─ refs match ──────────────────────────────────────│
      │                        │                              │                     │
      │                        │ INSERT INTO pipelines        │                     │
      │                        │   status = "pending"         │                     │
      │   200 ok               │                              │                     │
      │ <──────────────────────│                              │                     │
      │                        │                              │                     │
      │                        │ [goroutine]                  │                     │
      │                        │  render NextflowHCL ────────>│                     │
      │                        │  Register(job, payload) ────>│                     │
      │                        │  status ← "ready"            │                     │
```

- The add is **asynchronous**: the HTTP response returns immediately while a
  background goroutine registers the Nomad job. The pipeline starts as
  `"pending"` and transitions to `"ready"` (or `"failed"`).
- The Nomad job ID is auto-generated as `<name>-<version>` (e.g.
  `nf-core-testpipeline-1.0.0`). It is a **parameterized batch job** —
  actual runs require a Slurm submit host (see note below).
- The Launch configuration page fetches `nextflow_schema.json` from
  `raw.githubusercontent.com` for the given tag, caches it in Redis, and
  renders a dynamic form from the JSON Schema `$defs`.

## Launching a job

The Launch page (Pipeline list → click a ready pipeline → **Launch**) lets you
configure parameters and submit a Nomad job. Two caveats apply in the local
development stack:

1. **Slurm is required.** The built-in Nextflow template writes an
   `nf.config` with `executor = 'slurm'` and submits processes via `sbatch`.
   Without a Slurm cluster reachable from the Nomad client node, the head
   process starts but child processes fail immediately.

2. **MinIO/S3 storage.** The Launch form has a "Browse" button for path fields
   backed by per-user S3 storage. Without a configured MinIO or S3 endpoint,
   paths must be entered manually.

For a full end-to-end demo (register → launch → live logs) on Day 3, a Slurm
cluster (real or emulated via a Slurm singlenode Docker image) and a MinIO
instance are required.

## Other public pipelines you can register

| Pipeline | Repo | Notable features |
|---|---|---|
| rnaseq | `nf-core/rnaseq` | Large, real RNA-seq workflow, has `nextflow_schema.json` |
| scrnaseq | `nf-core/scrnaseq` | Single-cell RNA-seq |
| sarek | `nf-core/sarek` | Variant calling, has schema |

All nf-core pipelines are publicly available and licensed under MIT.
