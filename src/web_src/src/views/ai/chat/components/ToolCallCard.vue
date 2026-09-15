<script setup>
import { computed, ref } from 'vue'
import hljs from 'highlight.js'
import AgentEventCard from './AgentEventCard.vue'

const props = defineProps({
  message: { type: Object, required: true },
})

// Tool argument shapes vary, but a handful of well-known fields carry the
// "interesting" payload (a shell command, a SQL query, a Python snippet,
// the file path that's being read). When we find one, render it in a
// syntax-highlighted code block so the user can see and copy the actual
// command rather than wade through the JSON envelope.
const COMMAND_FIELDS = [
  { key: 'command', lang: 'bash' },
  { key: 'cmd', lang: 'bash' },
  { key: 'script', lang: 'bash' },
  { key: 'shell', lang: 'bash' },
  { key: 'bash', lang: 'bash' },
  { key: 'code', lang: 'python' },
  { key: 'python', lang: 'python' },
  { key: 'source', lang: 'python' },
  { key: 'query', lang: 'sql' },
  { key: 'sql', lang: 'sql' },
  { key: 'path', lang: '' },
  { key: 'file_path', lang: '' },
]

const parsedArgs = computed(() => {
  const raw = props.message.arguments
  if (!raw) return null
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
})

const prettyJSON = computed(() => {
  if (parsedArgs.value !== null) {
    return JSON.stringify(parsedArgs.value, null, 2)
  }
  return props.message.arguments || ''
})

// commandBlock = { value, lang } when the args expose a recognisable
// command field. The rest of the args (without that field) is rendered
// alongside as JSON so we don't hide any input parameters.
const commandBlock = computed(() => {
  const args = parsedArgs.value
  if (!args || typeof args !== 'object') return null
  for (const spec of COMMAND_FIELDS) {
    const v = args[spec.key]
    if (typeof v === 'string' && v.trim()) {
      const lang = inferLanguage(props.message.tool_name, spec) || spec.lang
      return { value: v, lang, fieldKey: spec.key }
    }
  }
  return null
})

const otherArgsJSON = computed(() => {
  if (!commandBlock.value) return ''
  const rest = { ...parsedArgs.value }
  delete rest[commandBlock.value.fieldKey]
  if (!Object.keys(rest).length) return ''
  return JSON.stringify(rest, null, 2)
})

function inferLanguage(toolName, spec) {
  // Light heuristics — explicit field lang wins, then tool name hints.
  if (spec.lang) return spec.lang
  const n = (toolName || '').toLowerCase()
  if (n.includes('python')) return 'python'
  if (n.includes('sql')) return 'sql'
  if (n.includes('exec') || n.includes('shell') || n.includes('bash')) return 'bash'
  return ''
}

const highlightedCommand = computed(() => {
  const block = commandBlock.value
  if (!block) return ''
  const lang = block.lang
  if (lang && hljs.getLanguage(lang)) {
    try {
      return hljs.highlight(block.value, { language: lang, ignoreIllegals: true }).value
    } catch { /* fall through */ }
  }
  return hljs.highlightAuto(block.value).value
})

const copied = ref(false)
let copyTimer = null
async function copyCommand() {
  const text = commandBlock.value?.value || ''
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    const el = document.createElement('textarea')
    el.value = text
    document.body.appendChild(el)
    el.select()
    document.execCommand('copy')
    document.body.removeChild(el)
  }
  copied.value = true
  clearTimeout(copyTimer)
  copyTimer = setTimeout(() => { copied.value = false }, 1500)
}

const hasBody = computed(() => !!commandBlock.value || !!prettyJSON.value)
</script>

<template>
  <AgentEventCard
    tone="info"
    icon="icon-park-outline-tool"
    :title="`Calling ${message.tool_name}`"
    :subtitle="message.tool_id"
    :collapsible="hasBody"
    :initially-collapsed="true"
  >
    <!-- Command-style payload: highlighted code block with copy button -->
    <template v-if="commandBlock">
      <div class="tool-call__code-wrap">
        <div class="tool-call__code-header">
          <span class="tool-call__lang">{{ commandBlock.lang || 'text' }}</span>
          <button class="tool-call__copy-btn" @click="copyCommand">
            <icon-park-outline-copy v-if="!copied" />
            <icon-park-outline-check v-else />
            <span>{{ copied ? 'Copied' : 'Copy' }}</span>
          </button>
        </div>
        <pre class="tool-call__code"><code class="hljs" v-html="highlightedCommand" /></pre>
      </div>
      <details v-if="otherArgsJSON" class="tool-call__more">
        <summary>Other arguments</summary>
        <pre class="tool-call__args">{{ otherArgsJSON }}</pre>
      </details>
    </template>

    <!-- Generic JSON args fallback -->
    <pre v-else class="tool-call__args">{{ prettyJSON }}</pre>
  </AgentEventCard>
</template>

<style scoped>
.tool-call__code-wrap {
  border-radius: 6px;
  overflow: hidden;
  background: var(--chat-code-bg, #1e1e2e);
  border: 1px solid var(--n-border-color, rgba(0,0,0,0.06));
}

.tool-call__code-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 10px;
  font-size: 11px;
  font-family: 'Fira Code', monospace;
  color: rgba(205, 214, 244, 0.6);
  background: rgba(255, 255, 255, 0.04);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.tool-call__lang {
  text-transform: lowercase;
  letter-spacing: 0.04em;
}

.tool-call__copy-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: transparent;
  border: none;
  color: inherit;
  font: inherit;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 4px;
}
.tool-call__copy-btn:hover {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(205, 214, 244, 0.9);
}

.tool-call__code {
  margin: 0;
  padding: 10px 14px;
  font-family: 'Fira Code', monospace;
  font-size: 12.5px;
  line-height: 1.5;
  background: transparent;
  color: var(--chat-code-fg, #cdd6f4);
  white-space: pre;
  overflow-x: auto;
}

.tool-call__more {
  margin-top: 8px;
  font-size: 12px;
  color: var(--n-text-color-3, #888);
}
.tool-call__more summary {
  cursor: pointer;
  padding: 2px 0;
  user-select: none;
}

.tool-call__args {
  font-family: 'Fira Code', monospace;
  font-size: 12px;
  background: rgba(127,127,127,.07);
  padding: 8px 10px;
  border-radius: 6px;
  white-space: pre-wrap;
  word-break: break-word;
  margin: 6px 0 0;
  color: var(--n-text-color-2);
}
</style>
