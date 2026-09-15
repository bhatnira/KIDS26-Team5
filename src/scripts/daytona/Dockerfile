# syntax=docker/dockerfile:1
#
# Antelope agent sandbox image for Daytona.
#
# This is the runtime the agent's 710 built-in skills execute in. The package
# list lives in environment.yml, which is derived from a static analysis of every
# import, library() call and shell command in the skill library — see README.md.
#
# Two facts about how the agent runs code drive the whole design:
#
#   1. Commands arrive through Daytona's ExecuteCommand API and are run by the
#      agent's workspace_exec tool as `sh -lc "... python3 script.py"`. Both
#      halves of that invocation are load-bearing:
#        - ~/.bashrc is never sourced and `conda activate` never runs, so the
#          environment has to be on ENV PATH, which Docker puts in the container
#          config and exec'd processes inherit. Do not replace this with a
#          shell-profile activation; it will silently stop working.
#        - `-l` makes it a LOGIN shell, and Debian's /etc/profile then overwrites
#          PATH wholesale, dropping /opt/env/bin straight back off it. The
#          /etc/profile.d shim further down puts it back. Without that shim every
#          command fails with "sh: N: python: not found" on an image that is
#          perfectly fine under `docker run`.
#   2. There is no display and no Jupyter kernel — `run_python_inline` writes a
#      .py file and runs it with plain python3. Hence MPLBACKEND=Agg, so a skill
#      that calls matplotlib without setting a backend does not die on import.
#
# Built and registered with Daytona via build-snapshot.sh.
#
# NOTE ON ARCHITECTURE: this image is linux/amd64 only. Large parts of bioconda
# (gatk4, star, salmon, bismark, …) publish no linux-aarch64 build, so an arm64
# image cannot be solved. build-snapshot.sh defaults to --platform linux/amd64
# for exactly this reason, including on Apple Silicon.
FROM debian:bookworm-slim

