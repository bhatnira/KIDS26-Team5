import { request } from '@/service/http'
import { local } from '@/utils/storage'

// ── Conversation CRUD ─────────────────────────────────────────────────────────
// Conversation IDs are now framework session IDs (string UUIDs).

export function fetchCreateConversation(data = {}) {
  return request.Post('/chat/conversations', data)
}

export function fetchListConversations(limit = 20, offset = 0) {
  return request.Get('/chat/conversations', { params: { limit, offset } })
}

export function fetchGetConversation(id) {
  return request.Get(`/chat/conversations/${id}`)
}

export function fetchDeleteConversation(id) {
  return request.Delete(`/chat/conversations/${id}`)
}

/**
 * Send a message and get a streaming SSE response.
 *
 * The backend now accepts an `attachments` array referencing files
 * already uploaded into the user's workspace bucket:
 *   [{ bucket, key, name, mime_type, size }]
 *
 * @param {string} conversationId
 * @param {string} content
 * @param {AbortController} abortController
 * @param {{bucket:string, key:string, name:string, mime_type?:string, size?:number}[]} [attachments]
 * @returns {Promise<Response>}
 */
export async function sendMessageStream(conversationId, content, abortController, attachments = []) {
  const baseURL = __URL_MAP__.url.path
  const token = local.get('accessToken')

  return await fetch(`${baseURL}/chat/conversations/${conversationId}/messages`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
    body: JSON.stringify({ content, attachments }),
    signal: abortController?.signal,
  })
}
