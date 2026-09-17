package runner

// SlurmBatchScript is a template for Slurm batch scripts.
// It mirrors the functionality of the Nomad HCL templates but uses SBATCH directives.
const SlurmBatchScript = `#!/bin/bash
#SBATCH --job-name={{.JobID}}
#SBATCH --partition={{.Partition}}
#SBATCH --time={{.TimeLimit}}
#SBATCH --cpus-per-task={{.Cores}}
#SBATCH --mem={{.Memory}}M
#SBATCH --output={{.WorkDir}}/{{.JobID}}-%j.out
#SBATCH --error={{.WorkDir}}/{{.JobID}}-%j.err
{{- if .Account}}
#SBATCH --account={{.Account}}
{{- end}}
{{- if .QoS}}
#SBATCH --qos={{.QoS}}
{{- end}}

set -euo pipefail

export NXF_OPTS="-Xms512m -Xmx2g"
export NXF_WORK={{.WorkDir}}/work

# Download Nextflow if not in PATH
if ! command -v nextflow &> /dev/null; then
  mkdir -p {{.WorkDir}}/bin
  curl -s https://get.nextflow.io | bash -s -- -d {{.WorkDir}}/bin
  export PATH={{.WorkDir}}/bin:$PATH
fi

# Write dispatch payload
PAYLOAD_FILE={{.WorkDir}}/{{.JobID}}-payload.json
cat > "$PAYLOAD_FILE" << 'PAYLOAD_EOF'
{{.Payload}}
PAYLOAD_EOF

# Extract payload fields
REPO=$(jq -r '.repository // empty' "$PAYLOAD_FILE")
REV=$(jq -r '.revision // empty' "$PAYLOAD_FILE")
PARAMS_FILE=$(mktemp)
jq -r '.params // {}' "$PAYLOAD_FILE" > "$PARAMS_FILE"

# Build Nextflow command
set -- nextflow run "$REPO"
[ -n "$REV" ] && set -- "$@" -r "$REV"
set -- "$@" -profile docker -work-dir "$NXF_WORK"
set -- "$@" --outdir results
[ -s "$PARAMS_FILE" ] && [ "$(jq 'length' "$PARAMS_FILE")" != "0" ] && set -- "$@" -params-file "$PARAMS_FILE"

echo "Executing: $@"
exec "$@"
`
