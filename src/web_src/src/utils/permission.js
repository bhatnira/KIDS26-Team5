import { useAuthStore, ROLE_SUPER, ROLE_ADMIN, ROLE_USER } from '@/stores/auth'
import { isArray, isString } from 'radash'

/** Permission check utility */
export function usePermission() {
  const authStore = useAuthStore()

  /**
   * Check if user has permission based on required roles
   * @param {string|string[]|undefined} permission - Required role(s) for access
   * @returns {boolean} - Whether user has permission
   */
  function hasPermission(permission) {
    // No permission required - allow access
    if (!permission)
      return true

    if (!authStore.userInfo)
      return false

    // Handle both 'role' and 'Role' for compatibility
    const role = authStore.userInfo.role || authStore.userInfo.Role

    // Normalize role to array
    const userRoles = isArray(role) ? role : [role]

    // Super users can access everything
    if (userRoles.includes(ROLE_SUPER))
      return true

    // Check against required permissions
    if (isArray(permission)) {
      // Permission is array - check if user has any of the required roles
      return permission.some(p => userRoles.includes(p))
    }

    if (isString(permission)) {
      // Permission is string - check if user has that role
      return userRoles.includes(permission)
    }

    return false
  }

  /**
   * Check if user is admin or super
   */
  function isAdminOrSuper() {
    if (!authStore.userInfo) return false
    const { role } = authStore.userInfo
    const userRoles = isArray(role) ? role : [role]
    return userRoles.includes(ROLE_SUPER) || userRoles.includes(ROLE_ADMIN)
  }

  /**
   * Check if user is super
   */
  function isSuper() {
    if (!authStore.userInfo) return false
    const { role } = authStore.userInfo
    const userRoles = isArray(role) ? role : [role]
    return userRoles.includes(ROLE_SUPER)
  }

  return {
    hasPermission,
    isAdminOrSuper,
    isSuper,
  }
}
