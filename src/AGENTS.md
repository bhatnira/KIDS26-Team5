# Agent Instructions for Antelope

This file provides guidance to AI agents when working with code in this repository.

## Project Overview

Antelope is an AI-augmented Nextflow bioinformatics pipeline management platform. It has a Go/Gin backend and a Vue 3 frontend (`web_src/`), with Nextflow jobs dispatched to HashiCorp Nomad.

## Commands

### Backend (Go)

```bash
just build        # Build all binaries (server + cli)
just server       # Build API server → bin/antelope (provides `web` and `monitor` subcommands)
just cli          # Build CLI client → bin/antelope-cli
just dev          # Run API server (web) in debug mode (ANTELOPE_SYSTEM_MODE=debug)
just dev-monitor  # Run standalone Nomad event monitor in debug mode
just test         # go test -race -cover ./...
just lint         # golangci-lint run ./...
just fmt          # gofmt + goimports
just swagger      # Regenerate Swagger docs (swag init -g main.go)
just tidy         # go mod tidy && go mod verify
just skills-bundle # Regenerate the built-in agent skill library (see "Agent Skills")
```

The server is a single binary with subcommands: `bin/antelope web` runs the HTTP API
(plus the Nomad event monitor in-process), and `bin/antelope monitor` runs the monitor
standalone. There is no separate monitor or MCP binary.

Run a single test:
```bash
go test -run TestFunctionName ./path/to/package/...
```

### Frontend (web_src/)

```bash
cd web_src
npm run dev       # Vite dev server (proxies API to backend)
npm run build     # Production build
npm run lint      # ESLint --fix
npm run format    # Prettier
```

## Architecture

### Layered Go Backend

```
main.go + cmd/           → CLI entry (web / monitor subcommands); cmd/cli/ is the CLI client

internal/app/app.go      → Infrastructure client construction (DB, Redis, Nomad, Storage, Mailer, SSE)
routers/routers.go       → Service wiring + route registration
routers/api/v1/          → HTTP request handlers
services/                → Business logic
models/                  → GORM entities + schema migrations (models/migrate.go)
internal/modules/        → Infrastructure wrappers (auth, nomad, sql, nosql, storage, sse, llmconfig, …)
pkg/                     → Shared utilities (apperr, response, types)
```

Dependency injection is explicit: infrastructure clients are built in `internal/app/app.go` and passed down as narrow interfaces. Services are constructed in `routers/routers.go` using the `AppDeps` interface.

### Key Systems

- **Auth**: JWT (access + refresh tokens), LDAP, OIDC — `services/auth/`, `internal/modules/auth/`
- **Job Dispatch**: Nextflow jobs submitted to Nomad as parameterized batch jobs — `services/job/`, `internal/modules/nomad/`
- **Live Logs**: Server-Sent Events stream Nomad job logs to the frontend — `internal/modules/sse/`
- **LLM Chat**: OpenAI-compatible chat via Cloudwego Eino — `services/llm/` (~480 lines)
- **Storage**: Per-user MinIO/S3 client management; configs persisted in Postgres (durable source of truth) with a Redis cache, encrypted at rest — `services/storage/`, `internal/modules/storage/`
- **HA Monitor**: Nomad event monitor with Redis-based leader election — `internal/modules/nomad/`, `services/monitor/`
- **Config**: Viper reads `config.yaml` + env vars prefixed `ANTELOPE_`. See `config.yaml.example`.

### Frontend (web_src/src/)

Vue 3 + Vite + Pinia + Naive UI. Key directories:
- `stores/` — Pinia state (with `pinia-plugin-persistedstate`)
- `api/` — Alova HTTP clients (handles auto token refresh)
- `views/` — Page-level components
- `components/` — Shared components
- `router/` — Vue Router config

Real-time job logs use SSE (`EventSource`). I18n is handled by `vue-i18n`.

### Configuration

All backend config is prefixed `ANTELOPE_`. `config.yaml` holds non-secret
structural defaults (committed as `config.yaml.example`); secrets and
per-deployment scalars come from `ANTELOPE_*` env vars, which override the file.
Key variables:
```
ANTELOPE_SYSTEM_PORT, ANTELOPE_SYSTEM_MODE (debug|release)
ANTELOPE_SYSTEM_SUPER_USER, ANTELOPE_SYSTEM_SUPER_USER_PASSWORD  # single bootstrap admin
ANTELOPE_POSTGRESQL_HOST/PORT/DB/USERNAME/PASSWORD
ANTELOPE_REDIS_HOST/PORT/PASSWORD
ANTELOPE_NOMAD_HOST/PORT/NAMESPACE/REGION/TOKEN
ANTELOPE_JWT_ACCESS_SIGNING_KEY, ANTELOPE_JWT_REFRESH_SIGNING_KEY
ANTELOPE_SYSTEM_ENCRYPT_KEY      # base64 32-byte AES-256 key; encrypts per-user
                                 # storage/LLM secrets at rest. Unset = plaintext
                                 # (still durable) + startup warning.
ANTELOPE_CAB_BASE_URL            # external CAB/nightingale upstream base URL
```
There is no platform-wide LLM config: per-user LLM access lives in the database
(`llmconfig.Manager`), with Postgres as the durable source of truth and Redis as
a cache, set in the UI. Per-user object-storage credentials follow the same
Postgres-durable + Redis-cache pattern (`storage.ClientManager`); both are
encrypted at rest when `ANTELOPE_SYSTEM_ENCRYPT_KEY` is set (`pkg/secretbox`).

