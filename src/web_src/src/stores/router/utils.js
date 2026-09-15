import { usePermission } from '@/utils/permission'
import { arrayToTree } from '@/utils/array'
import { renderIcon } from '@/utils/icon'
import Layout from '@/layouts/index.vue'
import { $t } from '@/utils/i18n'
import { clone, min, omit, pick } from 'radash'
import { RouterLink } from 'vue-router'

const metaFields = [
  'title',
  'icon',
  'requiresAuth',
  'roles',
  'keepAlive',
  'hide',
  'order',
  'href',
  'activeMenu',
  'withoutTab',
  'pinTab',
  'menuType',
]

function standardizedRoutes(route) {
  return clone(route).map((i) => {
    const route = omit(i, metaFields)

    Reflect.set(route, 'meta', pick(i, metaFields))
    return route
  })
}

export function createRoutes(routes) {
  const { hasPermission } = usePermission()

  // Structure the meta field
  let resultRouter = standardizedRoutes(routes)

  // Route permission filtering
  resultRouter = resultRouter.filter((i) => hasPermission(i.meta.roles))

  // Generate routes, no need to import files for those with redirect
  const modules = import.meta.glob('@/views/**/*.vue')
  resultRouter = resultRouter.map((item) => {
    if (item.componentPath && !item.redirect)
      item.component = modules[`/src/views${item.componentPath}`]
    return item
  })

  // Generate route tree
  resultRouter = arrayToTree(resultRouter)

  const appRootRoute = {
    path: '/appRoot',
    name: 'appRoot',
    redirect: import.meta.env.VITE_HOME_PATH,
    component: Layout,
    meta: {
      title: '',
      icon: 'icon-park-outline:home',
    },
    children: [],
  }

  // set the correct redirect path for the route
  setRedirect(resultRouter)

  // Insert the processed route into the root route
  appRootRoute.children = resultRouter

  return appRootRoute
}

// Generate an array of route names that need to be kept alive
export function generateCacheRoutes(routes) {
  return routes.filter((i) => i.keepAlive).map((i) => i.name)
}

function setRedirect(routes) {
  routes.forEach((route) => {
    if (route.children) {
      if (!route.redirect) {
        // Filter out a collection of child elements that are not hidden
        const visibleChildren = route.children.filter(child => !child.meta.hide)

        // Redirect page to the path of the first child element by default
        let target = visibleChildren[0]

        // Filter out pages with the order attribute
        const orderChildren = visibleChildren.filter(child => child.meta.order)

        if (orderChildren.length > 0) target = min(orderChildren, i => i.meta.order)

        if (target) route.redirect = target.path
      }

      setRedirect(route.children)
    }
  })
}

export function createMenus(userRoutes) {
  const resultMenus = standardizedRoutes(userRoutes)

  // filter menus that do not need to be displayed
  const visibleMenus = resultMenus.filter(route => !route.meta.hide)

  // generate side menu
  return arrayToTree(transformAuthRoutesToMenus(visibleMenus))
}

function transformAuthRoutesToMenus(userRoutes) {
  const { hasPermission } = usePermission()
  return userRoutes
    .filter(i => hasPermission(i.meta.roles))
    //  Sort the menu according to the order size
    .sort((a, b) => {
      if (a.meta && a.meta.order && b.meta && b.meta.order)
        return a.meta.order - b.meta.order
      else if (a.meta && a.meta.order)
        return -1
      else if (b.meta && b.meta.order)
        return 1
      else return 0
    })
    // Convert to side menu data structure
    .map((item) => {
      return {
        id: item.id,
        pid: item.pid,
        label:
          (!item.meta.menuType || item.meta.menuType === 'page')
            ? () =>
              h(
                RouterLink,
                {
                  to: {
                    path: item.path,
                  },
                },
                { default: () => $t(`route.${String(item.name)}`, item.meta.title) },
              )
            : () => $t(`route.${String(item.name)}`, item.meta.title),
        key: item.path,
        icon: item.meta.icon ? renderIcon(item.meta.icon) : undefined,
      }
    })
}
