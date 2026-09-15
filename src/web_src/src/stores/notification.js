import { fetchNotifications, fetchMarkRead, fetchMarkAllRead, openNotificationStream } from '@/api/notification'

export const useNotificationStore = defineStore('notification-store', () => {
  const notifications = ref([])
  const loading = ref(false)

  const unreadCount = computed(() => notifications.value.filter(n => !n.is_read).length)

  // Group by type for Notices.vue tabs (0=notifications, 1=messages, 2=todos)
  const grouped = computed(() => {
    const map = {}
    for (const n of notifications.value) {
      if (!map[n.type]) map[n.type] = []
      map[n.type].push(toMessage(n))
    }
    return map
  })

  async function loadNotifications() {
    loading.value = true
    try {
      const { isSuccess, data } = await fetchNotifications(1, 50)
      if (isSuccess && data?.items) {
        notifications.value = data.items
      }
    } finally {
      loading.value = false
    }
  }

  async function markRead(id) {
    await fetchMarkRead(id)
    const n = notifications.value.find(n => n.id === id)
    if (n) n.is_read = true
  }

  async function markAllRead() {
    await fetchMarkAllRead()
    notifications.value.forEach(n => { n.is_read = true })
  }

  // Opens the SSE stream with automatic exponential-backoff reconnect.
  // Returns a stop function that permanently aborts the stream (used on unmount).
  function startStream() {
    // A single AbortController spans all reconnect attempts.
    // Calling abort() prevents any further reconnections.
    const controller = new AbortController()

    const INITIAL_DELAY_MS = 1_000
    const MAX_DELAY_MS = 30_000
    let retryDelay = INITIAL_DELAY_MS
    let retryTimer = null

    async function connect() {
      if (controller.signal.aborted) return

      let response
      try {
        response = await openNotificationStream(controller.signal)
      } catch {
        // fetch threw (network error or intentional abort)
        scheduleRetry()
        return
      }

      // Auth gap: no token yet (response === null), or the token was rejected
      // (401/403). Don't hammer the server — the HTTP layer refreshes the token
      // for normal requests, or redirects to login (which unmounts this panel
      // and aborts the stream). Poll at the max interval so the live stream
      // self-heals once a fresh token lands, without spamming 401s.
      if (!response || response.status === 401 || response.status === 403) {
        retryDelay = MAX_DELAY_MS
        scheduleRetry()
        return
      }

      if (!response.ok || !response.body) {
        scheduleRetry()
        return
      }

      // Successfully opened — reset backoff delay
      retryDelay = INITIAL_DELAY_MS

      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buf = ''

      try {
        while (true) {
          const { value, done } = await reader.read()
          if (done) break

          buf += decoder.decode(value, { stream: true })
          const lines = buf.split('\n')
          buf = lines.pop() // keep incomplete trailing line

          let eventType = ''
          const dataLines = []
          for (const line of lines) {
            if (line.startsWith('event: ')) {
              eventType = line.slice(7).trim()
            } else if (line.startsWith('data: ')) {
              dataLines.push(line.slice(6))
            } else if (line === '') {
              if (eventType === 'notifications' && dataLines.length) {
                try {
                  notifications.value = JSON.parse(dataLines.join('\n'))
                } catch { /* ignore malformed frames */ }
              }
              eventType = ''
              dataLines.length = 0
            }
          }
        }
      } catch {
        // reader.read() threw — network drop or abort
      }

      // Stream ended (server closed or network error).
      // Reconnect unless we were deliberately stopped.
      scheduleRetry()
    }

    function scheduleRetry() {
      if (controller.signal.aborted) return
      retryTimer = setTimeout(() => {
        retryTimer = null
        connect()
      }, retryDelay)
      retryDelay = Math.min(retryDelay * 2, MAX_DELAY_MS)
    }

    connect()

    return function stop() {
      controller.abort()
      if (retryTimer !== null) {
        clearTimeout(retryTimer)
        retryTimer = null
      }
    }
  }

  let stopStream = null

  function init() {
    loadNotifications()
    if (stopStream) stopStream()
    stopStream = startStream()
  }

  function cleanup() {
    if (stopStream) {
      stopStream()
      stopStream = null
    }
  }

  return {
    notifications,
    loading,
    unreadCount,
    grouped,
    init,
    cleanup,
    markRead,
    markAllRead,
  }
})

// Maps a backend notification DTO to the Entity.Message shape used by NoticeList.vue
function toMessage(n) {
  return {
    id: n.id,
    type: n.type,
    title: n.title,
    icon: n.icon || 'icon-park-outline:remind',
    tagTitle: n.tag_title || undefined,
    tagType: n.tag_type || undefined,
    description: n.description || undefined,
    date: n.date,
    isRead: n.is_read,
  }
}
