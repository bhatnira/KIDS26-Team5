#!/usr/bin/env bash
# verify-runtime.sh — prove the sandbox can actually run the skill library.
#
# Run at image build time (so a broken environment fails the build rather than a
# user's chat turn) and installed as /usr/local/bin/verify-runtime so it can be
# re-run inside a live sandbox when debugging.
#
# It checks the things a conda solve does NOT: that every package the skills
# import can be imported, that every R library can be attached, and that every
# CLI tool is on PATH — both in this shell and in the login shell the agent's
# workspace_exec tool actually runs commands under.
#
# Exit status is the number of failures, capped at 125.
set -uo pipefail

failures=0
section() { printf '\n\033[1m== %s\033[0m\n' "$1"; }
ok() { printf '  \033[32mok\033[0m   %s\n' "$1"; }
bad() {
    printf '  \033[31mFAIL\033[0m %s\n' "$1"
    failures=$((failures + 1))
}

# ── Interpreters resolve from PATH ───────────────────────────────────────────
# This mirrors how the agent invokes code: a bare command name, resolved by a
# non-interactive shell, with no conda activation.
section "interpreters on PATH"
for bin in python3 Rscript; do
    if command -v "$bin" >/dev/null 2>&1; then
        ok "$bin -> $(command -v "$bin")"
    else
        bad "$bin is not on PATH"
    fi
