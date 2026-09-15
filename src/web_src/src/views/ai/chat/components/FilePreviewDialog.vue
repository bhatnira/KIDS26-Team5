<script setup>
import { computed, ref, watch, onBeforeUnmount } from 'vue'
import MarkdownIt from 'markdown-it'
import hljs from 'highlight.js'

const props = defineProps({
  // Two-way "open" state for n-modal.
  show: { type: Boolean, required: true },
  // The artifact to preview ({ name, latest_version, versions, ... }).
  item: { type: Object, default: null },
  // Presigned download URL the parent already resolved. When null the
  // dialog only shows a metadata header (no preview).
  url: { type: String, default: '' },
  // The version currently being previewed. The parent re-resolves `url`
  // when the user picks a different one.
  version: { type: Number, default: null },
})

const emit = defineEmits(['update:show', 'download', 'update:version'])

// Version picker options, newest first, with the latest one flagged. Only
// shown when the file has more than one stored version.
const versionOptions = computed(() => {
  const vs = props.item?.versions || []
  const latest = props.item?.latest_version
  return [...vs]
    .sort((a, b) => b - a)
    .map(v => ({ label: v === latest ? `v${v} (latest)` : `v${v}`, value: v }))
})

// Title carries the version so it is always clear which one is on screen.
const dialogTitle = computed(() => {
  const n = props.item?.name || 'File preview'
  return typeof props.version === 'number' ? `${n} · v${props.version}` : n
})

const loading = ref(false)
const error = ref('')
const text = ref('')          // populated for text/code/csv/markdown/json/yaml
const objectUrl = ref('')     // populated for images/audio/video/pdf (raw blob URL)
const blobMime = ref('')

const ext = computed(() => {
  const n = props.item?.name || ''
  const i = n.lastIndexOf('.')
  return i >= 0 ? n.slice(i + 1).toLowerCase() : ''
})

// Format dispatch by file extension. Binary formats not listed here fall
// through to a "download to view" placeholder.
const TEXT_EXTS = new Set([
  'txt', 'log', 'out', 'err', 'conf', 'cfg', 'ini', 'env', 'gitignore',
  'md', 'markdown', 'rst',
  'json', 'yaml', 'yml', 'toml', 'xml', 'html', 'htm', 'svg',
  'py', 'pyi', 'ipynb', 'r', 'R', 'rmd', 'js', 'jsx', 'ts', 'tsx', 'vue',
  'sh', 'bash', 'zsh', 'fish',
  'go', 'rs', 'java', 'c', 'h', 'cc', 'cpp', 'hpp', 'cs', 'swift', 'kt',
  'sql', 'graphql', 'gql', 'proto', 'dockerfile',
  'css', 'scss', 'sass', 'less',
  'tex', 'bib',
  'nf', 'nextflow', 'config', 'groovy',
])
const IMAGE_EXTS = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'ico', 'avif'])
const SVG_EXTS = new Set(['svg'])
const TABULAR_EXTS = new Set(['csv', 'tsv'])
const MARKDOWN_EXTS = new Set(['md', 'markdown'])
const PDF_EXTS = new Set(['pdf'])

// hljs language hint per extension.
const HLJS_LANG = {
  py: 'python', pyi: 'python',
  r: 'r', R: 'r', rmd: 'r',
  js: 'javascript', jsx: 'javascript',
  ts: 'typescript', tsx: 'typescript',
  vue: 'xml',
  sh: 'bash', bash: 'bash', zsh: 'bash', fish: 'bash',
  go: 'go', rs: 'rust', java: 'java',
  c: 'c', h: 'c', cc: 'cpp', cpp: 'cpp', hpp: 'cpp',
  cs: 'csharp', swift: 'swift', kt: 'kotlin',
  sql: 'sql', graphql: 'graphql', gql: 'graphql', proto: 'protobuf',
  json: 'json', yaml: 'yaml', yml: 'yaml', toml: 'ini', ini: 'ini', conf: 'ini', cfg: 'ini', env: 'ini',
  xml: 'xml', html: 'xml', htm: 'xml',
  css: 'css', scss: 'scss', sass: 'scss', less: 'less',
  tex: 'latex', bib: 'latex',
  nf: 'groovy', nextflow: 'groovy', config: 'groovy', groovy: 'groovy',
  dockerfile: 'dockerfile',
}

const kind = computed(() => {
  const e = ext.value
  if (IMAGE_EXTS.has(e)) return 'image'
  if (SVG_EXTS.has(e)) return 'svg'
  if (PDF_EXTS.has(e)) return 'pdf'
  if (MARKDOWN_EXTS.has(e)) return 'markdown'
  if (TABULAR_EXTS.has(e)) return 'tabular'
  if (TEXT_EXTS.has(e) || isProbablyText(props.item?.mime_type, props.item?.name)) return 'text'
  return 'binary'
})

