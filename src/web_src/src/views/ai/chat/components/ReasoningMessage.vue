<script setup>
import { ref, watch } from 'vue'
import MarkdownStream from './MarkdownStream.vue'

// A foldable, muted "thinking" block rendered ahead of the assistant's
// answer (Claude/ChatGPT pattern). It is auto-expanded while the model is
// still streaming its reasoning, and collapses once the reasoning finishes
// so it never competes with the real answer for attention.
const props = defineProps({
  // The accumulated reasoning/thinking text (markdown).
  content: { type: String, default: '' },
  // True while reasoning deltas are still arriving for this block.
  streaming: { type: Boolean, default: false },
})

// Expanded while streaming; auto-collapse the moment streaming ends. On
// reloaded history `streaming` is false from the start, so it opens collapsed.
const expanded = ref(props.streaming)
watch(
  () => props.streaming,
  (now, prev) => {
    if (prev && !now) expanded.value = false
  },
)
</script>

<template>
  <div class="reasoning" :class="{ 'reasoning--active': streaming }">
    <button
      class="reasoning__header"
      :aria-expanded="expanded"
      @click="expanded = !expanded"
    >
      <span class="reasoning__icon">
        <n-spin v-if="streaming" :size="12" />
        <icon-park-outline-thinking-problem v-else />
      </span>
      <span class="reasoning__title">{{ streaming ? 'Thinking…' : 'Thought process' }}</span>
      <span class="reasoning__spacer" />
      <span class="reasoning__toggle">
        {{ expanded ? 'Hide' : 'Show' }}
        <icon-park-outline-down
          class="reasoning__chev"
          :class="{ 'reasoning__chev--up': expanded }"
        />
      </span>
    </button>

    <div v-if="expanded" class="reasoning__body">
      <MarkdownStream :content="content" :streaming="streaming" />
    </div>
  </div>
</template>

<style scoped>
.reasoning {
  border: 1px dashed var(--n-border-color, rgba(0, 0, 0, .12));
  border-radius: 10px;
  background: var(--n-color-embedded, rgba(0, 0, 0, .02));
  margin: 4px 0;
  overflow: hidden;
}

.reasoning--active {
  border-style: solid;
  border-color: color-mix(in srgb, var(--chat-primary, #18a058) 35%, transparent);
  background: color-mix(in srgb, var(--chat-primary, #18a058) 4%, transparent);
}

.reasoning__header {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 8px 14px;
  background: transparent;
  border: none;
  text-align: left;
  font: inherit;
  color: var(--n-text-color-2, #555);
  font-size: 13px;
  cursor: pointer;
}

.reasoning__header:hover {
  background: var(--n-color-embedded, rgba(0, 0, 0, .03));
}

.reasoning__icon {
  flex-shrink: 0;
  font-size: 14px;
  display: inline-flex;
  align-items: center;
}

.reasoning__title {
  font-weight: 500;
}

.reasoning__spacer { flex: 1; }

.reasoning__toggle {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--n-text-color-3, #888);
  flex-shrink: 0;
}

.reasoning__chev {
  font-size: 12px;
  transition: transform 0.15s;
}
.reasoning__chev--up { transform: rotate(180deg); }

/* Muted, smaller body so the thinking never competes with the answer. */
.reasoning__body {
  padding: 4px 14px 12px;
  border-top: 1px solid var(--n-border-color, rgba(0, 0, 0, .06));
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--n-text-color-3, #888);
}

.reasoning__body :deep(p) { margin: 0.4em 0; }
.reasoning__body :deep(p:first-child) { margin-top: 0; }
.reasoning__body :deep(p:last-child) { margin-bottom: 0; }
.reasoning__body :deep(pre) {
  background: var(--n-color-embedded, rgba(0, 0, 0, .04));
  padding: 8px 10px;
  border-radius: 6px;
  overflow-x: auto;
}
.reasoning__body :deep(code) { font-size: 12px; }
</style>
