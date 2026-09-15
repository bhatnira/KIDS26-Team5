#!/usr/bin/env bash
# build-snapshot.sh — build the Antelope agent sandbox image and register it as
# a Daytona snapshot. Idempotent: re-running with an existing snapshot name
# replaces the image it points at.
#
# The image is large (~13 GB) because it carries the Python, R and command-line
# runtime that the 710 bundled skills reference — see README.md. Expect the
# first build to take a while; conda's solve alone is several minutes.
#
# Environment:
#   DAYTONA_API_KEY   Daytona API key. Required unless --build-only.
#   DAYTONA_API_URL   Optional. Override the Daytona control-plane URL, e.g. a
#                     local deployment at http://localhost:3000/api. The CLI and
#                     SDKs read this variable directly. When unset the CLI uses
#                     whichever server is active in the current daytona profile.
#   IMAGE_NAME        Docker tag for the local build.
#                     Default: antelope-daytona:<git-short-sha>, falling back to
#                     a timestamp outside a git checkout.
#                     Daytona rejects the 'latest' tag — always use a real tag.
#   REGISTRY          Optional registry prefix. When set, the image is tagged and
#                     pushed there before the snapshot is created. Required when
#                     Daytona runs anywhere other than this machine.
#   SNAPSHOT_NAME     Snapshot name to register. Default: antelope-bio.
#                     Must match agent.daytona.default-snapshot in config.yaml.
#   PLATFORM          Build platform. Default: linux/amd64 — and you almost
#                     certainly should not change it. Large parts of bioconda
#                     (gatk4, star, salmon, bismark, …) publish no
#                     linux-aarch64 build, so an arm64 image cannot even be
#                     solved. On Apple Silicon this builds under emulation.
#
# Flags:
#   --solve-only      Resolve environment.yml and stop. No image layers, no
#                     downloads beyond repodata, and the repodata is cached in a
#                     docker volume between runs — so a broken environment.yml
#                     fails in minutes instead of after a full image build.
#                     Use this while iterating on the package list.
#   --build-only      Build (and verify) the image; skip registration. Useful for
#                     iterating without a Daytona endpoint.
#   --no-cache        Pass --no-cache to docker build, forcing a fresh solve.
#
# Usage:
#   DAYTONA_API_KEY=… ./scripts/daytona/build-snapshot.sh
#   DAYTONA_API_KEY=… REGISTRY=ghcr.io/myorg ./scripts/daytona/build-snapshot.sh
#   ./scripts/daytona/build-snapshot.sh --solve-only     # check the env resolves
#   ./scripts/daytona/build-snapshot.sh --build-only
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BUILD_ONLY=0
SOLVE_ONLY=0
DOCKER_BUILD_FLAGS=()
for arg in "$@"; do
    case "$arg" in
    --solve-only) SOLVE_ONLY=1 ;;
    --build-only) BUILD_ONLY=1 ;;
    --no-cache) DOCKER_BUILD_FLAGS+=(--no-cache) ;;
    -h | --help)
        sed -n '2,/^set -euo/p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//;$d'
        exit 0
        ;;
    *)
        echo "error: unknown argument '$arg' (try --help)" >&2
        exit 1
        ;;
    esac
done

# Daytona rejects 'latest'; derive a specific tag from the git SHA when possible.
_default_tag() {
    local sha
    sha=$(git -C "$SCRIPT_DIR" rev-parse --short HEAD 2>/dev/null) && echo "$sha" && return
    date +%Y%m%d%H%M%S
}
IMAGE_NAME="${IMAGE_NAME:-antelope-daytona:$(_default_tag)}"
SNAPSHOT_NAME="${SNAPSHOT_NAME:-antelope-bio}"
REGISTRY="${REGISTRY:-}"
DAYTONA_API_URL="${DAYTONA_API_URL:-}"
PLATFORM="${PLATFORM:-linux/amd64}"

# Export so the daytona CLI and SDKs pick it up automatically.
[[ -n "$DAYTONA_API_URL" ]] && export DAYTONA_API_URL

if [[ -n "$REGISTRY" ]]; then
    FULL_IMAGE="${REGISTRY%/}/${IMAGE_NAME}"
else
    FULL_IMAGE="$IMAGE_NAME"
fi

require() {
    command -v "$1" >/dev/null 2>&1 || {
        echo "error: $1 not found in PATH" >&2
        exit 1
    }
}

