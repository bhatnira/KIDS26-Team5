<script setup>

import { installRouter } from '@/router'
import { installPinia } from '@/stores'
import { naiveI18nOptions } from '@/utils/i18n.js'
import { darkTheme } from 'naive-ui'
import { useAppStore } from '@/stores'
import { computed, getCurrentInstance } from 'vue'

const initializationPromise = (async () => {
  // get app instance
  const app = getCurrentInstance()?.appContext.app
  if (!app) {
    throw new Error('App not found')
  }

  // register Pinia
  await installPinia(app)

  // register Vue-router
  await installRouter(app)

  // register module
  const modules = import.meta.glob('./modules/*.js', {
    eager: true
  });
  Object.values(modules).forEach(module => app.use(module))

  return true
})()

// await init
await initializationPromise

const appStore = useAppStore()

const naiveLocale = computed(() => {
  return naiveI18nOptions[appStore.lang] ? naiveI18nOptions[appStore.lang] : naiveI18nOptions.enUS
})

</script>

<template>
    <n-config-provider
      class="wh-full"
      inline-theme-disabled
      :theme="appStore.colorMode === 'dark' ? darkTheme : null"
      :locale="naiveLocale.locale"
      :date-locale="naiveLocale.dateLocale"
      :theme-overrides="appStore.theme"
    >
      <naive-provider>
        <router-view />
        <Watermark :show-watermark="appStore.showWatermark" />
      </naive-provider>
    </n-config-provider>
</template>