On start-up `setting.InitConfig` prints a secret-redacted config summary and
runs `Validate()`: in **release** mode it fails fast when JWT signing keys, the
DB password, the super-user password, or the Nomad host are empty or left at a
shipped default; in **debug** mode the same checks only warn.

### Agent Skills

The agent's built-in skill library (~710 bioinformatics skills, ~23 MiB) is
**generated, not committed**. Four upstream libraries are vendored as git
submodules under `skills/upstream/`; `skills/sources.yaml` declares which parts
of each to keep, and `cmd/skillsbundle` prunes them into
`internal/modules/agent/skillbundle/bundle/` (gitignored).

```bash
just skills-submodules   # git submodule update --init --depth 1 (shallow: 2 repos are >100 MB)
just skills-bundle       # regenerate the bundle; prints per-source counts, drops and sizes
just skills-clean        # remove the generated tree
```

How it reaches the runtime:

- **Release / Docker / goreleaser** builds add the `skills` build tag, which
  `//go:embed`s the bundle into the binary. On start-up it unpacks once to
  `agent.skills.bundle-cache-root`, gated on a content hash so restarts are free.
  Unpacking is mandatory, not an optimisation: the framework stages a skill by
  copying its directory into the Daytona sandbox, so `skill.Repository.Path` has
  to return a real filesystem path.
- **Dev builds** (`make dev`, `make server`, `go test`) skip the tag and read the
  generated tree from `agent.skills.bundle-root`. A missing bundle is not an
  error — the agent runs without built-in skills and logs a warning.

`docker build` needs `make skills-bundle` to have run on the host first;
`.dockerignore` excludes `skills/upstream/`, so the ~380 MB of upstream working
trees never enter the build context. The Dockerfile fails with a pointed message
if the bundle is absent.

Two constraints are easy to break and worth knowing before touching this code:

- **The prompt never lists the library.** The framework injects every visible
  skill's name and description into the system prompt on *every* request — ~107k
  tokens for this library. `skillbundle.RenderOverview` replaces that with domain
  counts (~500 tokens) via `agent.WithAvailableSkillsRenderer`, and the model
  resolves names through the `skill_search` tool. `Repository.Summaries()` is
  served from the pre-built `index.json`, not by re-parsing 710 SKILL.md files
  per turn.
- **Front matter must be one line per key.** The framework's SKILL.md reader is
  line-oriented and flattens nesting, so a nested `name:` at any indentation
  shadows the real one and silently drops skills from its index. The bundler
  normalises every SKILL.md accordingly and then re-reads the finished bundle
  through the framework's own repository, failing the build if any catalogued
  skill no longer resolves.

The skills have to *run* somewhere: `scripts/daytona/` builds the sandbox image
whose package set is derived from a static analysis of the same 710 skills, so a
skill does not fail with "package not found". Regenerating the bundle with new
upstream skills can introduce new dependencies — re-run that analysis and update
`scripts/daytona/environment.yml` when it does.

`skills/upstream/openclaw-medical` is deliberately **not** bundled: the repo
ships no licence and 1151 of its files are marked "All Rights Reserved /
proprietary", so baking it into a distributed image would be redistribution
without a grant. The submodule stays as a pointer; opt in at your own risk with
`make skills-bundle INCLUDE_UNLICENSED=1`.

### Swagger Docs

API docs are generated by `swag` from `main.go` annotations and served at `/api/docs/`. Run `just swagger` after changing handler annotations.

### Helm / Kubernetes

Deployment charts are in `helm/antelope/`. The chart renders a **single**
Deployment: the Nomad event monitor runs in-process inside the `web` pods
(`cmd/web.go`), and its Redis leader election keeps exactly one instance active,
so `replicaCount` is safe to scale. Running `bin/antelope monitor` as its own
workload is supported by the binary but the chart ships no such Deployment.

Health endpoints differ per subcommand: `web` serves `/api/v1/healthz`
(dependency-free, used for liveness) and `/api/v1/ready`; the standalone
`monitor` serves `/api/v1/health` and `/api/v1/ready`. `/ready` checks
PostgreSQL, Redis **and** Nomad, so pods stay NotReady until all three answer.

`InitPostgres`/`InitRedis`/`InitNomad` panic when a dependency is unreachable,
so the container exits rather than degrading. The chart therefore gates pod
start on an init container that waits for PostgreSQL and Redis to accept
connections (`waitForDependencies`); without it a fresh `helm install`
CrashLoopBackOffs until the subchart pods are up.

Agent skills need no volume or ConfigMap — they ship inside the image (see "Agent Skills").
