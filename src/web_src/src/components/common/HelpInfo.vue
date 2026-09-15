<script setup>
const props = defineProps({
  message: {
    type: String,
    required: true
  }
})

const { message } = props
</script>

<template>
  <n-tooltip :show-arrow="false" trigger="hover">
    <template #trigger>
      <icon-park-outline-help class="op-50 cursor-help" />
    </template>
    {{ message }}
  </n-tooltip>
</template>
