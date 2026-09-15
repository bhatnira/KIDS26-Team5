import { useAppStore } from '@/stores/app'
import { useRouteStore } from '@/stores/router'
import { useTabStore } from '@/stores'
import { local } from '@/utils/storage'

const { VITE_APP_NAME } = import.meta.env
const title = VITE_APP_NAME

export function setupRouterGuard(router) {
  const appStore = useAppStore()
  const routeStore = useRouteStore()
  const tabStore = useTabStore()

  router.beforeEach(async (to, from, next) => {
    const meta = {...to.meta};
    delete meta.__navigationId;
    to.meta = meta;

    // if outlink, open the link and intercept it
    if (to.meta.href) {
      window.open(to.meta.href)
      next(false) // 取消当前导航
      return
    }

    appStore.showProgress && window.$loadingBar?.start()

    // whether having the auth token
    const isLogin = Boolean(local.get('accessToken'))

    if (to.name === 'root') {
      if (isLogin) {
        next({ path: import.meta.env.VITE_HOME_PATH, replace: true })
      } else {
        // nog logged in
        next({ path: '/login', replace: true })
      }
      return
    }

    if (to.name === 'login') {
      // login, no checking
    } else if (to.meta.requiresAuth === false) {
      // no checking
    } else if (to.meta.requiresAuth === true && !isLogin) {
      const redirect = to.name === 'not-found' ? undefined : to.fullPath
      next({ path: '/login', query: { redirect } })
      return
    }

    if (!routeStore.isInitAuthRoute && to.name !== 'login') {
      try {
        await routeStore.initAuthRoute()
        // dynamic route loaded then go to root
        if (to.name === 'not-found') {
          // wait authStore loaded and go to the previous or 404
          next({
            path: to.fullPath,
            replace: true,
            query: to.query,
            hash: to.hash,
          })
          return
        }
      } catch (error) {
        // if initRoute failed (401 error), go to the login page
        const redirect = to.fullPath !== '/' ? to.fullPath : undefined
        next({ path: '/login', query: redirect ? { redirect }: undefined })
        return
      }
    }

    // if user already logged in and go to login page, then redirect to root
    if (to.name === 'login' && isLogin) {
      next({path: '/'})
      return
    }

    next()
  })

  router.beforeResolve((to) => {
    // set menu highlight
    routeStore.setActiveMenu(to.meta.activeMenu ?? to.fullPath)
    // add tabs
    tabStore.addTab(to)
    tabStore.setCurrentTab(to.fullPath)
  })

  router.afterEach((to) => {
    // modify page title
    document.title = `${to.meta.title} - ${title}`
    // end the loading bar
    appStore.showProgress && window.$loadingBar?.finish()
  })
}
