<script setup>
import { computed } from 'vue'
import AgentEventCard from './AgentEventCard.vue'

const props = defineProps({
  message: { type: Object, required: true },
})

const hasStdout = computed(() => !!(props.message.stdout || '').trim())
const hasStderr = computed(() => !!(props.message.stderr || '').trim())
const hasFiles = computed(() => (props.message.files || []).length > 0)
const tone = computed(() => (hasStderr.value || props.message.exit_code > 0) ? 'warning' : 'success')

const subtitle = computed(() => {
  if (props.message.exit_code != null && props.message.exit_code !== 0) {
    return `exit ${props.message.exit_code}`
  }
  return hasFiles.value ? `${(props.message.files || []).length} file(s)` : ''
})
</script>

<template>
  <AgentEventCard
    :tone="tone"
    icon="icon-park-outline-terminal"
    title="Code result"
    :subtitle="subtitle"
    :collapsible="true"
    :initially-collapsed="false"
  >
    <div v-if="hasStdout" class="code-result__section">
      <div class="code-result__label">stdout</div>
      <pre class="code-result__pre">{{ message.stdout }}</pre>
    </div>
    <div v-if="hasStderr" class="code-result__section">
      <div class="code-result__label code-result__label--err">stderr</div>
      <pre class="code-result__pre code-result__pre--err">{{ message.stderr }}</pre>
    </div>
    <div v-if="hasFiles" class="code-result__section">
      <div class="code-result__label">output files</div>
      <ul class="code-result__files">
        <li v-for="f in message.files" :key="f.name">
          <icon-park-outline-file-code />
          <span>{{ f.name }}</span>
          <span v-if="f.size_bytes" class="code-result__size">({{ f.size_bytes }} B)</span>
        </li>
      </ul>
    </div>
    <div v-if="!hasStdout && !hasStderr && !hasFiles" class="code-result__empty">
      (no output)
    </div>
  </AgentEventCard>
</template>

<style scoped>
.code-result__section { margin: 4px 0 8px; }
.code-result__section:last-child { margin-bottom: 0; }

.code-result__label {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--n-text-color-3, #888);
  margin: 2px 0 4px;
}

.code-result__label--err { color: #d03050; }

.code-result__pre {
  font-family: 'Fira Code', monospace;
  font-size: 12px;
  background: rgba(127,127,127,.07);
  padding: 8px 10px;
  border-radius: 6px;
  white-space: pre-wrap;
  word-break: break-word;
  margin: 0;
  max-height: 240px;
  overflow: auto;
}

.code-result__pre--err {
  background: rgba(208, 48, 80, 0.07);
  color: #d03050;
}

.code-result__files {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.code-result__files li {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-family: monospace;
}

.code-result__size {
  color: var(--n-text-color-3);
}

.code-result__empty {
  font-size: 12px;
  color: var(--n-text-color-3);
  font-style: italic;
}
</style>