done
if [[ "$(command -v python3)" != /opt/env/* ]]; then
    bad "python3 resolves outside the conda env — the agent would run the wrong interpreter"
fi

# ── … and in a login shell ───────────────────────────────────────────────────
# Not redundant with the check above. workspace_exec runs every command as
# `sh -lc "<command>"`, and Debian's /etc/profile replaces PATH wholesale for a
# login shell, dropping /opt/env/bin. The /etc/profile.d shim in the Dockerfile
# puts it back; this is what proves it still does. The symptom when it does not
# is a bare `sh: N: python: not found` from the agent on an image where
# `docker run <image> python --version` — a non-login shell — works fine, so it
# is invisible to every other check in this script.
section "interpreters in a login shell (sh -lc)"
for bin in python python3 Rscript samtools; do
    resolved=$(sh -lc "command -v $bin" 2>/dev/null) || resolved=""
    if [[ "$resolved" == /opt/env/* ]]; then
        ok "sh -lc: $bin -> $resolved"
    elif [[ -n "$resolved" ]]; then
        bad "sh -lc: $bin resolves to $resolved, outside the conda env"
    else
        bad "sh -lc: $bin not found — /etc/profile.d/10-antelope-path.sh missing or not sourced"
    fi
done

# ── Python imports ───────────────────────────────────────────────────────────
# Import names, not conda package names: biopython installs as `Bio`,
# scikit-learn as `sklearn`, primer3-py as `primer3`, and so on. Getting this
# mapping wrong is exactly the failure this script exists to catch.
section "python imports"
PY_MODULES=(
    numpy pandas scipy matplotlib seaborn sklearn statsmodels networkx plotly
    h5py zarr numba joblib pyarrow polars openpyxl requests yaml tqdm click rich
    pydantic jinja2 reportlab upsetplot adjustText pytest
    Bio pysam cyvcf2 pybedtools bioframe pyBigWig pyfaidx pyranges gffutils HTSeq
    allel primer3 intervaltree sourmash
    scanpy anndata mudata muon leidenalg igraph harmonypy scrublet scanorama
    celltypist pydeseq2 scvelo squidpy
    cooler cooltools
    rdkit matchms cobra
    lifelines sksurv skimage tifffile imageio umap shap
)
missing_py=$(python3 - "${PY_MODULES[@]}" <<'PYEOF'
import importlib, sys
bad = []
for name in sys.argv[1:]:
    try:
        importlib.import_module(name)
    except Exception as exc:                      # noqa: BLE001 - report, don't raise
        bad.append(f"{name}: {type(exc).__name__}: {exc}")
print("\n".join(bad))
PYEOF
)
if [[ -z "$missing_py" ]]; then
    ok "${#PY_MODULES[@]} modules import"
else
    while IFS= read -r line; do bad "python: $line"; done <<<"$missing_py"
fi

# ── R libraries ──────────────────────────────────────────────────────────────
section "R libraries"
R_LIBS=(
    ggplot2 dplyr tidyr readr stringr purrr tibble data.table Matrix MASS
    survival lme4 broom metafor pwr Hmisc pheatmap RColorBrewer scales ggrepel
    patchwork circlize ggseqlogo vegan ape iNEXT knitr rmarkdown optparse
    jsonlite yaml R.utils writexl RSQLite BiocManager remotes devtools
    Seurat Signac harmony presto SoupX
    coloc susieR MendelianRandomization ieugwasr TwoSampleMR MRPRESSO
    BiocParallel Biostrings S4Vectors IRanges GenomeInfoDb GenomicRanges
    GenomicFeatures GenomicAlignments SummarizedExperiment SingleCellExperiment
    AnnotationDbi Rsamtools rtracklayer VariantAnnotation biomaRt
    DESeq2 edgeR limma apeglm glmGamPoi sva tximport tximeta clusterProfiler
    enrichplot ReactomePA qvalue ComplexHeatmap EnhancedVolcano Mfuzz
    scater scran scuttle MAST celldex infercnv ChIPseeker DiffBind motifmatchr
    chromVAR TFBSTools JASPAR2024 minfi sesame
    dada2 phyloseq decontam ALDEx2 flowCore flowWorkspace CATALYST xcms ropls
    org.Hs.eg.db TxDb.Hsapiens.UCSC.hg38.knownGene EnsDb.Hsapiens.v86
    BSgenome.Hsapiens.UCSC.hg38
)
# The failure message is reported, not just the name. A bare "cannot be loaded"
# is undiagnosable without shell access to the half-built image, which is exactly
# the situation this script exists to avoid — the R error text names the missing
# shared library or unsatisfiable dependency directly.
r_out=$(Rscript --vanilla -e '
  libs <- commandArgs(trailingOnly = TRUE)
  for (l in libs) {
    msg <- tryCatch({
      suppressWarnings(suppressMessages(loadNamespace(l)))
      NA_character_
    }, error = function(e) conditionMessage(e))
    if (!is.na(msg)) {
      cat(sprintf("%s :: %s\n", l, gsub("[\r\n]+", " ", msg)))
    }
  }
' "${R_LIBS[@]}" 2>/dev/null)
r_out=$(printf '%s' "$r_out" | sed '/^[[:space:]]*$/d')
if [[ -z "$r_out" ]]; then
    ok "${#R_LIBS[@]} libraries load"
else
    while IFS= read -r line; do bad "R: ${line}"; done <<<"$r_out"
fi

# ── Command-line tools ───────────────────────────────────────────────────────
section "command-line tools"
# "a|b" means either name satisfies the check. Several bioconda recipes disagree
# with upstream on capitalisation or the .py suffix, and which one you get varies
# by version — the requirement is that the tool is callable, not that it is
# spelled a particular way. Without this, a rename upstream fails the build after
# the entire conda solve has already run.
CLI_TOOLS=(
    samtools bcftools tabix bgzip bedtools bedops seqkit seqtk csvtk sambamba
    bamtools gffread pigz parallel
    fastp cutadapt fastqc multiqc trimmomatic umi_tools qualimap
    bwa bwa-mem2 bowtie2 hisat2 minimap2 STAR salmon kallisto featureCounts
    stringtie rsem-calculate-expression
    gatk picard freebayes vcftools 'snpEff|snpeff' plink plink2
    macs3 macs2 bamCoverage computeMatrix plotHeatmap bismark
    mafft muscle clustalo trimal iqtree 'FastTree|fasttree'
    blastn blastp makeblastdb diamond hmmscan hmmsearch 'mmseqs|mmseqs2'
    kraken2 bracken 'spades.py|spades' megahit flye 'quast.py|quast' prodigal
    prefetch fasterq-dump esearch efetch
    snakemake nextflow micromamba
)
missing_cli=()
for spec in "${CLI_TOOLS[@]}"; do
    found=0
    # shellcheck disable=SC2086 # intentional word splitting on the | alternatives
    IFS='|' read -ra alternatives <<<"$spec"
    for tool in "${alternatives[@]}"; do
        if command -v "$tool" >/dev/null 2>&1; then
            found=1
            break
        fi
    done
    [[ $found -eq 1 ]] || missing_cli+=("$spec")
done
if [[ ${#missing_cli[@]} -eq 0 ]]; then
    ok "${#CLI_TOOLS[@]} tools on PATH"
else
    for tool in "${missing_cli[@]}"; do bad "cli: $tool not on PATH"; done
fi

# ── Behavioural smoke tests ──────────────────────────────────────────────────
# Presence is not the same as working. These catch the classic runtime traps:
# a matplotlib that needs a display, an R annotation package whose SQLite blob
# did not ship, and a samtools built against the wrong htslib.
#
# Each test reports the real error rather than a guess at what went wrong.
# Swallowing stderr here once turned a plain "Permission denied" into the
# thoroughly misleading "matplotlib cannot render without a display".
section "smoke tests"

# run_py <success-message> <failure-prefix> <python-code>
run_py() {
    local ok_msg="$1" fail_msg="$2" code="$3" out
    if out=$(python3 -c "$code" 2>&1); then
        ok "$ok_msg"
    else
        bad "$fail_msg: $(printf '%s' "$out" | tail -n 1)"
    fi
}

# Scratch output goes to a per-run temp directory that is cleaned up afterwards.
# A fixed path like /tmp/_vfy.png is worse than it looks: the build-time run
# executes as root and bakes that file into the image, so every later run as the
# unprivileged sandbox user fails to overwrite it — a permission error that has
# nothing to do with the thing being tested.
#
# A directory rather than mktemp'd file so the name can end in .png, which is how
# matplotlib picks the output format.
scratch=$(mktemp -d "${TMPDIR:-/tmp}/verify-runtime.XXXXXX") || scratch=""
trap '[[ -n "${scratch:-}" ]] && rm -rf "$scratch"' EXIT
mpl_out="${scratch:+$scratch/plot.png}"

if [[ -n "$mpl_out" ]]; then
    run_py "matplotlib renders headless (Agg)" "matplotlib render failed" "
import matplotlib; matplotlib.use('Agg')
import matplotlib.pyplot as plt
fig, ax = plt.subplots(); ax.plot([0, 1], [1, 0]); fig.savefig('$mpl_out')
"
else
    bad "could not create a temp file to test matplotlib rendering"
fi

run_py "scanpy runs a minimal pipeline" "scanpy pipeline failed" "
import scanpy as sc, anndata, numpy as np
a = anndata.AnnData(np.random.default_rng(0).poisson(1.0, (60, 40)).astype('float32'))
sc.pp.normalize_total(a); sc.pp.log1p(a); sc.pp.pca(a, n_comps=5)
"

if Rscript --vanilla -e '
  suppressMessages(library(org.Hs.eg.db))
  s <- AnnotationDbi::select(org.Hs.eg.db, keys = "TP53",
                             keytype = "SYMBOL", columns = "ENTREZID")
  if (nrow(s) < 1) quit(status = 1)
' >/dev/null 2>&1; then
    ok "org.Hs.eg.db resolves a gene symbol"
else
    bad "org.Hs.eg.db is present but cannot be queried"
fi

if samtools --version >/dev/null 2>&1 && bcftools --version >/dev/null 2>&1; then
    ok "samtools/bcftools execute"
else
    bad "samtools or bcftools is present but does not run"
fi

# Checked via the mode bits rather than -w, because this also runs at build time
# as root, for whom everything looks writable. Daytona does not guarantee which
# uid it exec's as, so the directory has to be writable by any of them.
ws_out=/home/user/trpc_agent_workspace/out
if [[ -d $ws_out ]] && [[ "$(stat -c '%a' "$ws_out")" == 777 ]]; then
    ok "workspace tree exists and is writable by any uid"
else
    bad "workspace $ws_out is missing or not mode 0777 (got: $(stat -c '%a' "$ws_out" 2>/dev/null || echo absent))"
fi

# The conda env must be writable by the sandbox user, or the runtime-install
# escape hatch the system prompt advertises fails with permission denied.
env_owner=$(stat -c '%U' /opt/env 2>/dev/null || echo '?')
if [[ "$env_owner" == "user" ]]; then
    ok "/opt/env is owned by the sandbox user (runtime installs work)"
else
    bad "/opt/env is owned by '$env_owner', not 'user' — micromamba install will fail at runtime"
fi

# ── Result ───────────────────────────────────────────────────────────────────
printf '\n'
if [[ $failures -eq 0 ]]; then
    printf '\033[32mAll runtime checks passed.\033[0m\n'
    exit 0
fi
printf '\033[31m%d runtime check(s) failed.\033[0m\n' "$failures"
printf 'Fix scripts/daytona/environment.yml and rebuild.\n'
exit $((failures > 125 ? 125 : failures))
