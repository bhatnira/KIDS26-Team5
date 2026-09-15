import { router } from '@/router'

export const useTabStore = defineStore(
  'tab-store',
  () => {
    // state
    const pinTabs = ref([])
    const tabs = ref([])
    const currentTabPath = ref('')

    const allTabs = computed(() => [...pinTabs.value, ...tabs.value])

    // actions
    function addTab(route) {
      if (route.meta.withoutTab) return

      if (hasExistTab(route.fullPath)) return

      if (route.meta.pinTab) pinTabs.value.push(route)
      else tabs.value.push(route)
    }

    async function closeTab(fullPath) {
      const tabsLength = tabs.value.length
      if (tabs.value.length > 1) {
        const index = getTabIndex(fullPath)
        const isLast = index + 1 === tabsLength
        if (currentTabPath.value === fullPath && !isLast) {
          await router.push(tabs.value[index + 1].fullPath)
        } else if (currentTabPath.value === fullPath && isLast) {
          await router.push(tabs.value[index - 1].fullPath)
        }
      }

      tabs.value = tabs.value.filter((item) => {
        return item.fullPath !== fullPath
      })

      if (tabsLength - 1 === 0) await router.push('/')
    }

    function closeOtherTabs(fullPath) {
      const index = getTabIndex(fullPath)
      tabs.value = tabs.value.filter((item, i) => i === index)
    }

    function closeLeftTabs(fullPath) {
      const index = getTabIndex(fullPath)
      tabs.value = tabs.value.filter((item, i) => i >= index)
    }

    function closeRightTabs(fullPath) {
      const index = getTabIndex(fullPath)
      tabs.value = tabs.value.filter((item, i) => i <= index)
    }

    function clearAllTabs() {
      tabs.value.length = 0
      pinTabs.value.length = 0
    }

    async function closeAllTabs() {
      tabs.value.length = 0
      await router.push('/')
    }

    function hasExistTab(fullPath) {
      const _tabs = [...tabs.value, ...pinTabs.value]
      return _tabs.some((item) => {
        return item.fullPath === fullPath
      })
    }

    /* 设置当前激活的标签 */
    function setCurrentTab(fullPath) {
      currentTabPath.value = fullPath
    }

    function getTabIndex(fullPath) {
      return tabs.value.findIndex((item) => {
        return item.fullPath === fullPath
      })
    }

    function modifyTab(fullPath, modifyFn) {
      const index = getTabIndex(fullPath)
      modifyFn(tabs.value[index])
    }

    return {
      // state
      pinTabs,
      tabs,
      currentTabPath,
      // getters
      allTabs,
      // actions
      addTab,
      closeTab,
      closeOtherTabs,
      closeLeftTabs,
      closeRightTabs,
      clearAllTabs,
      closeAllTabs,
      hasExistTab,
      setCurrentTab,
      getTabIndex,
      modifyTab,
    }
  },
  {
    persist: {
      storage: sessionStorage,
    },
  },
)
