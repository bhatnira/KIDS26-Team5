<script setup>
defineProps({
  message: { type: Object, required: true },
})
</script>

<template>
  <div class="skill-pill">
    <icon-park-outline-bookmark />
    <span class="skill-pill__name">Loaded skill:</span>
    <span class="skill-pill__value">{{ message.name }}</span>
    <span v-if="message.domain" class="skill-pill__tag">{{ message.domain }}</span>
  </div>
</template>

<style scoped>
.skill-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--n-primary-color, #18a058) 12%, transparent);
  color: var(--n-primary-color, #18a058);
  width: fit-content;
}

.skill-pill__name {
  color: var(--n-text-color-2);
}

.skill-pill__value {
  font-family: monospace;
  font-weight: 600;
}

.skill-pill__tag {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 999px;
  background: var(--n-color-embedded, rgba(0,0,0,.05));
  color: var(--n-text-color-3);
}
</style>
