<script setup>
import { computed, ref } from 'vue'
import { useAppStore } from '@/stores'
import MarkdownStream from './MarkdownStream.vue'
import ToolCallCard from './ToolCallCard.vue'
import ToolResultCard from './ToolResultCard.vue'
import CodeExecCard from './CodeExecCard.vue'
import CodeResultCard from './CodeResultCard.vue'
import ArtifactCard from './ArtifactCard.vue'
import SkillLoadedPill from './SkillLoadedPill.vue'
import AgentThinkingCluster from './AgentThinkingCluster.vue'
import ReasoningMessage from './ReasoningMessage.vue'

const props = defineProps({
  message: {
    type: Object,
    required: true,
  },
})

const role = computed(() => props.message.role)
const isUser = computed(() => role.value === 'user')
const isAssistant = computed(() => role.value === 'assistant')
const isAgentEvent = computed(() =>
  ['tool_call', 'tool_result', 'code_exec', 'code_result', 'artifact', 'skill_loaded', 'error', 'thinking_cluster', 'reasoning'].includes(role.value)
)

const formattedTime = computed(() => {
  if (!props.message.created_at) return ''
  return new Date(props.message.created_at).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })
})

const appStore = useAppStore()
const primaryColor = computed(() => appStore.primaryColor || '#18a058')

const isHovered = ref(false)
const copied = ref(false)
let copyTimer = null

async function copyMessage() {
  const text = props.message.content || ''
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
  copyTimer = setTimeout(() => { copied.value = false }, 2000)
}

</script>

