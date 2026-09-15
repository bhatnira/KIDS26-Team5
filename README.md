# Biohackathon Project Template

This repository is a starting point for a three-day team project. This repository is populated with a starting template for team organization and planning. Use it to plan, build, and document work. Please adjust this repository to suit the needs of your team.

> **Team leads:** Start with the [team lead checklist](project-management/CHECKLIST.md) before the event or during your first team meeting.

## Project Profile

- **Project name:** AnTelOpe: An LLM-Powered Web Platform for Nextflow Pipeline Orchestration and Monitoring
- **Question, problem, or opportunity:** Running Nextflow pipelines at scale means researchers depend on engineers to wire together a scheduler, object storage, log streaming, and access control before they can launch a single run. AnTelOpe (the codebase already in [`src/`](src/)) packages that plumbing into one platform with an AI research agent in the loop. The hackathon opportunity is to stress-test it against a real pipeline end to end, polish the researcher-facing experience, and get it demo-ready.
- **Data, inputs, or evidence:** A small, publicly available Nextflow pipeline (for example, an nf-core test/demo pipeline) run against its public test data, so the team has something safe and redistributable to launch live during the demo.
- **Expected output:** A live demo of the golden path — register a pipeline, launch a job on Nomad, watch its logs stream live over SSE, and ask the built-in AI agent about job status — plus a short write-up of what worked and what remains rough (see [Final Output and Handoff](project-management/CHECKLIST.md#final-output-and-handoff)).
- **Tools and stack:** Backend: Go 1.25 + Gin, GORM/PostgreSQL, Redis, HashiCorp Nomad (job dispatch), MinIO/S3 (per-user object storage). Frontend: Vue 3 + Vite + Pinia + Naive UI. AI: OpenAI-compatible LLM API. Pipelines: Nextflow. Local dev: Docker Compose, `just` / `make`. See [`src/AGENTS.md`](src/AGENTS.md) and [`src/DEVELOPMENT.md`](src/DEVELOPMENT.md) for the full architecture and build commands.
- **Team lead:** [HaidYi](https://github.com/haidyi)
- **Team members and roles:** See [`project-management/team.md`](project-management/team.md).
- **Communication:** Slack Channel, [#team5](https://stjudebiohackathon.slack.com/archives/C0BSA3FBJMB) under the [St. Jude BioHackathon](stjudebiohackathon.slack.com).

Naming the tools and stack early helps the team lead create useful roles and divide work realistically. It is fine to revise this section as the project develops.

## Vision and Mission

- **Vision:** A single, approachable front door where a researcher — not just an engineer — can launch, monitor, and reason about Nextflow pipelines, with an AI agent that lowers the barrier further.
- **Mission:** During the hackathon, run AnTelOpe against a real pipeline end to end, fix or document the rough edges the team hits along the way, and leave the project easier for the next person to pick up.

## About

Bioinformatics teams routinely rebuild the same plumbing to run Nextflow at scale: a scheduler, object storage, log streaming, and user accounts. AnTelOpe (this repository's [`src/`](src/)) packages that plumbing into one deployable platform with an AI agent built in, so researchers spend less time waiting on engineers to launch a run.

## Roadmap and Milestones

| When | Focus | Expected outcome |
| --- | --- | --- |
| Day 1 | Everyone gets AnTelOpe running locally (Docker Compose); confirm the demo pipeline/data; claim one of the small first tasks in [project-plan.md](project-management/project-plan.md) | Everyone can run the app and has opened one small first PR |
| Day 2 | Register the demo pipeline and launch a job on Nomad; work the small tasks (translations, UI polish, error messages) in parallel | A pipeline runs end to end with live logs; small-task PRs in review |
| Day 3 | Rehearse the golden-path demo; finish the demo script and known-limitations write-up | A repeatable live demo plus a written handoff in this README |

The goal is not a perfect production system. The goal is a clear, honest, useful result that the team can explain and others can build on.