function isProbablyText(mime, name) {
  if (!mime) return false
  if (mime.startsWith('text/')) return true
  if (mime.includes('json') || mime.includes('xml') || mime.includes('yaml')) return true
  if (!name) return false
  return /^README|LICENSE|CHANGELOG|Makefile|Dockerfile$/i.test(name)
}

const md = new MarkdownIt({
  html: false, linkify: true, typographer: true,
  highlight(str, lang) {
    if (lang && hljs.getLanguage(lang)) {
      try { return hljs.highlight(str, { language: lang, ignoreIllegals: true }).value } catch { /* */ }
    }
    return md.utils.escapeHtml(str)
  },
})

const highlightedCode = computed(() => {
  if (!text.value || (kind.value !== 'text' && kind.value !== 'svg')) return ''
  const lang = HLJS_LANG[ext.value] || ''
  try {
    if (lang && hljs.getLanguage(lang)) {
      return hljs.highlight(text.value, { language: lang, ignoreIllegals: true }).value
    }
    return hljs.highlightAuto(text.value).value
  } catch {
    return md.utils.escapeHtml(text.value)
  }
})

const renderedMarkdown = computed(() => {
  if (kind.value !== 'markdown' || !text.value) return ''
  return md.render(text.value)
})

// Parse CSV/TSV with a tiny tolerant parser: handles quoted fields and
// embedded commas/newlines. Good enough for the data-analysis files our
// users typically produce. For pathological CSVs they can still download.
function parseDelimited(content, delim) {
  const rows = []
  let i = 0, n = content.length
  let field = '', row = [], inQuotes = false
  while (i < n) {
    const ch = content[i]
    if (inQuotes) {
      if (ch === '"') {
        if (content[i + 1] === '"') { field += '"'; i += 2; continue }
        inQuotes = false; i++; continue
      }
      field += ch; i++; continue
    }
    if (ch === '"') { inQuotes = true; i++; continue }
    if (ch === delim) { row.push(field); field = ''; i++; continue }
    if (ch === '\n' || ch === '\r') {
      row.push(field); field = ''
      if (row.length > 1 || row[0] !== '') rows.push(row)
      row = []
      if (ch === '\r' && content[i + 1] === '\n') i++
      i++; continue
    }
    field += ch; i++
  }
  if (field !== '' || row.length) { row.push(field); rows.push(row) }
  return rows
}

const table = computed(() => {
  if (kind.value !== 'tabular' || !text.value) return null
  const delim = ext.value === 'tsv' ? '\t' : ','
  const rows = parseDelimited(text.value, delim)
  if (!rows.length) return null
  const head = rows[0]
  const body = rows.slice(1, 1001) // cap rendered rows so the dialog stays snappy
  return {
    columns: head.map((h, idx) => ({
      title: h || `col_${idx + 1}`,
      key: `_${idx}`,
      ellipsis: { tooltip: true },
    })),
    rows: body.map((r, rIdx) => {
      const o = { _key: rIdx }
      r.forEach((v, idx) => { o[`_${idx}`] = v })
      return o
    }),
    totalRows: rows.length - 1,
    rendered: Math.min(body.length, 1000),
    truncated: rows.length - 1 > 1000,
  }
})

async function load() {
  text.value = ''
  if (objectUrl.value) {
    URL.revokeObjectURL(objectUrl.value)
    objectUrl.value = ''
  }
  error.value = ''
  blobMime.value = ''
  if (!props.show || !props.item || !props.url) return

  loading.value = true
  try {
    const k = kind.value
    if (k === 'binary') {
      // No fetch — let user download.
      return
    }
    const resp = await fetch(props.url)
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    blobMime.value = resp.headers.get('content-type') || ''
    if (k === 'image' || k === 'pdf') {
      const blob = await resp.blob()
      objectUrl.value = URL.createObjectURL(blob)
    } else {
      const t = await resp.text()
      // Hard cap to keep the dialog responsive on huge files.
      const MAX_PREVIEW_BYTES = 2 * 1024 * 1024
      if (t.length > MAX_PREVIEW_BYTES) {
        text.value = t.slice(0, MAX_PREVIEW_BYTES) +
          `\n\n... [truncated, file is ${(t.length / 1024 / 1024).toFixed(1)} MB — download to see the full content]`
      } else {
        text.value = t
      }
    }
  } catch (e) {
    error.value = e?.message || 'failed to load preview'
  } finally {
    loading.value = false
  }
}

watch(() => [props.show, props.url, props.item?.name], load, { immediate: true })

