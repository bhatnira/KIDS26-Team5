<script setup>
import { Icon } from '@iconify/vue'

const props = defineProps({
  /* 图标名称 */
  icon: String,
  /* 图标颜色 */
  color: String,
  /* 图标大小 */
  size: {
    type: Number,
    default: 18
  },
  /* 图标深度 */
  depth: {
    type: Number,
    validator: (value) => [1, 2, 3, 4, 5].includes(value)
  }
})

const { size, icon } = props

const isLocal = computed(() => {
  return icon && icon.startsWith('local:')
})

function getLocalIcon(icon) {
  const svgName = icon.replace('local:', '')
  const svg = import.meta.glob('@/assets/svg-icons/*.svg', {
    query: '?raw',
    import: 'default',
    eager: true,
  })

  return svg[`/src/assets/svg-icons/${svgName}.svg`]
}
</script>

<template>
  <n-icon
    v-if="icon"
    :size="size"
    :depth="depth"
    :color="color"
  >
    <template v-if="isLocal">
      <i v-html="getLocalIcon(icon)" />
    </template>
    <template v-else>
      <Icon :icon="icon" />
    </template>
  </n-icon>
</template>
