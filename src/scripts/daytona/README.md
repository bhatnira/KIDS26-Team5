# Daytona sandbox image for the Antelope agent

When a user selects **Daytona** as their code-execution runtime, every chat turn
gets a fresh sandbox created from a Daytona *snapshot*. This directory builds and
registers that snapshot.

The image exists to make one thing true: **a bundled skill should never fail
with "package not found."** Its package list is derived from the skills
themselves rather than guessed — see [How the package list was
derived](#how-the-package-list-was-derived).

| File | Purpose |
|---|---|
| `environment.yml` | The package list. Single source of truth for what the sandbox can run. |
| `Dockerfile` | Builds the image around that environment. |
| `verify-runtime.sh` | Proves every package imports/runs. Executed during the build; also installed in the image as `verify-runtime`. |
| `build-snapshot.sh` | Builds, verifies, and registers the snapshot with Daytona. |

## Quick start

```sh
# Check environment.yml resolves before paying for a full build (minutes, not
# the ~35 it takes to discover a conflict the slow way). Always do this first
# after editing the package list.
./scripts/daytona/build-snapshot.sh --solve-only

# Build and register (needs the daytona CLI, logged in).
DAYTONA_API_KEY=… ./scripts/daytona/build-snapshot.sh

# Push to a registry first — required when Daytona runs on another host.
DAYTONA_API_KEY=… REGISTRY=ghcr.io/myorg ./scripts/daytona/build-snapshot.sh

# Iterate on environment.yml without a Daytona endpoint.
./scripts/daytona/build-snapshot.sh --build-only
```

Then point the agent at it:

```yaml
agent:
  daytona:
    default-snapshot: antelope-bio       # must equal SNAPSHOT_NAME
    workspace-path: /home/user/trpc_agent_workspace   # must equal the Dockerfile
```

> **Mind the snapshot name.** This script defaults to `SNAPSHOT_NAME=antelope-bio`,
> which matches `config.yaml.example` and the built-in default — but
> `docker/config.yaml` and `helm/antelope/values.yaml` both ask for
> `antelope-daytona`. Build with `SNAPSHOT_NAME=antelope-daytona` for those
> deployments, or sandbox creation fails with a snapshot-not-found error.

> **Budget 30–60 minutes and ~40 GB of build disk for a cold build**, and note
> that the resulting image is ~13 GB. Conda's solve over ~250 packages is the
> slow part. On Apple Silicon the build runs under emulation and is slower
> still; prefer an amd64 machine or CI runner.

## What is in the image

- **Python 3.12** — numpy, pandas, scipy, matplotlib, seaborn, scikit-learn,
  statsmodels, networkx, plotly, numba, pyarrow, polars
- **Bio-Python** — biopython, pysam, cyvcf2, pybedtools, bioframe, pyBigWig,
  pyranges, gffutils, HTSeq, scikit-allel, primer3-py, sourmash
- **Single-cell / spatial** — scanpy, anndata, muon, leidenalg, harmonypy,
  scrublet, celltypist, decoupler, pydeseq2, scvelo, squidpy
- **Other domains** — rdkit (chemoinformatics), cobra (metabolic models),
  cooler/cooltools (Hi-C), lifelines / scikit-survival, scikit-image
- **R 4.3** — tidyverse essentials, data.table, Seurat + Signac, DESeq2, edgeR,
  limma, GenomicRanges, rtracklayer, clusterProfiler, ComplexHeatmap,
  scater/scran, dada2, phyloseq, flowCore, CATALYST, methylKit, minfi, sesame,
  xcms, coloc, susieR, plus **TwoSampleMR** and **MRPRESSO** from GitHub
- **Human reference annotation** — `org.Hs.eg.db`,
  `TxDb.Hsapiens.UCSC.hg38.knownGene`, `EnsDb.Hsapiens.v86`,
  `BSgenome.Hsapiens.UCSC.hg38` (~2 GB, but without them the enrichment and
  peak-annotation skills fail on their first line)
- **Command line** — samtools, bcftools, bedtools, seqkit, fastp, cutadapt,
  fastqc, multiqc, bwa/bwa-mem2, bowtie2, hisat2, minimap2, STAR, salmon,
  kallisto, featureCounts, StringTie, RSEM, GATK4, picard, freebayes, snpEff,
  plink/plink2, MACS3 (with a `macs2` shim), deepTools, Bismark, MAFFT, MUSCLE, IQ-TREE, BLAST,
  DIAMOND, HMMER, MMseqs2, Kraken2/Bracken, SPAdes, MEGAHIT, Flye, QUAST,
  sra-tools, Snakemake, Nextflow

`micromamba` stays in the image so the agent can install a missing package
mid-turn; the system prompt tells it how, and warns that the install is lost when
the sandbox is rebuilt next turn.

## Deliberately excluded

Leaving these out is a decision, not an oversight:

| Excluded | Why |
|---|---|
| PyTorch, scvi-tools, cell2location, cellpose | ~7 GB of GPU-oriented stack that only pays off on GPU workers. Roughly doubles the image for a handful of skills. |
| AlphaFold, BindCraft, ColabFold | Need GPUs and hundreds of GB of weights. Not installable in a general sandbox at any size. |
| `dorado` (nanopore basecalling) | GPU-only, multi-GB. |
| `pyscenic` | Its only bioconda build (0.12.1) pins `numpy <1.24`; `macs3` needs `numpy >=1.25`. Unsolvable together, and satisfying pySCENIC would hold the whole image's scientific stack at numpy 1.23. Install into a throwaway prefix when a skill needs it: `micromamba create -y -p /tmp/scenic -c conda-forge -c bioconda pyscenic`. |
| `idr` | Newest build caps at `python_abi 3.10`, which would hold the entire image back a Python release for one niche peak-reproducibility tool. |
| `decoupler` | bioconda's newest is 1.5.0, whose `np.arange(start=…)` is rejected by numba ≥0.61 (argument became positional-only) — it fails at import. No skill references it. `pip install --user decoupler` gets a fixed version. |
| `bioconductor-methylkit` | Current build (1.36.0) needs `r-base >=4.5`, so the `r-base=4.3` pin forces 1.28.0, which fails to load. Moving to R 4.5 shifts the whole Bioconductor generation — too large a change for one package. minfi/sesame still cover array methylation. On demand: `Rscript -e 'BiocManager::install("methylKit")'`. |
| `macs2` | Newest build (2.2.9.1) stops at `python_abi 3.11`, and macs3/cooltools/rdkit have no cp311 build — no single Python satisfies both MACS generations. **A `macs2` shim forwarding to macs3 is installed**, so skills calling `macs2` still run; it warns on stderr that results may differ from MACS2. |
| `pyopenms` | No version coexists with the rest: 3.4+ needs `numpy >=2.0` while cooltools caps `numpy <2.0`; 3.2.0 needs `numpy 1.23.*` which macs3 rejects; 3.3.0 needs `icu >=75.1`, colliding with rdkit's boost/icu. cooltools (21 skills) and rdkit (102) outweigh pyopenms (6). Install on demand: `micromamba create -y -p /tmp/openms -c conda-forge -c bioconda pyopenms`. |
| Cell Ranger, and other licensed tools | Proprietary; not redistributable in an image. |
| `ensembl-vep`, `metaphlan`, `bakta` databases | The tools are small, the reference databases are tens of GB and must be provisioned separately. |
| Mouse annotation (`org.Mm.eg.db`, mouse BSgenome/TxDb) | ~1.5 GB for the cross-species minority. Install on demand. |

When a skill needs one of these, the agent is instructed to say so rather than
flail.

## How the package list was derived

`environment.yml` is not a guess. Every `SKILL.md`, `.py`, `.R` and `.sh` file in
the built-in skill library (710 skills — see the "Agent Skills" section of
`AGENTS.md`) was parsed for:

- Python `import x` / `from x import …`, in both `.py` files and ```python
  fences inside `SKILL.md`
- R `library(x)`, `require(x)` and `x::` namespace calls
- shell commands in ```bash fences

…then ranked by how many skills reference each one. The head of that ranking:

| Python | n | R | n | CLI | n |
|---|---|---|---|---|---|
| `Bio` | 480 | `ggplot2` | 73 | `samtools` | 692 |
| `numpy` | 395 | `DESeq2` | 52 | `bcftools` | 514 |
| `pandas` | 376 | `dplyr` | 37 | `bedtools` | 128 |
| `matplotlib` | 239 | `Seurat` | 36 | `gatk` | 120 |
| `sklearn` | 133 | `org.Hs.eg.db` | 31 | `plink2` | 81 |
| `rdkit` | 102 | `rtracklayer` | 30 | `cutadapt` | 44 |
| `scanpy` | 91 | `clusterProfiler` | 26 | `fastp` | 42 |
| `pysam` | 88 | `TwoSampleMR` | 26 | `bowtie2` | 36 |

Re-run the analysis after a `make skills-bundle` that pulls in new upstream
skills, and add anything new that clears the bar.

## Two constraints that are easy to break

**1. The environment must be on `ENV PATH` *and* survive a login shell.**
The agent executes code through Daytona's `ExecuteCommand` API, and the
framework's `workspace_exec` tool runs it as `sh -lc "… python3 script.py"`.
`~/.bashrc` is never sourced, so `conda init` / `conda activate` never runs — the
Dockerfile therefore puts `/opt/env/bin` on `ENV PATH`, which Docker stores in
the image config and exec'd processes inherit. Replacing this with profile-based
activation will silently give the agent the system Python instead.

`ENV PATH` alone is not sufficient, though, because of the `-l`: Debian's
`/etc/profile` opens by *overwriting* `PATH` with a fixed system list, which
throws `/opt/env/bin` away again. The Dockerfile installs
`/etc/profile.d/10-antelope-path.sh` to put it back — `/etc/profile` sources
`profile.d` after the overwrite, so that is the only place the value survives.
Delete it and every agent command dies with `sh: N: python: not found`, on an
image where `docker run <image> python --version` still prints a version,
because that is a non-login shell. `verify-runtime` checks both shells for
exactly this reason.

**2. `linux/amd64` only.** Much of bioconda (gatk4, star, salmon, bismark, …)
publishes no `linux-aarch64` build, so an arm64 image cannot be solved at all.
`build-snapshot.sh` defaults to `--platform linux/amd64` and warns if you
override it.

**3. Version floors are load-bearing, not decoration.** Conda optimises for *a*
solution, not a good one. An unpinned `squidpy` resolved to 1.2.2 (2022), which
solved cleanly and then failed at import on `anndata._core.views.SparseCSCView`.
If a package would be silently broken at an old version, give it a floor. The
upper bound on `squidpy<1.8` is equally deliberate: ≥1.8 pulls `spatialdata`,
which needs `numpy >=2.0`, and cooltools caps `numpy <2.0` — 1.6.x is the window
that satisfies both spatial transcriptomics and Hi-C.

**4. The Python pin is load-bearing.** bioconda builds its Python extensions for
a rolling set of interpreters and currently skips 3.11 — cooltools, macs3 and
rdkit publish no `cp311` build at all, so `python=3.11` makes the environment
unsolvable regardless of what else you change. If you move the pin, run
`--solve-only` first; the failure mode is a wall of unrelated-looking
`liblzma`/`boost`/`icu` conflicts rather than an honest "no 3.11 build".

Related: there is no display and no Jupyter kernel in the sandbox —
`run_python_inline` writes a `.py` file and runs it with plain `python3`. The
image sets `MPLBACKEND=Agg` so a skill that imports matplotlib without choosing a
backend does not die.

## Verifying an image

`verify-runtime` runs automatically during the build and fails it on any problem.
To re-run it against a built image, or inside a live sandbox while debugging:

```sh
docker run --rm antelope-daytona:<tag> verify-runtime
```

It checks that `python3`/`Rscript` resolve *inside the conda env*, that every
Python module imports, that every R library attaches, that every CLI tool is on
`PATH`, and then runs behavioural smoke tests — a headless matplotlib render, a
minimal scanpy pipeline, an `org.Hs.eg.db` lookup, and a workspace write.

## Removed in this revision

Earlier versions of this directory carried `mv-shim.sh`, `runner.Dockerfile` and
`build-runner.sh`. All three existed to work around S3-backed Daytona *volumes*:
`mountpoint-s3` does not implement `rename(2)`, which broke the framework's
`mv`-based metadata commit, and the Daytona runner image shipped without
`mount-s3`.

The agent no longer mounts a volume — `internal/modules/agent/executor.go` builds
the executor with `WithLazyStart()` + `WithDeleteOnClose()` and no
`WithLazyVolume`, so the workspace lives on the sandbox's local disk and is
discarded with the sandbox each turn. Both workarounds were therefore dead code
and were removed. They remain in git history if volumes are ever reintroduced.
