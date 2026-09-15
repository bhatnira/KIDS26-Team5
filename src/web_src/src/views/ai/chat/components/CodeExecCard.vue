<script setup>
import { computed } from 'vue'
import hljs from 'highlight.js'
import AgentEventCard from './AgentEventCard.vue'

const props = defineProps({
  message: { type: Object, required: true },
})

const highlighted = computed(() => {
  const code = props.message.code || ''
  const lang = props.message.language || 'python'
  if (!code) return ''
  if (hljs.getLanguage(lang)) {
    try {
      return hljs.highlight(code, { language: lang, ignoreIllegals: true }).value
    } catch { /* fallthrough */ }
  }
  return hljs.highlightAuto(code).value
})
</script>

<template>
  <AgentEventCard
    tone="info"
    icon="icon-park-outline-code"
    :title="`Executing ${message.language}`"
    :collapsible="true"
    :initially-collapsed="false"
  >
    <pre class="code-exec__pre"><code class="hljs" v-html="highlighted" /></pre>
  </AgentEventCard>
</template>

<style scoped>
.code-exec__pre {
  margin: 0;
  background: var(--chat-code-bg, #1e1e2e);
  border: 1px solid var(--n-border-color, rgba(0,0,0,0.06));
  border-radius: 6px;
  overflow-x: auto;
}

.code-exec__pre code.hljs {
  display: block;
  padding: 10px 14px;
  font-family: 'Fira Code', monospace;
  font-size: 12.5px;
  line-height: 1.5;
  background: transparent;
  color: var(--chat-code-fg, #cdd6f4);
}
</style>
