import { router } from '@/router'
import { local } from '@/utils/storage'
import { useRouteStore } from '@/stores/router'
import { fetchLogin } from '@/api/login'
import { useTabStore } from './tab'
import { computed } from 'vue'

// Role constants
export const ROLE_SUPER = 'super'
export const ROLE_ADMIN = 'admin'
export const ROLE_USER = 'user'

export const useAuthStore = defineStore('auth-store', () => {
  // state
  const userInfo = ref(local.get('userInfo'))
  const token = ref(local.get('accessToken') || '')

  // getters
  const isLogin = computed(() => Boolean(token.value))

  // Role getters
  const userRole = computed(() => {
    if (!userInfo.value) return ROLE_USER
    // Handle both 'role' and 'Role' for compatibility, can be string or array
    const role = userInfo.value.role || userInfo.value.Role
    if (Array.isArray(role)) return role[0] || ROLE_USER
    return role || ROLE_USER
  })

  const isSuper = computed(() => userRole.value === ROLE_SUPER)
  const isAdmin = computed(() => userRole.value === ROLE_ADMIN)
  const isAdminOrSuper = computed(() => userRole.value === ROLE_SUPER || userRole.value === ROLE_ADMIN)

  // methods
  function clearAuthStorage() {
    sessionStorage.clear()
    local.remove('accessToken')
    local.remove('refreshToken')
    local.remove('userInfo')
  }

  function setAuthStorage(data) {
    const { accessExpiresAt, refreshExpiresAt } = data
    // Use role from backend (handle both 'role' and 'Role' for compatibility)
    const role = data.userInfo.role || data.userInfo.Role || ROLE_USER
    // Normalize to array for backward compatibility
    data.userInfo.role = Array.isArray(role) ? role : [role]
    // Remove capitalized Role if present to avoid confusion
    delete data.userInfo.Role
    local.set('userInfo', data.userInfo, accessExpiresAt)
    local.set('accessToken', data.accessToken, accessExpiresAt) // save access token in localStorage
    local.set('refreshToken', data.refreshToken, refreshExpiresAt)

    token.value = data.accessToken
    userInfo.value = data.userInfo
  }

  /** merge fields into the cached userInfo and persist them */
  function updateUserInfo(partial) {
    const merged = { ...(userInfo.value || {}), ...partial }
    userInfo.value = merged
    local.set('userInfo', merged)
  }

  /** user logout **/
  async function logout() {
    const route = unref(router.currentRoute)
    // clear local storage
    clearAuthStorage()

    // clear route and menu
    const routeStore = useRouteStore()
    routeStore.resetRouteStore()

    // clear the tab store
    const tabStore = useTabStore()
    tabStore.clearAllTabs()

    // reset status
    userInfo.value = null
    token.value = ''

    // redirect to Login page
    if (route.meta.requiresAuth) {
      await router.push({
        name: 'login',
        query: {
          redirect: route.fullPath,
        },
      })
    }
  }

  async function login(account, pwd) {
    try {
      const { isSuccess, data } = await fetchLogin({email: account, password: pwd})
      if (!isSuccess) return

      // process login info
      await handleLoginInfo(data)
    } catch (error) {
      console.warn('[Login Error]:', error)
    }
  }

  async function handleLoginInfo(data) {
    setAuthStorage(data)

    // add routes and menus
    const routeStore = useRouteStore()
    await routeStore.initAuthRoute()

    // redirect
    const route = unref(router.currentRoute)
    const query = route.query
    await router.push({
      path: query.redirect || '/',
    })
  }

  return {
    // state
    userInfo,
    token,

    // getters
    isLogin,
    userRole,
    isSuper,
    isAdmin,
    isAdminOrSuper,

    // actions
    logout,
    login,
    setAuthStorage,
    clearAuthStorage,
    handleLoginInfo,
    updateUserInfo,
  }
})