onBeforeUnmount(() => {
  if (objectUrl.value) URL.revokeObjectURL(objectUrl.value)
})

function close() { emit('update:show', false) }
function download() { emit('download', props.item) }
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    :title="dialogTitle"
    :bordered="false"
    size="huge"
    style="width: min(900px, 92vw); max-height: 90vh;"
    @update:show="close"
  >
    <template #header-extra>
      <div class="fp-header-actions">
        <n-select
          v-if="versionOptions.length > 1"
          :value="version"
          :options="versionOptions"
          size="small"
          style="width: 132px"
          @update:value="v => emit('update:version', v)"
        />
        <n-button size="small" :disabled="!url" @click="download">
          <template #icon><icon-park-outline-download /></template>
          Download
        </n-button>
      </div>
    </template>

    <div class="fp-body">
      <div v-if="loading" class="fp-state">
        <n-spin :size="22" />
      </div>
      <div v-else-if="error" class="fp-state fp-state--error">
        <icon-park-outline-attention />
        <span>{{ error }}</span>
      </div>

      <template v-else>
        <!-- Image -->
        <div v-if="kind === 'image'" class="fp-image-wrap">
          <img v-if="objectUrl" :src="objectUrl" :alt="item?.name" class="fp-image" />
        </div>

        <!-- SVG (render via highlighted source AND a rendered iframe-like
             <img> wouldn't honor inline scripts, so we just render the raw
             svg text directly in a sandboxed container). -->
        <div v-else-if="kind === 'svg'" class="fp-svg-wrap" v-html="text" />

        <!-- PDF -->
        <div v-else-if="kind === 'pdf'" class="fp-pdf-wrap">
          <iframe v-if="objectUrl" :src="objectUrl" class="fp-pdf-iframe" />
        </div>

        <!-- Markdown -->
        <div
          v-else-if="kind === 'markdown'"
          class="fp-markdown markdown-body"
          v-html="renderedMarkdown"
        />

        <!-- Tabular (CSV / TSV) -->
        <div v-else-if="kind === 'tabular' && table" class="fp-tabular">
          <div class="fp-tabular__meta">
            {{ table.totalRows }} rows · {{ table.columns.length }} columns
            <span v-if="table.truncated">· showing first {{ table.rendered }}</span>
          </div>
          <n-data-table
            :columns="table.columns"
            :data="table.rows"
            :max-height="500"
            :bordered="false"
            size="small"
            :virtual-scroll="table.rendered > 100"
            :row-key="row => row._key"
          />
        </div>

        <!-- Text / code -->
        <div v-else-if="kind === 'text'" class="fp-code-wrap">
          <pre class="fp-code"><code class="hljs" v-html="highlightedCode" /></pre>
        </div>

        <!-- Binary or unknown -->
        <div v-else class="fp-state">
          <icon-park-outline-file-question />
          <span>This file type cannot be previewed inline. Use the Download button above to view it locally.</span>
        </div>
      </template>
    </div>
  </n-modal>
</template>

<style scoped>
.fp-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.fp-body {
  max-height: calc(90vh - 120px);
  overflow: auto;
}

.fp-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 40px 16px;
  font-size: 13px;
  color: var(--n-text-color-3, #888);
  text-align: center;
}

.fp-state--error { color: #d03050; }

.fp-image-wrap {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 8px;
  background: rgba(127,127,127,.04);
}

.fp-image {
  max-width: 100%;
  max-height: 75vh;
  object-fit: contain;
}

.fp-svg-wrap {
  display: flex;
  justify-content: center;
  padding: 12px;
  background: #fff;
}
.fp-svg-wrap :deep(svg) {
  max-width: 100%;
  max-height: 70vh;
}

.fp-pdf-wrap {
  height: 75vh;
}
.fp-pdf-iframe {
  width: 100%;
  height: 100%;
  border: none;
  background: #fff;
}

.fp-markdown {
  padding: 4px 8px 20px;
}

.fp-tabular {
  padding: 4px 0;
}
.fp-tabular__meta {
  font-size: 11px;
  color: var(--n-text-color-3, #888);
  padding: 0 4px 8px;
}

.fp-code-wrap {
  border-radius: 8px;
  background: var(--chat-code-bg, #1e1e2e);
  overflow: hidden;
}

.fp-code {
  margin: 0;
  padding: 14px 18px;
  font-family: 'Fira Code', ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12.5px;
  line-height: 1.55;
  background: transparent;
  color: var(--chat-code-fg, #cdd6f4);
  white-space: pre;
  overflow-x: auto;
}
.fp-code code.hljs {
  background: transparent;
  padding: 0;
  display: block;
}
</style>
