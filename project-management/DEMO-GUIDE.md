# AnTelOpe Demo Guide

## Team Roles and Responsibilities

| Person | Role | Main Responsibility | Backup/Support Needed |
|--------|------|---------------------|----------------------|
| Nirajan Bhattarai | Environment & Demo Pipeline | Docker Compose setup, Nomad configuration, demo pipeline registration (Tasks 1 & 3) | Docker/Nomad familiarity |
| [Team Member 2] | Content & Translations | UI text alignment in en_US.json / zh_CN.json (Task 2) | None — JSON editing only |
| [Team Member 3] | Frontend/UI Polish | Vue/CSS improvements to dashboard screens (Task 4) | Pairing on API shape questions |
| [Team Member 4] | Backend Error Messages | Clarify user-facing API error strings (Task 5) | Short Go walkthrough from team lead |
| [Team Member 5] | Docs & Demo Script | Day 3 demo script and known-limitations doc (Task 6) | None |

## Pull Requests Summary

| PR | Branch | Title | Status |
|----|--------|-------|--------|
| #1 | `fix/quickstart-nomad-prereq` | Quickstart fixes + blank-page fix + local pipeline execution | ✅ Ready |
| #2 | `task2-locale-alignment` | 11 route keys, common.close, copyText.unpermittedError | ✅ Ready |
| #3 | `task3-demo-pipeline` | nf-core/testpipeline registration + demo docs | ✅ Ready |
| #4 | `task4-workbench` | Workbench real stats, i18n, tooltips, empty states | ✅ Ready |
| #5 | `task5-apperr` | Clarify pipeline schema fetch error for researchers | ✅ Ready |
| #6 | `task6-day3-demo` | Day 3 demo script and known limitations | ✅ Ready |

## Live Demo Script (5 minutes)

### Setup (Before Presentation)

1. **Start the stack:**
   ```bash
   cd src/docker
   docker compose up -d
   ```

2. **Start Nomad dev agent:**
   ```bash
   nomad agent -dev -bind 0.0.0.0
   ```

3. **Verify services:**
   ```bash
   curl http://127.0.0.1:8086/api/v1/ready
   # Should return: {"status":"ready"}
   ```

4. **Open browser:** http://127.0.0.1:8086

5. **Login:** `admin@antelope.dev` / `password`

### Demo Flow

#### 1. Dashboard Overview (30 seconds)
- Show the workbench dashboard with stat cards
- Point out: registered pipelines, job totals, recent activity
- Mention the left sidebar: Pipelines, Jobs, AI Chat

**Talking point:** "This is our AI-augmented Nextflow pipeline management platform. It handles everything from pipeline registration to live execution monitoring."

#### 2. Pipeline Registration (60 seconds)
- Navigate to **Pipelines** in the sidebar
- Show the registered `nf-core/testpipeline` with status **ready**
- Click into the pipeline to show its schema (parsed from `nextflow_schema.json`)

**Talking point:** "We've registered a public nf-core pipeline. The system automatically fetches and parses the pipeline schema, exposing all parameters in the UI."

#### 3. Launch a Job (60 seconds)
- From the pipeline detail, click **Launch Job**
- Enter parameters:
  - `input`: `/tmp/test_samplesheet.csv`
  - `outdir`: `/tmp/results`
  - `igenomes_base`: `/tmp/igenomes`
- Click **Submit**
- Show the job appearing in the Jobs list with status **running**

**Talking point:** "With one click, we dispatch the pipeline to Nomad. The job runs asynchronously — we can track it in real-time."

#### 4. Live Logs via SSE (60 seconds)
- Navigate to **Jobs** → click the running job
- Show the live log stream (Server-Sent Events)
- Point out the Nextflow banner, process submissions, Docker container pulls

**Talking point:** "Logs stream live from Nomad via Server-Sent Events — no polling, no refresh. This is the 'it's actually computing' moment."

