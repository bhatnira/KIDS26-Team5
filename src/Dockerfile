# syntax=docker/dockerfile:1
# Build the frontend on the native platform to avoid QEMU-related issues with the
# nodejs ecosystem, then cross-build the backend per target platform.
FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.25-alpine AS frontend-build
# Pin pnpm rather than using Alpine's unpinned `pnpm` package: newer pnpm
# (v10.16+/v11) renamed the build-script allowlist (onlyBuiltDependencies ->
# allowBuilds) and defaults strictDepBuilds=true, so it fails the build on
# unreviewed scripts and ignores our pnpm-workspace.yaml allowlist. Pin to the
# version the team validates with locally to keep behavior deterministic.
ARG PNPM_VERSION=10.11.1
RUN apk --no-cache add build-base git nodejs npm \
    && npm install -g pnpm@${PNPM_VERSION}
WORKDIR /src
# Install deps first (cached layer keyed on the lockfile), then build via make.
COPY Makefile ./
COPY web_src/package.json web_src/pnpm-lock.yaml web_src/pnpm-workspace.yaml web_src/
RUN --mount=type=cache,target=/root/.local/share/pnpm/store make frontend-deps
COPY --exclude=.git/ . .
RUN --mount=type=cache,target=/root/.local/share/pnpm/store make frontend

# Build the backend. The build commands live in the Makefile so they stay in
# lock-step with local/CI/goreleaser builds.
FROM docker.io/library/golang:1.25-alpine AS build-env
ARG VERSION=dev
RUN apk --no-cache add build-base git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY --exclude=.git/ . .
# Drop in the frontend built on the native platform so `make server-embed`
# (-tags embed) can pick it up via //go:embed.
COPY --from=frontend-build /src/web_src/dist web_src/dist
# The agent skill bundle is generated on the host (`make skills-bundle`) rather
# than here, because generating it needs the skills/upstream/ submodule trees —
# ~380 MB of working tree that would have to be shipped in the build context to
# produce a ~23 MiB result. Fail early and say so, rather than letting the
# //go:embed in skillbundle fail with a bare "pattern bundle: no matching files".
RUN test -f internal/modules/agent/skillbundle/bundle/index.json || { \
      echo "ERROR: the agent skill bundle is missing."; \
      echo "       Run this on the host before docker build:"; \
      echo "         git submodule update --init --depth 1 --recursive"; \
      echo "         make skills-bundle"; \
      exit 1; \
    }
RUN --mount=type=cache,target="/root/.cache/go-build" \
    CGO_ENABLED=0 make server-embed VERSION=${VERSION}

# Minimal runtime image — a single self-contained server binary. The compiled
# frontend, the agent skill library and tzdata all live inside it, so the only
# thing a deployment must supply is configuration.
FROM docker.io/library/alpine:3.21 AS antelope
EXPOSE 8086
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=build-env /src/bin/antelope ./antelope
# The binary unpacks its embedded skill library here on first start (see
# agent.skills.bundle-cache-root in docker/config.yaml). Pre-creating the
# directory keeps that first write cheap and makes the path obvious to anyone
# mounting an emptyDir over it under a read-only root filesystem.
RUN mkdir -p /app/skills/bundle
ENTRYPOINT ["./antelope", "web"]
