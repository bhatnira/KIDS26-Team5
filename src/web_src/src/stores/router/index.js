import { router } from '@/router'
import { staticRoutes } from '@/router/routes.auth.js'
import { $t } from '@/utils/i18n'
import { createMenus, createRoutes, generateCacheRoutes } from './utils'

/**
 * use composition api for router status store
 */
export const useRouteStore = defineStore('route-store', () => {
  // status definition
  const isInitAuthRoute = ref(false)
  const activeMenu = ref(null)
  const menus = ref([])
  const rowRoutes = ref([])
  const cacheRoutes = ref([])

  /**
   * reset routes store
   */
  function resetRouteStore() {
    resetRoutes()
    // reset all the status to default value
    isInitAuthRoute.value = false
    activeMenu.value = null
    menus.value = []
    rowRoutes.value = []
    cacheRoutes.value = []
  }

  /**
   * reset routes
   */
  function resetRoutes() {
    if (router.hasRoute('appRoot')) router.removeRoute('appRoot')
  }

  /**
   * set current active menu
   * @param {string} key menu key
   */
  function setActiveMenu(key) {
    activeMenu.value = key
  }

  /**
   * 初始化路由信息
   */
  function initRouteInfo() {
    rowRoutes.value = staticRoutes
    return staticRoutes
  }

  /**
   * init auth routers
   */
  function initAuthRoute() {
    isInitAuthRoute.value = false

    try {
      // 初始化路由信息
      const routes = initRouteInfo()
      if (!routes) {
        const error = new Error('Failed to get route information')
        window.$message.error($t(`app.getRouteError`))
        throw error
      }
      rowRoutes.value = routes

      // 生成实际路由并插入
      const appRoutes = createRoutes(routes)
      router.addRoute(appRoutes)

      // 生成侧边菜单
      menus.value = createMenus(routes)

      // 生成路由缓存
      cacheRoutes.value = generateCacheRoutes(routes)

      isInitAuthRoute.value = true
    } catch (error) {
      // 重置状态并重新抛出错误
      isInitAuthRoute.value = false
      throw error
    }
  }

  // return all the status and methods
  return {
    // status
    isInitAuthRoute,
    activeMenu,
    menus,
    rowRoutes,
    cacheRoutes,

    // methods
    resetRouteStore,
    resetRoutes,
    setActiveMenu,
    initRouteInfo,
    initAuthRoute,
  }
})
