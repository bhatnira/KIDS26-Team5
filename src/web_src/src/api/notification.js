import { request } from '@/service/http'
import { local } from '@/utils/storage'

export function fetchNotifications(page = 1, pageSize = 50) {
  return request.Get('/notifications', { params: { page, page_size: pageSize } })
}

export function fetchMarkRead(id) {
  return request.Put(`/notifications/${id}/read`)
}

export function fetchMarkAllRead() {
  return request.Put('/notifications/read-all')
}

// Returns a fetch() Response with a readable SSE body, or null when there is no
// valid access token (expired tokens are evicted by local.get, returning null).
// Uses fetch (not EventSource) so we can set the Authorization header.
export async function openNotificationStream(signal) {
  const baseURL = __URL_MAP__.url.path
  const token = local.get('accessToken')
  // Avoid sending `Bearer null`, which the server rejects with 401. Signal the
  // missing-token case to the caller so it can wait for a refresh instead.
  if (!token) return null
  return fetch(`${baseURL}/notifications/stream`, {
    headers: {
      Authorization: `Bearer ${token}`,
      Accept: 'text/event-stream',
    },
    signal,
  })
}
