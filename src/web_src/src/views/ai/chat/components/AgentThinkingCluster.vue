<script setup>
import { computed, ref } from 'vue'
import ToolCallCard from './ToolCallCard.vue'
import ToolResultCard from './ToolResultCard.vue'
import CodeExecCard from './CodeExecCard.vue'
import CodeResultCard from './CodeResultCard.vue'
import SkillLoadedPill from './SkillLoadedPill.vue'

const props = defineProps({
  // A list of consecutive detail events (tool_call, tool_result,
  // code_exec, code_result, skill_loaded) that ran between two pieces
  // of user-visible output. Collapsed by default — claude.ai-style —
  // so the chat focuses on the model's prose and the final artifacts,
  // not the plumbing.
  steps: { type: Array, required: true },
  // True while the agent is still producing events for this cluster
  // (the turn hasn't reached the assistant's final reply yet).
  active: { type: Boolean, default: false },
})

const expanded = ref(false)

const summary = computed(() => {
  const n = props.steps.length
  if (props.active && n === 0) return 'Working…'
  if (props.active) return `Working… (${n} step${n === 1 ? '' : 's'})`
  return `${n} step${n === 1 ? '' : 's'}`
})

// The most recently active step gives the user a hint about what the
// agent is doing right now while the cluster is collapsed (similar to
// claude.ai's "Searching the web…" / "Running code…" label).
const lastLabel = computed(() => {
  const last = props.steps[props.steps.length - 1]
  if (!last) return ''
  switch (last.role) {
    case 'tool_call':    return `Calling ${last.tool_name}`
    case 'tool_result':  return `Got result from ${last.tool_name}`
    case 'code_exec':    return `Executing ${last.language || 'python'}`
    case 'code_result':  return 'Code finished'
    case 'skill_loaded': return `Loading skill ${last.name}`
    default:             return ''
  }
})

function componentFor(role) {
  switch (role) {
    case 'tool_call':    return ToolCallCard
    case 'tool_result':  return ToolResultCard
    case 'code_exec':    return CodeExecCard
    case 'code_result':  return CodeResultCard
    case 'skill_loaded': return SkillLoadedPill
    default:             return null
  }
}
</script>

<template>
  <div class="thinking-cluster" :class="{ 'thinking-cluster--active': active }">
    <button
      class="thinking-cluster__header"
      :aria-expanded="expanded"
      @click="expanded = !expanded"
    >
      <span class="thinking-cluster__icon">
        <icon-park-outline-thinking-problem v-if="!active" />
        <n-spin v-else :size="12" />
      </span>
      <span class="thinking-cluster__title">{{ summary }}</span>
      <span v-if="lastLabel && !expanded" class="thinking-cluster__hint">
        · {{ lastLabel }}
      </span>
      <span class="thinking-cluster__spacer" />
      <span class="thinking-cluster__toggle">
        {{ expanded ? 'Hide details' : 'Show details' }}
        <icon-park-outline-down
          class="thinking-cluster__chev"
          :class="{ 'thinking-cluster__chev--up': expanded }"
        />
      </span>
    </button>

    <div v-if="expanded" class="thinking-cluster__body">
      <component
        :is="componentFor(step.role)"
        v-for="step in steps"
        :key="step.id"
        :message="step"
      />
    </div>
  </div>
</template>

<style scoped>
.thinking-cluster {
  border: 1px dashed var(--n-border-color, rgba(0,0,0,.12));
  border-radius: 10px;
  background: var(--n-color-embedded, rgba(0,0,0,.02));
  margin: 4px 0;
  overflow: hidden;
}

.thinking-cluster--active {
  border-style: solid;
  border-color: color-mix(in srgb, var(--chat-primary, #18a058) 35%, transparent);
  background: color-mix(in srgb, var(--chat-primary, #18a058) 4%, transparent);
}

.thinking-cluster__header {
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

.thinking-cluster__header:hover {
  background: var(--n-color-embedded, rgba(0,0,0,.03));
}

.thinking-cluster__icon {
  flex-shrink: 0;
  font-size: 14px;
  display: inline-flex;
  align-items: center;
}

.thinking-cluster__title {
  font-weight: 500;
}

.thinking-cluster__hint {
  font-size: 12px;
  color: var(--n-text-color-3, #888);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex-shrink: 1;
  min-width: 0;
}

.thinking-cluster__spacer { flex: 1; }

.thinking-cluster__toggle {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--n-text-color-3, #888);
  flex-shrink: 0;
}

.thinking-cluster__chev {
  font-size: 12px;
  transition: transform 0.15s;
}
.thinking-cluster__chev--up { transform: rotate(180deg); }

.thinking-cluster__body {
  padding: 4px 12px 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  border-top: 1px solid var(--n-border-color, rgba(0,0,0,.06));
}
</style>
