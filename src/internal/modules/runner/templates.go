package runner

// This file contains the three built-in HCL templates. They are exported
// as plain strings so the business layer can:
//   - pass them directly to Register as RegisterSpec.Template, OR
//   - seed them into its own database and let users customize them.
//
// The engine layer itself does not touch any database.
//
// All three templates share the same variable contract:
//
//	local.job_id       (string)   — Nomad job ID
//	local.datacenters  (list)     — e.g. ["cab"]
//	local.cores        (number)   — task cores
//	local.memory       (number)   — task memory in MB
//
// NextflowHCL additionally requires:
//
//	local.nextflow_url (string)   — URL of the Nextflow launcher binary
//
// Dispatch payloads are opaque to the engine layer; the shape each
// template expects is documented in comments above the constant.

// TaskName is the single Nomad task name declared by every built-in template
// (`task "run" { … }`). It is the source of truth shared between the templates
// here and the log-streamer, which must address the allocation's task by this
// exact name (services/job). Keep it in sync with the `task "run"` blocks
// below; the task name is an internal detail and is intentionally not
// configurable.
const TaskName = "run"

// NextflowHCL
//
// One registration serves any nf-core / custom Nextflow workflow. The
// repository and revision come from the dispatch payload, not the template.
//
// Expected dispatch payload JSON:
//
//	{
//	  "repository": "nf-core/rnaseq",         // required
//	  "revision":   "3.14.0",                 // optional but recommended
//	  "params":     { ... pipeline params },  // optional; written to params.json
//	                                          // an "outdir" key here is honored —
//	                                          // see the outdir precedence note below
//	  "storage": {                            // optional; used for S3/MinIO nf.config
//	    "host":       "minio.example.com:9000",
//	    "access_key": "...",
//	    "secret_key": "...",
//	    "use_ssl":    true
//	  }
//	}
//
// The run.sh uses a bash array + exec to invoke nextflow — NO `eval` —
// so repository / revision / storage values cannot cause shell injection.
//
// Execution model: the Nomad `raw_exec` task runs only the Nextflow *head*
// process. run.sh always writes an nf.config selecting `executor = 'slurm'`
// with Singularity containers, so the pipeline's own processes are submitted
// to Slurm via sbatch rather than run on the Nomad client. Consequences for
// whoever operates the Nomad clients:
//
//   - The client node must be a Slurm submit host: sbatch/squeue/scancel on
//     PATH (see the task's PATH env) and the `standard` partition reachable.
//   - `scratch = false` makes each task execute directly in the work
//     directory, so -work-dir MUST resolve to a filesystem shared with the
//     compute nodes. The default (${NOMAD_TASK_DIR}/work) is node-local and
//     only works for a single-node cluster — override it per dispatch with
//     the `work_dir` meta.
//   - `outdir` resolves as: `outdir` meta > params.json > a task-local
//     fallback. The CLI flag is omitted entirely when params.json carries an
//     outdir, because a command-line --param outranks -params-file in
//     Nextflow and would otherwise shadow the caller's value.
//   - local.cores / local.memory now size the head process only; per-process
//     caps come from `resourceLimits` in the config below.
//
// ----------------------------------------------------------------------------
const NextflowHCL = `
job "antelope-job" {
  datacenters = local.datacenters
  type        = "batch"

  parameterized {
    meta_optional = ["profile", "work_dir", "outdir"]
    payload       = "required"
  }

  group "nextflow" {
    restart {
      attempts = 0
      mode     = "fail"
    }
    reschedule {
      attempts  = 0
      unlimited = false
    }

    task "run" {
      driver = "raw_exec"

      artifact {
        source      = local.nextflow_url
        destination = "local/bin/nextflow"
        mode        = "file"
      }

      dispatch_payload { file = "dispatch_payload.json" }

      resources {
        cores  = local.cores
        memory = local.memory
      }

      env {
        NXF_HOME     = "${NOMAD_TASK_DIR}"
        NXF_WORK     = "${NOMAD_TASK_DIR}/work"
        NXF_OPTS     = "-Dcom.amazonaws.sdk.disableCertChecking=true -Xms512m -Xmx2g"
        NXF_ANSI_LOG = "false"
        PATH         = "${NOMAD_TASK_DIR}/bin:/run/current-system/sw/bin:${PATH}"
      }

      template {
        destination = "local/run.sh"
        perms       = "755"
        data        = <<EOH
#!/bin/bash
set -euo pipefail

PAYLOAD="${NOMAD_TASK_DIR}/dispatch_payload.json"
PARAMS="${NOMAD_TASK_DIR}/params.json"
CONF="${NOMAD_TASK_DIR}/nf.config"

chmod +x "${NOMAD_TASK_DIR}/bin/nextflow"
mkdir -p "${NOMAD_TASK_DIR}/work" "${NOMAD_TASK_DIR}/results"

# Validate payload
jq -e . "$PAYLOAD" >/dev/null || { echo "invalid JSON in dispatch payload"; exit 1; }

# Pipeline params -> params.json
jq -r '.params // {}' "$PAYLOAD" > "$PARAMS"

# Repository / revision are required (revision optional but recommended)
REPO=$(jq -r '.repository // empty' "$PAYLOAD")
REV=$(jq  -r '.revision   // empty' "$PAYLOAD")
if [ -z "$REPO" ]; then
  echo "payload.repository is required"; exit 1
fi

# Base Nextflow config — Slurm executor + Singularity. Always written, so a
# run without storage credentials still lands on the cluster instead of
# falling back to Nextflow's default "local" executor.
# Quoted heredoc: nothing in this block is expanded by bash.
cat > "$CONF" <<'NFCONF'
process {
    // use the resourceLimits directive to control resource
    // Note: resourceLimits is a new feature starting from version 24.04.0.
    // To use it, Nextflow should be upgraded to 24.04.0.
    // see: https://github.com/nf-core/tools/issues/2923
    resourceLimits = [
        memory: 60.GB,
        cpus: 12,
        time: 240.h
    ]
    executor       = 'slurm'
    scratch        = false
    cache          = 'lenient'

    maxRetries     = 3
    errorStrategy  = { task.exitStatus in ((130..145) + 104) ? 'retry' : 'finish' }

    queue          = 'standard'
}


singularity {
    envWhitelist = "SINGULARITY_TMPDIR,CUDA_VISIBLE_DEVICES"
    // allow the tmp dir and GPU visible devices visible in the containers
    enabled      = true
    autoMounts   = true
    runOptions   = '-p'
    pullTimeout  = "3 hours"
}

executor {
    queueSize       = 100
    submitRateLimit = "10/1sec"
    jobName         = {
        task.name
            .replace("[", "(")
            .replace("]", ")")
            .replace(" ", "_")
    }
}
NFCONF

# Storage credentials -> appended to nf.config (optional)
HOST=$(jq  -r '.storage.host       // ""'   "$PAYLOAD")
AK=$(jq    -r '.storage.access_key // ""'   "$PAYLOAD")
SK=$(jq    -r '.storage.secret_key // ""'   "$PAYLOAD")
SSL=$(jq   -r '.storage.use_ssl    // true' "$PAYLOAD")

if [ -n "$HOST" ]; then
  if [ "$SSL" = "true" ]; then PROTO=https; else PROTO=http; fi
  cat >> "$CONF" <<NFCONF

aws {
  accessKey = '$AK'
  secretKey = '$SK'
  client {
    endpoint          = '$PROTO://$HOST'
    s3PathStyleAccess = true
  }
}
NFCONF
fi
chmod 400 "$CONF"

# Container engine must match singularity.enabled above — a "docker" profile
# would enable a second engine and Nextflow refuses to start.
PROFILE="${NOMAD_META_profile}"
[ -z "$PROFILE" ] && PROFILE=singularity
WORK="${NOMAD_META_work_dir}"
[ -z "$WORK" ] && WORK="${NOMAD_TASK_DIR}/work"

# outdir precedence: "outdir" meta > params.json > task-local fallback.
# Nextflow ranks a command-line --param ABOVE -params-file, so passing
# --outdir unconditionally would silently shadow whatever the caller put in
# params.json. Only pass it when the meta asks for a specific path, or when
# params.json supplies no outdir at all (nf-core pipelines require one).
OUT="${NOMAD_META_outdir}"
if [ -z "$OUT" ] && [ "$(jq -r 'if type == "object" then has("outdir") else false end' "$PARAMS")" != "true" ]; then
  OUT="${NOMAD_TASK_DIR}/results"
fi

# Build command using positional parameters — safe against injection in REPO/REV/etc.
set -- nextflow run "$REPO"
[ -n "$REV" ]  && set -- "$@" -r "$REV"
[ -f "$CONF" ] && set -- "$@" -c "$CONF"
set -- "$@" -profile "$PROFILE" -work-dir "$WORK"
[ -n "$OUT" ] && set -- "$@" --outdir "$OUT"

if [ -s "$PARAMS" ] && [ "$(jq 'length' "$PARAMS")" != "0" ]; then
  set -- "$@" -params-file "$PARAMS"
fi

if [ -n "$REV" ]; then
  echo "Executing: nextflow run $REPO -r $REV"
else
  echo "Executing: nextflow run $REPO"
fi
cd "${NOMAD_TASK_DIR}"
exec "$@"
EOH
      }

      config {
        command = "/bin/bash"
        args    = ["local/run.sh"]
      }
    }
  }
}
`