#### 5. AI Chat (60 seconds)
- Navigate to **AI Chat**
- Ask: "Is the testpipeline job still running? What's its status?"
- Show the AI response (powered by NVIDIA LLM)

**Talking point:** "Our AI agent can answer questions about your jobs, pipelines, and system status — powered by NVIDIA's language model."

#### 6. Wrap-up (30 seconds)
- Show the health check endpoint:
  ```bash
  curl http://127.0.0.1:8086/api/v1/healthz
  ```
- Mention all 6 PRs are ready for review
- Thank the team

**Talking point:** "This is a complete, working prototype — from pipeline registration to live execution to AI-powered monitoring. All code is in our PRs, ready for review."

## Proof of Results

### What We Built

1. **End-to-End Pipeline Execution**
   - Register any public Nextflow pipeline via GitHub URL
   - Launch jobs with custom parameters
   - Monitor execution with live SSE logs
   - AI-powered job status queries

2. **Full-Stack Application**
   - **Backend:** Go/Gin API with PostgreSQL, Redis, Nomad integration
   - **Frontend:** Vue 3 + Vite + Naive UI + Pinia
   - **Infrastructure:** Docker Compose for local development

3. **Six Pull Requests Delivering:**
   - Quickstart fixes (Docker, Nomad, Java, pipeline execution)
   - UI translations (English/Chinese)
   - Demo pipeline registration
   - Dashboard improvements
   - Error message clarification
   - Documentation and demo scripts

### Technical Achievements

| Metric | Value |
|--------|-------|
| Pipeline Registration | Automatic schema parsing from GitHub |
| Job Dispatch | Async via Nomad parameterized jobs |
| Live Logs | Real-time SSE streaming |
| AI Integration | NVIDIA LLM for job status queries |
| Container Support | Docker (local), Singularity (cluster) |
| Storage | MinIO/S3 compatible (per-user) |

### Known Limitations (Be Honest)

1. **Nomad must be reachable** — local dev agent or VPN for shared cluster
2. **Frontend build files gitignored** — `docker compose up --build` restores them
3. **Default language needs `.env`** — `VITE_DEFAULT_LANG=enUS` required
4. **AI answers depend on LLM config** — each user must configure their provider
5. **Storage is per-user** — pipelines need S3/MinIO credentials configured
6. **Schema fetch requires public repos** — private repos fail gracefully
7. **Single bootstrap admin** — multi-user flows exist but weren't exercised

## Troubleshooting

### App shows raw locale keys
```bash
# Ensure .env exists in src/web_src/
echo "VITE_DEFAULT_LANG=enUS" > src/web_src/.env
# Rebuild: docker compose up -d --build antelope
```

### Nomad job stays pending
```bash
# Check datacenter matches
curl http://127.0.0.1:4646/v1/job/<job-id> | python3 -c "import sys,json; print(json.load(sys.stdin).get('Datacenters'))"
# Should show: ['dc1'] for local dev agent
```

### Pipeline fails with "singularity: command not found"
```bash
# Ensure dispatch profile is 'docker' in src/services/job/service.go
# Rebuild: docker compose build antelope && docker compose up -d --force-recreate antelope
```

### Pipeline fails with memory error
```bash
# Check resourceLimits in src/internal/modules/runner/templates.go
# Should be ≤ your system RAM (e.g., 14GB for 16GB Mac)
```

## Presentation Tips

1. **Start with the problem:** "Bioinformatics pipelines are complex. We built a platform that simplifies registration, execution, and monitoring — with AI assistance."

2. **Show, don't tell:** The live demo is more convincing than slides. Focus on the flow.

3. **Acknowledge limitations honestly:** It builds trust. "This is a hackathon prototype — here's what works, here's what needs polish."

4. **Highlight the AI integration:** It's the differentiator. Show the chat responding to job status questions.

5. **Credit the team:** "Each team member delivered a focused PR. This is collaborative work."

6. **End with next steps:** "What we'd do with more time: multi-user auth, pipeline versioning, automated testing."
