<script setup lang="ts">
import { useAppStore, useRouteStore } from '@/stores'

const appStore = useAppStore()
const routeStore = useRouteStore()
</script>

<template>
  <n-el
    class="layout-content"
    :class="[
      appStore.layoutMode === 'full-content' ? 'p-0' : 'p-16px',
    ]"
    style="background-color: var(--action-color);"
  >
    <router-view
      v-slot="{ Component, route }"
    >
      <transition :name="appStore.transitionAnimation" mode="out-in">
        <keep-alive :include="routeStore.cacheRoutes">
          <component :is="Component" v-if="appStore.loadFlag" :key="route.fullPath" />
        </keep-alive>
      </transition>
    </router-view>
  </n-el>
</template>

<!--
  Non-scoped: fixes a Safari/WebKit layout bug.

  The pro-layout content region (<main class="n-pro-layout__content">) gets its
  height from `flex-grow`. Chrome/Firefox resolve `height: 100%` on descendants
  against that flex-grown height, but Safari/DuckDuckGo treat it as indefinite,
  so `height: 100%` on this element (and every page below it) collapses to the
  content height. That left the grey content background stopping short — showing
  the white `bodyColor` under it as a large gap above the footer — and stopped
  full-height pages (e.g. AI chat) from centering.

  Fix: make the content region a flex row and let this element fill it via flex
  + stretch instead of a percentage height. A *stretched* flex item has a
  definite cross-size, which WebKit resolves descendants' `height: 100%` against.
-->
<style>
.n-pro-layout__content {
  display: flex;
}

.n-pro-layout__content > .layout-content {
  /* grow to full width (main axis); full height comes from stretch */
  flex: 1 1 0;
  min-width: 0;
}
</style>