// ScriptHCL
//
// Generic bash/python/R/... runner. Script contents come from the dispatch
// payload (base64-encoded to survive JSON/shell quoting) so one
// registration serves arbitrary user scripts.
//
// Expected dispatch payload JSON:
//
//	{
//	  "interpreter": "python3",                // optional; default "bash"
//	  "script_b64": "ZWNobyBoaSAkMQ==",        // required; base64-encoded script
//	  "args":       ["--input", "/data/x"],    // optional string array
//	  "env":        {"FOO": "bar"}             // optional string map
//	}
//
// ----------------------------------------------------------------------------
const ScriptHCL = `
job "antelope-job" {
  datacenters = local.datacenters
  type        = "batch"

  parameterized {
    meta_optional = ["interpreter", "work_dir"]
    payload       = "required"
  }

  group "script" {
    restart {
      attempts = 0
      mode     = "fail"
    }
    reschedule {
      attempts  = 0
      unlimited = false
    }

    task "run" {
      driver = "raw_exec"

      dispatch_payload { file = "dispatch_payload.json" }

      resources {
        cores  = local.cores
        memory = local.memory
      }

      template {
        destination = "local/run.sh"
        perms       = "755"
        data        = <<EOH
#!/bin/bash
set -euo pipefail

PAYLOAD="${NOMAD_TASK_DIR}/dispatch_payload.json"
jq -e . "$PAYLOAD" >/dev/null || { echo "invalid JSON in dispatch payload"; exit 1; }

# Interpreter: payload > meta > default
INTERP=$(jq -r '.interpreter // empty' "$PAYLOAD")
[ -z "$INTERP" ] && INTERP="${NOMAD_META_interpreter}"
[ -z "$INTERP" ] && INTERP=bash

# Script content (base64-encoded — avoids JSON/shell quoting issues)
SCRIPT_B64=$(jq -r '.script_b64 // empty' "$PAYLOAD")
if [ -z "$SCRIPT_B64" ]; then
  echo "payload.script_b64 is required"; exit 1
fi

# Optional env vars
while IFS=$(printf '\t') read -r k v; do
  [ -n "$k" ] && export "$k=$v"
done < <(jq -r '.env // {} | to_entries[] | "\(.key)\t\(.value)"' "$PAYLOAD")

SCRIPT_WORKDIR="${NOMAD_META_work_dir}"
[ -z "$SCRIPT_WORKDIR" ] && SCRIPT_WORKDIR="${NOMAD_TASK_DIR}"
cd "$SCRIPT_WORKDIR"

SCRIPT_FILE="${NOMAD_TASK_DIR}/user_script"
echo "$SCRIPT_B64" | base64 -d > "$SCRIPT_FILE"
chmod +x "$SCRIPT_FILE"

# Build command using positional parameters — safe against injection
set -- "$INTERP" "$SCRIPT_FILE"
while IFS= read -r arg; do
  set -- "$@" "$arg"
done < <(jq -r '.args[]? // empty' "$PAYLOAD")

exec "$@"
EOH
      }

      config {
        command = "/bin/bash"
        args    = ["local/run.sh"]
      }
    }
  }
}
`

