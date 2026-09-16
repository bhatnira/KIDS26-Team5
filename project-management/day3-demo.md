# Day 3 Demo Script and Known-Limitations

Live demo of the AnTelOpe golden path: register a pipeline → launch a job on Nomad → watch live logs → ask the AI agent about job status.

## Prerequisites / Setup

1. The demo stack must be running locally:
   ```bash
   cd src/docker
   docker compose up -d --build
   ```
   (Your local Docker Compose environment needs `docker compose` working, plus a reachable Nomad — see [Known limitations](#known-limitations) item 1.)
2. Verify the demo pipeline is registered (`nf-core/testpipeline`, v1.0.0):
   ```bash
   curl -s http://127.0.0.1:8086/api/v1/pipelines | python3 -m json.tool | grep -i testpipeline
   ```
   If it is missing, follow `src/docs/demo-pipeline.md` to register it.
3. Open the web UI at http://127.0.0.1:8086 and sign in:
   - **Email:** `admin@antelope.dev`
   - **Password:** `password`

## Script (about 5 minutes)

### 1. Landing page and dashboard (15 s)
- Note the workbench dashboard: stat cards (registered/ready pipelines, job totals, activity), recent jobs list, pipeline-status badges, announcements.
- The left sidebar exposes Pipelines, Jobs, and AI Chat.

### 2. Pipelines (45 s)
- **Pipelines → list:** show the registered `nf-core/testpipeline`. Confirm status shows **ready** (schema parsed at registration).
- **Pipelines → detail:** open the pipeline; see its schema, parsed from `nextflow_schema.json`, and the params exposed by it.

### 3. Register a second run or a demo pipeline (60 s)
- **Pipelines → Register:** enter a public nf-core test pipeline repository URL and a tag (e.g. `nf-core/testpipeline`, tag `1.0.0`).
- Register it. Backend fetches `nextflow_schema.json` from GitHub and reports whether the pipeline is ready.
- This is the moment to show the clarified schema error if you have time: try a bogus version and show the new message ("could not read nextflow_schema.json ... check that the repository URL and version (tag/branch) are correct and publicly accessible").

### 4. Launch a job (60 s)
- From the pipeline detail, launch a job with the default/test params.
- Watch it become a Nomad job: status flows from queued → running (→ succeeded).
- Refresh to show the job appears in the recent-jobs list on the workbench.

### 5. Live logs over SSE (60 s)
- Open the running job, go to its logs view.
- Logs stream live (Server-Sent Events) straight off Nomad — no polling.
- This is the convincing "it's actually computing" moment; keep it short and stable.

### 6. Ask the AI agent (60 s)
- **AI Chat:** ask something concrete, e.g. "Is the testpipeline job I just launched still running? What's its status?"
- The agent answers against current job status (results depend on per-user LLM configuration — see limitation item 4).

### 7. Wrap-up (30 s)
- Open `src/docs/demo-pipeline.md` and this file to show the team wrote down exactly how to reproduce everything.
- Mention available health checks: `curl http://127.0.0.1:8086/api/v1/healthz` → ok, `/api/v1/ready` → ready.

## Known Limitations

These are honest gaps from the event, worth saying out loud during the demo:

1. **Nomad must be reachable from the host.** The app fails to dispatch jobs if the Nomad server is unreachable. During the hackathon we used a local dev agent (`nomad agent -dev -bind 0.0.0.0` from `src/docker/README.md`) or the shared cluster at `http://10.48.197.189:4646` (St. Jude network / VPN only). Not a product flaw — an environment prerequisite.
2. **Frontend build files are gitignored.** `src/web_src/build/` is in `.gitignore`, so a fresh clone of `web_src` can render a blank page until the missing pieces are restored (the Naive UI resolver, the pinia auto-import, and the `__URL_MAP__` Vite define for the dev API URL). See PR #1/#build-plugin work; a plain `docker compose up -d --build` from our working tree works because those files exist on disk.
3. **Default language needs a `.env`.** `web_src/.env` is gitignored; without `VITE_DEFAULT_LANG` the UI shows raw locale keys (`login.signInTitle` etc.) rather than translated strings. Our local stack sets `VITE_DEFAULT_LANG=enUS`.
4. **Agent answers depend on per-user LLM configuration.** There is no platform-wide LLM key; each user must configure their provider/model in **User settings → Agent workspace** before AI Chat gives useful job-status answers. The prompt alone will not populate an LLM credential.
5. **Object storage is per-user and configured in the UI.** Pipelines that need S3/MinIO require the user to configure storage in **User Settings** first; otherwise storage-backed steps fail with a clear "no storage configured" error.
6. **Schema fetch requires public, reachable repositories.** `nextflow_schema.json` is pulled over the network from GitHub. Private or unreachable repos fail; unlicensed repos should not be used because the task queue and agent may mirror them.
7. **Single bootstrap admin by default.** The out-of-the-box login is the super user from environment config. Multi-user, LDAP, and OIDC flows exist in the codebase but were not exercised during the hackathon.

## Handoff Notes

- Demo data: `nf-core/testpipeline` v1.0.0 (public domain, safe to demo publicly).
- Reproduction: `.env`-less builds work from this working tree; the exact registration steps are in `src/docs/demo-pipeline.md`.
- Main result: AnTelOpe runs end to end locally — register, launch on Nomad, live SSE logs, AI status chat — with the rough edges above documented rather than hidden.