require docker

# ── Solve-only: check environment.yml resolves, then stop ────────────────────
# A conda conflict is by far the most likely reason this script fails, and
# finding it via a full image build is a ~35 minute round trip. The repodata
# cache volume makes the second and later runs substantially faster.
if [[ $SOLVE_ONLY -eq 1 ]]; then
    echo "==> Resolving environment.yml for $PLATFORM (no image is built)"
    docker volume create antelope-mamba-cache >/dev/null
    if docker run --rm --platform "$PLATFORM" \
        -v "$SCRIPT_DIR/environment.yml:/tmp/environment.yml:ro" \
        -v antelope-mamba-cache:/opt/conda/pkgs \
        mambaorg/micromamba:1.5.10 \
        micromamba create -y -n solvecheck -f /tmp/environment.yml --dry-run; then
        echo
        echo "environment.yml resolves. Run again with --build-only to build the image."
        exit 0
    fi
    echo
    echo "environment.yml does NOT resolve — see the conflict report above." >&2
    echo "Drop or relax the offending package in $SCRIPT_DIR/environment.yml," >&2
    echo "and remove it from verify-runtime.sh too, then re-run --solve-only." >&2
    exit 1
fi

if [[ $BUILD_ONLY -eq 0 ]]; then
    require daytona
    if [[ -z "${DAYTONA_API_KEY:-}" ]]; then
        echo "error: DAYTONA_API_KEY is required (or pass --build-only)" >&2
        exit 1
    fi
fi

if [[ "$PLATFORM" != "linux/amd64" ]]; then
    echo "warning: PLATFORM=$PLATFORM — bioconda has no linux-aarch64 build for" >&2
    echo "         several bundled tools, so the conda solve will likely fail." >&2
fi

echo "==> Building $IMAGE_NAME ($PLATFORM)"
echo "    This installs ~250 conda packages and runs the full runtime"
echo "    verification; budget 30-60 minutes for a cold build."
docker build \
    --platform "$PLATFORM" \
    "${DOCKER_BUILD_FLAGS[@]+"${DOCKER_BUILD_FLAGS[@]}"}" \
    -t "$IMAGE_NAME" \
    "$SCRIPT_DIR"

# The Dockerfile already ran verify-runtime, so reaching here means the image is
# sound. Report what it costs to pull, since that is per-sandbox cold-start time.
SIZE=$(docker image inspect "$IMAGE_NAME" --format '{{.Size}}' 2>/dev/null || echo 0)
echo "==> Built $IMAGE_NAME ($((SIZE / 1000 / 1000)) MB uncompressed)"

if [[ $BUILD_ONLY -eq 1 ]]; then
    cat <<EOF

Build-only mode; skipping snapshot registration.
Inspect the runtime with:
  docker run --rm -it $IMAGE_NAME bash
  docker run --rm $IMAGE_NAME verify-runtime
EOF
    exit 0
fi

if [[ -n "$REGISTRY" ]]; then
    echo "==> Tagging as $FULL_IMAGE and pushing"
    docker tag "$IMAGE_NAME" "$FULL_IMAGE"
    docker push "$FULL_IMAGE"
fi

echo "==> Registering snapshot '$SNAPSHOT_NAME' -> $FULL_IMAGE"
# The daytona CLI errors when a snapshot already exists; in that case delete and
# recreate so repeated dev/CI runs stay idempotent.
snapshot_err=$(mktemp)
trap 'rm -f "$snapshot_err"' EXIT
if ! daytona snapshot create "$SNAPSHOT_NAME" --image "$FULL_IMAGE" 2>"$snapshot_err"; then
    if grep -qi "already exists" "$snapshot_err"; then
        echo "==> Snapshot exists — recreating to pick up the new image"
        daytona snapshot delete "$SNAPSHOT_NAME" --yes
        daytona snapshot create "$SNAPSHOT_NAME" --image "$FULL_IMAGE"
    else
        cat "$snapshot_err" >&2
        exit 1
    fi
fi

cat <<EOF

Done. Point the agent at this snapshot in config.yaml:

  agent:
    daytona:
      default-snapshot: $SNAPSHOT_NAME

or set ANTELOPE_AGENT_DAYTONA_DEFAULT_SNAPSHOT=$SNAPSHOT_NAME.
EOF