# ── System layer ─────────────────────────────────────────────────────────────
# Kept deliberately small; everything scientific comes from conda below.
#   build-essential + *-dev  → so the agent (and remotes::install_github) can
#                              compile an R or Python package that is not
#                              bundled. This is the documented escape hatch, and
#                              without a toolchain it fails confusingly.
#   procps/less/vim-tiny     → the agent inspects its own long-running commands.
#   git, curl, unzip         → skills fetch reference data and clone helper repos.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        wget \
        git \
        unzip \
        bzip2 \
        xz-utils \
        procps \
        less \
        vim-tiny \
        build-essential \
        gfortran \
        libcurl4-openssl-dev \
        libssl-dev \
        libxml2-dev \
    && rm -rf /var/lib/apt/lists/*

# ── micromamba ───────────────────────────────────────────────────────────────
# A single static binary rather than the mambaorg/micromamba image: that image
# ships its own uid-1000 `mambauser`, which collides with the `user` account the
# agent's workspace path is built around.
#
# Downloaded to a file and then extracted, rather than `curl … | tar -xj`: in a
# pipeline only tar's exit status propagates, so a failed download (proxy, 404,
# network blip) surfaces as a baffling "compressed file(s) have become corrupted"
# from bzip2 instead of the actual HTTP error.
ARG MICROMAMBA_VERSION=1.5.10
RUN curl -fsSL -o /tmp/micromamba.tar.bz2 \
        "https://micro.mamba.pm/api/micromamba/linux-64/${MICROMAMBA_VERSION}" \
    && tar -xj -C /usr/local -f /tmp/micromamba.tar.bz2 bin/micromamba \
    && rm -f /tmp/micromamba.tar.bz2 \
    && /usr/local/bin/micromamba --version

ENV MAMBA_ROOT_PREFIX=/opt/mamba \
    CONDA_PREFIX=/opt/env \
    PATH=/opt/env/bin:/home/user/.local/bin:$PATH

# ── Sandbox user ─────────────────────────────────────────────────────────────
# Created BEFORE the environment, and the environment is then built as this
# user, so every file under /opt/env ends up user-owned.
#
# That matters for the documented escape hatch: the agent is told it may
# `micromamba install` a missing package mid-turn, and it does not run as root.
# Installing as root and fixing it afterwards with `chown -R` is not an option —
# it would rewrite every inode and add a second ~12 GB layer to the image.
#
# The workspace path is coupled to agent.daytona.workspace-path in config.yaml
# (default /home/user/trpc_agent_workspace). Changing it here requires changing
# it there, and vice versa.
# /opt/env is deliberately NOT created here: `micromamba create -p` refuses a
# prefix that already exists as a plain directory ("Non-conda folder exists at
# prefix"). Making /opt itself user-owned lets micromamba create the prefix.
RUN useradd --create-home --shell /bin/bash user \
    && mkdir -p /opt/mamba \
    && chown user:user /opt /opt/mamba

# Set before the USER switch below: Docker does not derive HOME from /etc/passwd
# for a `USER` instruction, so without this micromamba and R would try to write
# their state into root's home and fail as the unprivileged build user.
ENV HOME=/home/user

# ── The scientific environment ───────────────────────────────────────────────
# One solve, not several sequential installs: conda resolves a single large
# specification far more reliably than a chain of installs that each get to
# downgrade what came before.
#
# The cache is dropped in the same layer as the install, otherwise the ~6 GB of
# package tarballs stay in the image forever.
USER user
COPY --chown=user:user environment.yml /tmp/environment.yml
RUN micromamba create -y -p /opt/env -f /tmp/environment.yml \
    && micromamba clean --all --yes \
    && rm -rf /opt/mamba/pkgs /home/user/.cache /tmp/environment.yml

# ── R packages that exist only on GitHub ─────────────────────────────────────
# TwoSampleMR is referenced by 26 skills and MRPRESSO by 7, and neither is
# published to conda or CRAN. They are installed in their own layer so a GitHub
# outage during a rebuild is obvious and easy to skip, rather than being buried
# in the middle of the conda solve.
# Note the absence of `options(warn = 2)` and `dependencies = TRUE`: the former
# turns routine "built under R version x.y" notes into build failures, and the
# latter drags in Suggests (test and vignette stacks) that fail far more often
# than they help. The `library()` call below is the check that actually matters.
RUN Rscript -e 'remotes::install_github(c("MRCIEU/TwoSampleMR", "rondolab/MR-PRESSO"), \
                                        upgrade = "never")' \
    && Rscript -e 'library(TwoSampleMR); library(MRPRESSO); cat("github R packages ok\n")' \
    && rm -rf /tmp/Rtmp* /home/user/.cache

USER root

# Runtime environment for every command the agent runs. Comments are kept above
# the instruction rather than between its continuation lines, which not every
# Dockerfile parser handles.
#
#   MPLBACKEND        headless plotting — the sandbox has no X server, and a
#                     skill that imports pyplot without picking a backend would
#                     otherwise fail on import.
#   MPLCONFIGDIR      matplotlib, numba and friends need a writable cache; they
#   NUMBA_CACHE_DIR   warn or fail when HOME belongs to a different uid than the
#   XDG_CACHE_HOME    one Daytona happens to exec as.
#   R_LIBS_USER       BiocManager::install() at runtime lands here.
#   PYTHONUSERBASE    `pip install --user` lands here; its bin dir is on PATH.
ENV MPLBACKEND=Agg \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    MPLCONFIGDIR=/home/user/.cache/matplotlib \
    NUMBA_CACHE_DIR=/home/user/.cache/numba \
    XDG_CACHE_HOME=/home/user/.cache \
    R_LIBS_USER=/home/user/R/library \
    PYTHONUSERBASE=/home/user/.local

# ── PATH for login shells ────────────────────────────────────────────────────
# ENV PATH above is not enough on its own. workspace_exec runs every command as
# `sh -lc "<command>"`, and Debian's /etc/profile starts by replacing PATH with a
# fixed system list (/usr/local/bin:/usr/bin:/bin:... for a non-root user). That
# happens after the process inherits ENV PATH, so /opt/env/bin is silently lost
# and python, Rscript and every bioconda CLI become "not found" — while
# `docker run <image> python --version`, which uses a non-login shell, keeps
# working and hides the problem.
#
# /etc/profile sources /etc/profile.d/*.sh *after* that overwrite, so a drop-in
# here is the one place the value survives. The name has to satisfy
# run-parts (^[a-zA-Z0-9_][a-zA-Z0-9._-]*\.sh$) or bookworm skips it silently.
RUN printf '%s\n' \
        '# Installed by scripts/daytona/Dockerfile — see the notes there.' \
        '# Debian /etc/profile overwrites PATH for login shells; workspace_exec' \
        '# runs `sh -lc`, so without this the conda env is invisible to it.' \
        '# Guarded so nested login shells do not stack duplicate entries.' \
        'case ":$PATH:" in' \
        '  *:/opt/env/bin:*) ;;' \
        '  *) PATH="/opt/env/bin:/home/user/.local/bin:$PATH" ;;' \
        'esac' \
        'export PATH' \
        > /etc/profile.d/10-antelope-path.sh \
    && chmod 0644 /etc/profile.d/10-antelope-path.sh \
    && sh -lc 'command -v python3 | grep -q "^/opt/env/" || { echo "login-shell PATH shim did not take effect" >&2; exit 1; }'

# Pre-create the workspace tree so the agent's first CreateWorkspace call is a
# no-op, and the caches/lib dirs so nothing has to mkdir them mid-run.
#
# These are world-writable on purpose: Daytona does not guarantee which uid it
# exec's commands as, and this is a single-tenant sandbox that is destroyed at
# the end of every turn, so a wrong-uid write failure is a much worse outcome
# than permissive modes on an ephemeral container.
RUN mkdir -p \
        /home/user/trpc_agent_workspace/skills \
        /home/user/trpc_agent_workspace/work/inputs \
        /home/user/trpc_agent_workspace/runs \
        /home/user/trpc_agent_workspace/out \
        /home/user/trpc_agent_workspace/src \
        /home/user/.cache/matplotlib \
        /home/user/.cache/numba \
        /home/user/R/library \
        /home/user/.local/bin \
    && chown -R user:user /home/user \
    && chmod -R 0777 \
        /home/user/trpc_agent_workspace \
        /home/user/.cache \
        /home/user/R \
        /home/user/.local

# ── macs2 → macs3 shim ───────────────────────────────────────────────────────
# macs2 cannot be installed alongside macs3 (see environment.yml), but ~14 skills
# call `macs2` by name. MACS3 is the same project's successor and keeps the
# subcommand interface, so forwarding is far more useful than "command not
# found" — provided it is not silent. Peak calls from MACS3 are not guaranteed
# identical to MACS2, so the shim announces the substitution on stderr, where the
# agent sees it and can report it. An unsupported flag surfaces as macs3's own
# error message, with the notice above it for context.
RUN printf '%s\n' \
        '#!/bin/sh' \
        '# Installed by scripts/daytona/Dockerfile — macs2 is not installable in' \
        '# this environment; MACS3 serves its callers. See the README.' \
        'echo "note: macs2 is not installed in this sandbox; running macs3 instead." >&2' \
        'echo "      MACS3 results may differ from MACS2 — say so when reporting them." >&2' \
        'exec macs3 "$@"' \
        > /usr/local/bin/macs2 \
    && chmod 0755 /usr/local/bin/macs2

# ── Build-time verification ──────────────────────────────────────────────────
# The point of this image is that skills do not hit "package not found", so the
# build fails if anything the library depends on cannot actually be imported or
# executed. A conda solve can succeed and still leave a broken package.
COPY verify-runtime.sh /usr/local/bin/verify-runtime
RUN chmod 0755 /usr/local/bin/verify-runtime \
    && /usr/local/bin/verify-runtime

USER user
WORKDIR /home/user

# Daytona keeps the container alive and exec's into it; there is no server to run.
CMD ["sleep", "infinity"]
