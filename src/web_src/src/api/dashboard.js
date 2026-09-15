import { request } from '@/service/http'

/**
 * Fetch dashboard statistics (user-specific)
 * Returns full stats for admin/super, user-specific stats for regular users
 */
export function fetchDashboardStats() {
  return request.Get('/user/dashboard/stats')
}

/**
 * Fetch full admin dashboard statistics
 * Only available to admin/super users
 */
export function fetchAdminDashboardStats() {
  return request.Get('/admin/dashboard/stats')
}
