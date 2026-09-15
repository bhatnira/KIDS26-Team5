<script setup lang="ts">
import { useAppStore } from '@/stores'

const router = useRouter()
const appStore = useAppStore()

const name = import.meta.env.VITE_APP_NAME

const hidenLogoText = computed(() => {
  if (['sidebar', 'mixed-sidebar', 'horizontal'].includes(appStore.layoutMode)) {
    return false
  }
  if (['two-column', 'mixed-two-column'].includes(appStore.layoutMode)) {
    return true
  }
  return appStore.collapsed
})
</script>

<template>
  <div
    class="h-60px text-xl flex-center cursor-pointer p-x-2"
    @click="router.push('/')"
  >
    <div class="flex items-center gap-2">
      <svg-icons-logo class="text-1.5em block" style="vertical-align: middle;" />
      <span
        v-show="!hidenLogoText"
        class="text-ellipsis overflow-hidden whitespace-nowrap leading-none"
      >{{ name }}</span>
    </div>
  </div>
</template>

<style scoped></style>
