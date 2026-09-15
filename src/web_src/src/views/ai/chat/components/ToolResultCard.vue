<script setup>
import { computed } from 'vue'
import AgentEventCard from './AgentEventCard.vue'

const props = defineProps({
  message: { type: Object, required: true },
})

const pretty = computed(() => {
  const raw = props.message.content
  if (!raw) return ''
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
})

const preview = computed(() => {
  // Short single-line summary for the collapsed header.
  const s = (props.message.content || '').replace(/\s+/g, ' ').trim()
  return s.length > 60 ? s.slice(0, 60) + '…' : s
})
</script>

<template>
  <AgentEventCard
    tone="success"
    icon="icon-park-outline-check"
    :title="`Result from ${message.tool_name}`"
    :subtitle="preview"
    :collapsible="!!pretty"
    :initially-collapsed="true"
  >
    <pre class="tool-result__content">{{ pretty }}</pre>
  </AgentEventCard>
</template>

<style scoped>
.tool-result__content {
  font-family: 'Fira Code', monospace;
  font-size: 12px;
  background: rgba(127,127,127,.07);
  padding: 8px 10px;
  border-radius: 6px;
  white-space: pre-wrap;
  word-break: break-word;
  margin: 0;
  max-height: 320px;
  overflow: auto;
  color: var(--n-text-color-2);
}
</style>
