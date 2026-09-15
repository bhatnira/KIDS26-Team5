---
name: diff-visualizer
description: Rich downstream visualisation and reporting for bulk RNA-seq differential expression and scRNA marker/contrast outputs.
license: "MIT"
metadata: {"author":"Yonghao Zhao","openclaw":{"always":false,"emoji":"📈","homepage":"https://github.com/ClawBio/ClawBio","install":[{"kind":"uv","package":"pandas"},{"kind":"uv","package":"numpy"},{"kind":"uv","package":"matplotlib"},{"kind":"uv","package":"anndata"},{"kind":"uv","package":"scanpy"}],"os":["darwin","linux"],"requires":{"bins":["python3"]},"trigger_keywords":["visualize de results","differential expression visualization","marker heatmap","marker dotplot","top genes heatmap","diffviz"]},"tags":["differential-expression","visualisation","rnaseq","scrna","volcano","heatmap"],"version":"0.1.0"}
---

# 📈 Differential Visualizer

You are **Differential Visualizer**, a specialised ClawBio agent for turning completed bulk RNA-seq and single-cell differential outputs into richer figure and report packages.

## Why This Exists

- **Without it**: Users get one or two useful figures from upstream analysis, then hand-build publication-style plots and summary tables.
- **With it**: A completed DE/marker table can be repackaged into volcanoes, heatmaps, bar charts, HTML/Markdown reports, and reproducibility artifacts in one step.
- **Why ClawBio**: The skill stays local-first, composes directly with existing `rnaseq-de` and `scrna-orchestrator` outputs, and preserves machine-readable outputs.

## Core Capabilities

1. **Auto-detect upstream outputs** from `rnaseq-de`, `scrna-orchestrator`, or direct DE/marker tables.
2. **Bulk RNA visualisation** with volcano, MA, top-gene bars, and optional counts+metadata heatmaps.
3. **scRNA visualisation** with dataset-level contrast volcanoes, within-cluster comparison panels, marker ranking bars, and optional AnnData-based enhancement where the grouping axis is unambiguous.
4. **Reporting** with `report.md`, self-contained `report.html`, `result.json`, and reproducibility files.

## Input Formats

| Format | Extension | Required Fields | Example |
|--------|-----------|-----------------|---------|
| rnaseq-de output directory | directory | `tables/de_results.csv` | `output/rnaseq_20260315/` |
| scrna-orchestrator output directory | directory | `tables/contrastive_markers_full.csv`, `tables/within_cluster_contrastive_markers_full.csv`, or `tables/markers_top.csv` | `output/scrna_20260315/` |
| Bulk DE table | `.csv`, `.tsv` | `gene`, `log2FoldChange`, plus `padj` or `pvalue` | `de_results.csv` |
| scRNA contrast table | `.csv`, `.tsv` | `names`, `scores` | `contrastive_markers_full.csv` |
| scRNA within-cluster contrast table | `.csv`, `.tsv` | `cluster`, `comparison_id`, `group1`, `group2`, `names`, `scores` | `within_cluster_contrastive_markers_full.csv` |
| scRNA markers table | `.csv`, `.tsv` | `cluster`, `names`, `scores` | `markers_top.csv` |
| Optional bulk counts | `.csv`, `.tsv` | gene rows, sample columns, first column gene id | `counts.csv` |
| Optional bulk metadata | `.csv`, `.tsv` | `sample_id` | `metadata.csv` |
| Optional AnnData | `.h5ad` | expression matrix plus gene names in `var_names` | `subset.h5ad` |

## Workflow

When the user asks to visualise differential expression or marker results:

1. **Detect**: Identify whether the input is bulk or scRNA, and whether it is an output directory or a direct result table.
2. **Validate**: Confirm required columns and reject ambiguous/unsupported inputs with clear guidance.
3. **Render**:
   - Bulk: volcano, top-gene bars, optional MA plot, optional heatmap.
   - scRNA: dataset-level contrast volcanoes, within-cluster marker panels, marker ranking bars, and optional AnnData UMAP/grouped panels when the inputs support a single grouping axis.
4. **Report**: Write `report.md`, `report.html`, `result.json`, tables, figures, and reproducibility files.

## CLI Reference

```bash
# Bulk table
python skills/diff-visualizer/diff_visualizer.py \
  --input de_results.csv --output diffviz_report

# Bulk directory with extra heatmap inputs
python skills/diff-visualizer/diff_visualizer.py \
  --input output/rnaseq_run --counts counts.csv --metadata metadata.csv \
  --output diffviz_report

# scRNA contrast table with AnnData enhancement
python skills/diff-visualizer/diff_visualizer.py \
  --mode scrna --input contrastive_markers_full.csv --adata cells.h5ad \
  --output diffviz_report

# Demo
python skills/diff-visualizer/diff_visualizer.py --demo --output /tmp/diffviz_demo
python skills/diff-visualizer/diff_visualizer.py --demo --mode scrna --output /tmp/diffviz_scrna_demo

# Via ClawBio runner
python clawbio.py run diffviz --input de_results.csv --output diffviz_report
python clawbio.py run diffviz --demo
```

## Demo

```bash
python clawbio.py run diffviz --demo
python clawbio.py run diffviz --demo --mode scrna
```

Expected outputs:
- `report.md`
- `report.html`
- `result.json`
- figure bundle in `figures/`
- summary tables in `tables/`
- reproducibility files in `reproducibility/`

## Output Structure

```text
output_directory/
├── report.md
├── report.html
├── result.json
├── figures/
│   ├── volcano.png
│   ├── top_genes_bar.png
│   ├── ma_plot.png
│   ├── top_genes_heatmap.png
│   ├── contrast_volcano.png
│   ├── top_markers_bar.png
│   ├── marker_rank_bars.png
│   ├── marker_dotplot.png
│   ├── marker_heatmap.png
│   └── umap_feature_panel.png
├── tables/
│   ├── top_genes.csv
│   ├── significant_genes.csv
│   ├── top_markers.csv
│   └── top_markers_by_cluster.csv
└── reproducibility/
    ├── commands.sh
    ├── environment.yml
    └── checksums.sha256
```

## Safety

- Local-first only.
- Reports include the ClawBio medical/research disclaimer.
- No DE statistics are recomputed beyond lightweight visual ranking/summary logic.
- Enhanced scRNA plots degrade gracefully if `anndata`/`scanpy` context is unavailable.

## Integration with Bio Orchestrator

- Routes from phrases like “visualize DE results”, “marker heatmap”, “marker dotplot”, and “top genes heatmap”.
- Works downstream of `rnaseq-de` and `scrna-orchestrator`.

## Citations

- Scanpy documentation: https://scanpy.readthedocs.io/
- Matplotlib documentation: https://matplotlib.org/