<template>
  <!-- ── Agent event message: no avatar, no bubble; card occupies the row ── -->
  <div v-if="isAgentEvent" class="agent-event-row">
    <AgentThinkingCluster
      v-if="role === 'thinking_cluster'"
      :steps="message.steps"
      :active="!!message.active"
    />
    <ReasoningMessage
      v-else-if="role === 'reasoning'"
      :content="message.content"
      :streaming="!!message.isStreaming"
    />
    <ToolCallCard v-else-if="role === 'tool_call'" :message="message" />
    <ToolResultCard v-else-if="role === 'tool_result'" :message="message" />
    <CodeExecCard v-else-if="role === 'code_exec'" :message="message" />
    <CodeResultCard v-else-if="role === 'code_result'" :message="message" />
    <ArtifactCard v-else-if="role === 'artifact'" :message="message" />
    <SkillLoadedPill v-else-if="role === 'skill_loaded'" :message="message" />
    <div v-else-if="role === 'error'" class="error-row">
      <icon-park-outline-attention />
      <span>{{ message.message || 'error' }}</span>
    </div>
  </div>

  <!-- ── User chat bubble (right-aligned bubble) ─────────────────────────── -->
  <div
    v-else-if="isUser"
    class="message-bubble-wrapper message-bubble-wrapper--user"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div class="message-avatar message-avatar--user">
      <n-avatar round :size="32" color="#2080f0">
        <icon-park-outline-user />
      </n-avatar>
    </div>

    <div class="message-content message-content--user">
      <div class="bubble bubble--user">
        <span class="bubble__text">{{ message.content }}</span>
        <div v-if="message.attachments?.length" class="attachments">
          <div
            v-for="a in message.attachments"
            :key="a.key"
            class="attachment-chip"
          >
            <icon-park-outline-file-code />
            <span>{{ a.name }}</span>
          </div>
        </div>
      </div>

      <div class="message-footer message-footer--user">
        <div class="message-time">{{ formattedTime }}</div>
        <div class="copy-btn-wrap" :class="{ 'copy-btn-wrap--visible': isHovered }">
          <button class="copy-btn" @click="copyMessage">
            <span v-if="copied" class="copy-check">✓</span>
            <icon-park-outline-copy v-else class="copy-icon" />
          </button>
          <span class="copy-label">{{ copied ? 'Copied!' : 'Copy' }}</span>
        </div>
      </div>
    </div>
  </div>

  <!-- ── Assistant message (full-width, no bubble — claude.ai pattern) ─── -->
  <div
    v-else-if="isAssistant"
    class="message-asst"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div class="message-asst__avatar">
      <n-avatar round :size="32" :color="primaryColor">
        <icon-park-outline-robot />
      </n-avatar>
    </div>
    <div class="message-asst__body">
      <MarkdownStream
        v-if="message.content"
        :content="message.content"
        :streaming="!!message.isStreaming"
      />
      <n-spin v-else-if="message.isStreaming && !message.content" :size="16" />
      <div class="message-footer">
        <div class="message-time">{{ formattedTime }}</div>
        <div class="copy-btn-wrap copy-btn-wrap--visible">
          <button class="copy-btn" @click="copyMessage">
            <span v-if="copied" class="copy-check">✓</span>
            <icon-park-outline-copy v-else class="copy-icon" />
          </button>
          <span class="copy-label">{{ copied ? 'Copied!' : 'Copy' }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.message-bubble-wrapper {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 6px 0;
  max-width: 100%;
}

.message-bubble-wrapper--user { flex-direction: row-reverse; }

.message-avatar { flex-shrink: 0; margin-top: 2px; }

.message-content {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-width: 75%;
}

.message-content--user { align-items: flex-end; }

.bubble {
  border-radius: 12px;
  padding: 10px 14px;
  word-break: break-word;
  line-height: 1.6;
}

.bubble--user {
  background: var(--n-primary-color, #2080f0);
  color: white;
  border-bottom-right-radius: 4px;
}

.bubble__text { white-space: pre-wrap; }

.attachments {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 6px;
}

.attachment-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 999px;
  background: rgba(255,255,255,.18);
  color: inherit;
}

/* ── Assistant message (full width, no bubble) ─────────────────────────── */
.message-asst {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 10px 0 6px;
  width: 100%;
}

.message-asst__avatar {
  flex-shrink: 0;
  margin-top: 2px;
}

.message-asst__body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

/* ── Footer / copy button (shared) ─────────────────────────────────────── */
.message-footer {
  display: flex;
  align-items: center;
  gap: 4px;
  min-height: 22px;
}

.message-footer--user { flex-direction: row-reverse; }

.message-time {
  font-size: 11px;
  color: var(--n-text-color-3, rgba(0, 0, 0, 0.4));
  padding: 0 4px;
  flex-shrink: 0;
}

/* ── Agent event row ─────────────────────────────────────────── */
.agent-event-row {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  padding: 4px 0;
}

.error-row {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border: 1px solid rgba(208, 48, 80, 0.3);
  border-left-width: 3px;
  border-radius: 8px;
  background: rgba(208, 48, 80, 0.05);
  color: #d03050;
  font-size: 13px;
  width: fit-content;
}

/* ── Copy button ──────────────────────────────────────────────── */
.copy-btn-wrap {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.15s;
}

.copy-btn-wrap--visible {
  opacity: 1;
  pointer-events: auto;
}

.copy-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: none;
  background: transparent;
  border-radius: 6px;
  color: var(--n-text-color-3, #999);
  font-size: 14px;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
  padding: 0;
}

.copy-btn:hover {
  background: var(--n-color-embedded, rgba(0, 0, 0, 0.07));
  color: var(--n-text-color, #333);
}

.copy-label {
  position: absolute;
  top: calc(100% + 2px);
  left: 50%;
  transform: translateX(-50%);
  font-size: 10px;
  color: var(--n-text-color-3, #aaa);
  line-height: 1;
  white-space: nowrap;
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.1s;
  z-index: 10;
}

.copy-btn-wrap:hover .copy-label { opacity: 1; }

.copy-icon { font-size: 14px; }

.copy-check {
  font-size: 14px;
  color: v-bind(primaryColor);
  font-weight: 600;
  line-height: 1;
}
</style>

<style>
/* ── Global markdown rendering rules (unscoped so v-html children pick
     them up). ─────────────────────────────────────────────────────────── */
.markdown-body {
  font-size: 14px;
  line-height: 1.7;
  color: inherit;
  word-wrap: break-word;
}

.markdown-body > *:first-child { margin-top: 0; }
.markdown-body > *:last-child  { margin-bottom: 0; }

.markdown-body h1,
.markdown-body h2,
.markdown-body h3,
.markdown-body h4 {
  margin: 18px 0 8px;
  font-weight: 600;
  line-height: 1.3;
}

.markdown-body h1 { font-size: 1.45em; }
.markdown-body h2 { font-size: 1.25em; }
.markdown-body h3 { font-size: 1.1em; }
.markdown-body h4 { font-size: 1em; }

.markdown-body p { margin: 8px 0; }

.markdown-body ul,
.markdown-body ol {
  padding-left: 1.6em;
  margin: 8px 0;
}

.markdown-body li { margin: 2px 0; }
.markdown-body li > p { margin: 2px 0; }

/* Inline code only — scoped to NOT match <code> inside streamdown's Shiki
   <pre> code blocks (which carry their own theme/background). */
.markdown-body :not(pre) > code {
  font-family: 'Fira Code', ui-monospace, SFMono-Regular, Menlo, monospace;
  background: rgba(127, 127, 127, 0.12);
  border-radius: 4px;
  padding: 1px 5px;
  font-size: 0.88em;
}

.markdown-body blockquote {
  border-left: 3px solid var(--n-primary-color, #2080f0);
  margin: 10px 0;
  padding: 4px 14px;
  color: var(--n-text-color-2);
  background: rgba(32, 128, 240, 0.05);
  border-radius: 0 6px 6px 0;
}

.markdown-body table {
  border-collapse: collapse;
  margin: 12px 0;
  font-size: 0.92em;
  display: block;
  max-width: 100%;
  overflow-x: auto;
}

.markdown-body th,
.markdown-body td {
  border: 1px solid var(--n-border-color);
  padding: 7px 12px;
  text-align: left;
  vertical-align: top;
}

.markdown-body th {
  background: var(--n-color-embedded);
  font-weight: 600;
}

.markdown-body tbody tr:nth-child(2n) {
  background: color-mix(in srgb, var(--n-color-embedded, rgba(0,0,0,.04)) 50%, transparent);
}

.markdown-body a {
  color: var(--n-primary-color, #2080f0);
  text-decoration: none;
}

.markdown-body a:hover { text-decoration: underline; }

.markdown-body hr {
  border: none;
  border-top: 1px solid var(--n-border-color);
  margin: 16px 0;
}

/* Code blocks are now rendered by streamdown-vue (Shiki) with its own
   styles from streamdown-vue/style.css; no hand-rolled code-block CSS here. */

/* Task-list-ish: if rendered without a plugin, at least don't show the
   bullet for [x] / [ ] items typed in raw markdown. */
.markdown-body input[type="checkbox"] {
  margin-right: 6px;
  vertical-align: middle;
}
</style>
