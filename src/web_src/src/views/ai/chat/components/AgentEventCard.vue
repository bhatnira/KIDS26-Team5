<script setup>
import { ref } from 'vue'

defineProps({
  // Visual tone: 'neutral' | 'info' | 'success' | 'warning' | 'error'
  tone: { type: String, default: 'neutral' },
  // Optional iconify-style icon class (e.g. 'icon-park-outline-tool')
  icon: { type: String, default: '' },
  title: { type: String, required: true },
  subtitle: { type: String, default: '' },
  // Whether the body should start collapsed.
  collapsible: { type: Boolean, default: false },
  initiallyCollapsed: { type: Boolean, default: true },
})

const collapsed = ref(true)

function toggle() {
  collapsed.value = !collapsed.value
}
</script>

<template>
  <div class="agent-card" :class="`agent-card--${tone}`">
    <button
      class="agent-card__header"
      :class="{ 'agent-card__header--clickable': collapsible }"
      :disabled="!collapsible"
      @click="toggle"
    >
      <span v-if="icon" class="agent-card__icon">
        <component :is="icon" />
      </span>
      <span class="agent-card__title">{{ title }}</span>
      <span v-if="subtitle" class="agent-card__subtitle">{{ subtitle }}</span>
      <span class="agent-card__spacer" />
      <icon-park-outline-up
        v-if="collapsible"
        class="agent-card__chevron"
        :class="{ 'agent-card__chevron--down': initiallyCollapsed ? collapsed : !collapsed }"
      />
    </button>
    <div
      v-if="!collapsible || (initiallyCollapsed ? !collapsed : collapsed)"
      class="agent-card__body"
    >
      <slot />
    </div>
  </div>
</template>

<style scoped>
.agent-card {
  border: 1px solid var(--n-border-color, rgba(0,0,0,.1));
  border-radius: 10px;
  background: var(--n-color, #fff);
  font-size: 13px;
  overflow: hidden;
  margin: 2px 0;
}

.agent-card--info    { border-left: 3px solid #2080f0; }
.agent-card--success { border-left: 3px solid #18a058; }
.agent-card--warning { border-left: 3px solid #f0a020; }
.agent-card--error   { border-left: 3px solid #d03050; }

.agent-card__header {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 8px 12px;
  background: transparent;
  border: none;
  text-align: left;
  font: inherit;
  color: inherit;
  cursor: default;
}

.agent-card__header--clickable { cursor: pointer; }
.agent-card__header--clickable:hover {
  background: var(--n-color-embedded, rgba(0,0,0,.03));
}

.agent-card__icon {
  flex-shrink: 0;
  font-size: 14px;
  color: var(--n-text-color-2, #555);
  display: inline-flex;
  align-items: center;
}

.agent-card__title {
  font-weight: 600;
  font-size: 13px;
}

.agent-card__subtitle {
  font-size: 12px;
  color: var(--n-text-color-3, #888);
}

.agent-card__spacer { flex: 1; }

.agent-card__chevron {
  font-size: 12px;
  color: var(--n-text-color-3, #aaa);
  transition: transform 0.15s;
}

.agent-card__chevron--down {
  transform: rotate(180deg);
}

.agent-card__body {
  padding: 8px 12px 10px;
  border-top: 1px solid var(--n-border-color, rgba(0,0,0,.06));
}
</style>