// WDLHCL (experimental)
//
// Runs a WDL workflow via miniwdl. Swap the final `exec miniwdl ...` line
// for a Cromwell invocation if you prefer that backend — everything else
// (payload contract, job shape) is identical.
//
// Expected dispatch payload JSON (supply EITHER wdl_url OR repository):
//
//	{
//	  "wdl_url":    "https://.../workflow.wdl",  // direct URL, OR
//	  "repository": "https://github.com/org/r",  // git repo to clone
//	  "revision":   "main",                      // branch/tag (optional)
//	  "wdl_path":   "path/to/workflow.wdl",      // default "main.wdl"
//	  "inputs":     { ... WDL inputs ... }       // written to inputs.json
//	}
//
// ----------------------------------------------------------------------------
const WDLHCL = `
job "antelope-job" {
  datacenters = local.datacenters
  type        = "batch"

  parameterized {
    meta_optional = ["work_dir", "outdir"]
    payload       = "required"
  }

  group "wdl" {
    restart {
      attempts = 0
      mode     = "fail"
    }
    reschedule {
      attempts  = 0
      unlimited = false
    }

    task "run" {
      driver = "raw_exec"

      dispatch_payload { file = "dispatch_payload.json" }

      resources {
        cores  = local.cores
        memory = local.memory
      }

      env {
        PATH = "${NOMAD_TASK_DIR}/bin:/run/current-system/sw/bin:${PATH}"
      }

      template {
        destination = "local/run.sh"
        perms       = "755"
        data        = <<EOH
#!/bin/bash
set -euo pipefail

PAYLOAD="${NOMAD_TASK_DIR}/dispatch_payload.json"
INPUTS="${NOMAD_TASK_DIR}/inputs.json"
WDL_FILE="${NOMAD_TASK_DIR}/workflow.wdl"

jq -e . "$PAYLOAD" >/dev/null || { echo "invalid JSON in dispatch payload"; exit 1; }
jq -r '.inputs // {}' "$PAYLOAD" > "$INPUTS"

WDL_URL=$(jq  -r '.wdl_url    // empty'      "$PAYLOAD")
REPO=$(jq     -r '.repository // empty'      "$PAYLOAD")
REV=$(jq      -r '.revision   // empty'      "$PAYLOAD")
WDL_PATH=$(jq -r '.wdl_path   // "main.wdl"' "$PAYLOAD")

if [ -n "$WDL_URL" ]; then
  curl -fsSL "$WDL_URL" -o "$WDL_FILE"
elif [ -n "$REPO" ]; then
  if [ -n "$REV" ]; then
    git clone --depth 1 --branch "$REV" "$REPO" "${NOMAD_TASK_DIR}/src"
  else
    git clone --depth 1 "$REPO" "${NOMAD_TASK_DIR}/src"
  fi
  cp "${NOMAD_TASK_DIR}/src/${WDL_PATH}" "$WDL_FILE"
else
  echo "payload must provide either wdl_url or repository"; exit 1
fi

OUT="${NOMAD_META_outdir}"
[ -z "$OUT" ] && OUT="${NOMAD_TASK_DIR}/results"
mkdir -p "$OUT"

WDL_WORKDIR="${NOMAD_META_work_dir}"
[ -z "$WDL_WORKDIR" ] && WDL_WORKDIR="${NOMAD_TASK_DIR}"
cd "$WDL_WORKDIR"
exec miniwdl run "$WDL_FILE" --input "$INPUTS" --dir "$OUT" --verbose
EOH
      }

      config {
        command = "/bin/bash"
        args    = ["local/run.sh"]
      }
    }
  }
}
`
