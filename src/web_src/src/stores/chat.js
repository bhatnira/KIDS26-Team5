import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  fetchListConversations,
  fetchGetConversation,
  fetchCreateConversation,
  fetchDeleteConversation,
  sendMessageStream,
} from '@/api/chat'
import { fetchListArtifacts } from '@/api/agent'

/**
 * Roles surfaced in the messages array. Each role corresponds to a
 * different rendering branch in MessageBubble:
 *   user, assistant     — chat text (assistant supports streaming)
 *   reasoning           — assistant thinking block (foldable, muted; streams)
 *   tool_call           — tool invocation card
 *   tool_result         — tool output card
 *   code_exec           — code-block-about-to-run card
 *   code_result         — stdout/stderr + output files
 *   artifact            — artifact card (image/csv/file)
 *   skill_loaded        — small pill
 *   error               — terminal error notice
 */

let nextLocalId = 1
function localId(prefix) {
  return `${prefix}-${Date.now()}-${nextLocalId++}`
}

export const useChatStore = defineStore('chat', () => {
  const conversations = ref([])
  // Conversation-list pagination: load the most recent CONV_PAGE_SIZE, then
  // append further pages as the sidebar is scrolled (infinite scroll).
  const CONV_PAGE_SIZE = 20
  const conversationsHasMore = ref(false)
  const conversationsLoadingMore = ref(false)
  const activeConversationId = ref(null)
  const messages = ref([])
  const isStreaming = ref(false)
  const abortController = ref(null)

  // Prompt-token count of the latest turn = size of the full context last sent
  // to the model. Drives the context-usage meter in the input bar. Updated live
  // by the SSE 'usage' event and seeded from context_tokens on conversation load.
  const contextTokens = ref(0)

  // Artifacts produced during the current session (also surfaced live
  // via the SSE 'artifact' event). The right-side file browser binds
  // to this.
  const artifacts = ref([])

  // ID of the assistant message currently accepting token deltas. Reset
  // by every non-token event so subsequent tokens start a fresh bubble.
  const streamingMsgId = ref(null)

  // ID of the reasoning ("thinking") message currently accepting reasoning
  // deltas. Reset whenever an answer token or any structured event arrives so
  // the next round of thinking (e.g. interleaved between tool calls) opens a
  // fresh, correctly-positioned block.
  const streamingReasoningId = ref(null)

  const activeConversation = computed(() =>
    conversations.value.find(c => c.id === activeConversationId.value) || null
  )

  // Roles considered "detail events" — invocation plumbing that is not
  // the model's final output. They collapse into a single per-turn
  // thinking_cluster so the chat reads like prose + outputs rather than
  // a log of every internal step.
  const DETAIL_ROLES = new Set([
    'tool_call', 'tool_result', 'code_exec', 'code_result', 'skill_loaded',
  ])

  // displayedMessages reorganises the raw event stream into the layout
  // the UI renders:
  //
  //   1. Consecutive detail events collapse into a thinking_cluster.
  //   2. Artifacts produced during a turn are deferred to AFTER the
  //      assistant's final text for that turn (claude.ai pattern).
  //
  // A "turn" starts at each user message. The last cluster of a streaming
  // turn is marked active=true so the UI can render a spinner / "Working…"
  // label rather than the static collapsed summary.
  const displayedMessages = computed(() => {
    const src = messages.value
    if (!src.length) return []

    const out = []
    let buffer = []          // current detail-event run
    let turnArtifacts = []   // artifacts seen since the current user message

    const flushCluster = (active) => {
      if (!buffer.length) return
      out.push({
        id: `cluster-${buffer[0].id}`,
        role: 'thinking_cluster',
        steps: buffer,
        active,
        created_at: buffer[0].created_at,
      })
      buffer = []
    }
    const flushTurnArtifacts = () => {
      if (!turnArtifacts.length) return
      for (const a of turnArtifacts) out.push(a)
      turnArtifacts = []
    }

    for (let i = 0; i < src.length; i++) {
      const m = src[i]
      if (DETAIL_ROLES.has(m.role)) {
        buffer.push(m)
        continue
      }
      if (m.role === 'artifact') {
        // Defer until end-of-turn so outputs always land at the bottom.
        turnArtifacts.push(m)
        continue
      }

      // Non-detail event: close out any pending cluster.
      flushCluster(false)

      if (m.role === 'user') {
        // New turn boundary: flush previous turn's deferred artifacts
        // before pushing the user message.
        flushTurnArtifacts()
        out.push(m)
        continue
      }

      // assistant text, error, or other terminal-ish event.
      out.push(m)
    }

    // Tail handling: any open cluster is the currently-running one when
    // the agent is still streaming. Deferred artifacts are emitted only
    // after streaming finishes so the user sees them as "results" rather
    // than mid-stream interruptions.
    flushCluster(isStreaming.value)
    if (!isStreaming.value) flushTurnArtifacts()
    return out
  })

  // ── Loading ────────────────────────────────────────────────────────────────

  async function loadConversations() {
    const { isSuccess, data } = await fetchListConversations(CONV_PAGE_SIZE, 0)
    if (isSuccess && data?.items) {
      conversations.value = data.items
      conversationsHasMore.value = !!data.has_more
    }
  }

  // Append the next page of conversations. Offset is derived from the number
  // currently loaded; results are deduped by id so a conversation bumped to
  // the top between pages can't appear twice.
  async function loadMoreConversations() {
    if (!conversationsHasMore.value || conversationsLoadingMore.value) return
    conversationsLoadingMore.value = true
    try {
      const offset = conversations.value.length
      const { isSuccess, data } = await fetchListConversations(CONV_PAGE_SIZE, offset)
      if (isSuccess && data?.items) {
        const seen = new Set(conversations.value.map(c => c.id))
        const fresh = data.items.filter(c => !seen.has(c.id))
        conversations.value.push(...fresh)
        conversationsHasMore.value = !!data.has_more
      }
    } finally {
      conversationsLoadingMore.value = false
    }
  }

  async function selectConversation(id) {
    activeConversationId.value = id
    messages.value = []
    artifacts.value = []
    contextTokens.value = 0
    const { isSuccess, data } = await fetchGetConversation(id)
    if (isSuccess) {
      contextTokens.value = data?.context_tokens || 0
    }
    if (isSuccess && data?.messages) {
      // Backend already returns the typed message list (user, assistant,
      // tool_call, tool_result). Pass through with synthesized IDs so the
      // v-for keys stay stable.
      messages.value = data.messages.map((m, i) => ({ id: `m-${i}`, ...m }))
    }
    await loadArtifacts()
  }

  async function loadArtifacts() {
    if (!activeConversationId.value) {
      artifacts.value = []
      return
    }
    try {
      const { isSuccess, data } = await fetchListArtifacts(activeConversationId.value)
      if (isSuccess && data?.items) {
        artifacts.value = data.items
      }
    } catch { /* ignore */ }
  }

  async function createConversation(title = '') {
    const { isSuccess, data } = await fetchCreateConversation({ title })
    if (isSuccess && data) {
      conversations.value.unshift({
        id: data.id,
        title: data.title,
        created_at: data.created_at,
        updated_at: data.updated_at,
      })
      await selectConversation(data.id)
      return data.id
    }
    return null
  }

  async function deleteConversation(id) {
    const { isSuccess } = await fetchDeleteConversation(id)
    if (isSuccess) {
      conversations.value = conversations.value.filter(c => c.id !== id)
      if (activeConversationId.value === id) {
        activeConversationId.value = null
        messages.value = []
        artifacts.value = []
      }
    }
    return isSuccess
  }

  // ── Send + Stream ─────────────────────────────────────────────────────────

  /**
   * Send a user message and stream the assistant reply.
   * @param {string} content
   * @param {{bucket:string,key:string,name:string,mime_type?:string,size?:number}[]} [attachments]
   */
  async function sendMessage(content, attachments = []) {
    if (!activeConversationId.value || isStreaming.value) return

    // 1. Echo the user message locally.
    messages.value.push({
      id: localId('u'),
      role: 'user',
      content,
      attachments: attachments.length ? attachments : undefined,
      created_at: new Date().toISOString(),
    })

    isStreaming.value = true
    streamingMsgId.value = null
    abortController.value = new AbortController()

    try {
      const response = await sendMessageStream(
        activeConversationId.value,
        content,
        abortController.value,
        attachments,
      )

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`)
      }

      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        let blockEnd
        while ((blockEnd = buffer.indexOf('\n\n')) !== -1) {
          const block = buffer.slice(0, blockEnd)
          buffer = buffer.slice(blockEnd + 2)
          if (block.trim()) processSSEChunk(block)
        }
      }
    } catch (err) {
      if (err.name !== 'AbortError') {
        pushMessage({
          role: 'error',
          message: err.message || 'connection failed',
        })
      }
    } finally {
      finalizeStreaming()
    }
  }

  // ── SSE plumbing ──────────────────────────────────────────────────────────

  function processSSEChunk(block) {
    let eventType = 'message'
    let dataStr = ''
    for (const line of block.split('\n')) {
      if (line.startsWith('event: ')) {
        eventType = line.slice(7).trim()
      } else if (line.startsWith('data: ')) {
        dataStr += (dataStr ? '\n' : '') + line.slice(6)
      }
    }
    if (!dataStr) return
    let payload
    try {
      payload = JSON.parse(dataStr)
    } catch {
      return
    }
    handleSSEEvent(eventType, payload)
  }

  function handleSSEEvent(type, payload) {
    switch (type) {
      case 'token':
        // Answer text starts: close any open reasoning block so it renders
        // above the answer and stops accepting deltas.
        streamingReasoningId.value = null
        appendToken(payload.content || '')
        break

      case 'reasoning':
        // Thinking text: close any open answer bubble so reasoning precedes it,
        // then accumulate into the foldable reasoning block.
        streamingMsgId.value = null
        appendReasoning(payload.content || '')
        break

      case 'usage':
        // prompt_tokens = full context sent on this turn. Drives the meter.
        if (payload.prompt_tokens) {
          contextTokens.value = payload.prompt_tokens
        }
        break

      case 'tool_call':
        streamingMsgId.value = null
        pushMessage({
          role: 'tool_call',
          tool_id: payload.id,
          tool_name: payload.name,
          arguments: payload.arguments,
        })
        break

      case 'tool_result':
        streamingMsgId.value = null
        pushMessage({
          role: 'tool_result',
          tool_id: payload.id,
          tool_name: payload.name,
          content: payload.content,
        })
        break

      case 'code_exec':
        streamingMsgId.value = null
        pushMessage({
          role: 'code_exec',
          language: payload.language || 'python',
          code: payload.code || '',
        })
        break

      case 'code_result':
        streamingMsgId.value = null
        pushMessage({
          role: 'code_result',
          stdout: payload.stdout || '',
          stderr: payload.stderr || '',
          exit_code: payload.exit_code,
          files: payload.files || [],
        })
        break

      case 'artifacts_pending':
        // The end-of-turn harvest is about to upload these files. Show a
        // skeleton placeholder per file immediately so the gap between the
        // text finishing and the cards landing isn't a blank wait. Each is
        // replaced in place by its real 'artifact' event when the upload
        // completes (matched by name).
        streamingMsgId.value = null
        for (const it of payload.items || []) {
          pushMessage({
            role: 'artifact',
            name: it.name,
            saved_as: it.name,
            size_bytes: it.size_bytes,
            pending: true,
          })
        }
        break

      case 'artifact': {
        streamingMsgId.value = null
        const card = {
          role: 'artifact',
          name: payload.name,
          saved_as: payload.saved_as,
          version: payload.version,
          mime_type: payload.mime_type,
          size_bytes: payload.size_bytes,
          ref: payload.ref,
          download_url: payload.download_url,
          pending: false,
        }
        // Replace a matching skeleton placeholder in place (keeps the card's
        // position and component instance); otherwise append a fresh card.
        const idx = messages.value.findIndex(
          m => m.role === 'artifact' && m.pending && m.name === payload.name,
        )
        if (idx >= 0) {
          messages.value[idx] = { ...messages.value[idx], ...card }
        } else {
          pushMessage(card)
        }
        // Keep the right-side file browser fresh.
        loadArtifacts()
        break
      }

      case 'artifacts_settled': {
        // Harvest finished. Drop any placeholder whose upload never produced
        // a real card (failed/skipped) so it can't shimmer forever.
        const saved = new Set(payload.saved || [])
        messages.value = messages.value.filter(
          m => !(m.role === 'artifact' && m.pending && !saved.has(m.name)),
        )
        loadArtifacts()
        break
      }

      case 'skill_loaded':
        streamingMsgId.value = null
        pushMessage({
          role: 'skill_loaded',
          name: payload.name,
          domain: payload.domain,
          description: payload.description,
        })
        break

      case 'error':
        streamingMsgId.value = null
        pushMessage({
          role: 'error',
          message: payload.message || 'unknown error',
        })
        break

      case 'title_updated':
        if (activeConversationId.value && payload.title) {
          const idx = conversations.value.findIndex(c => c.id === activeConversationId.value)
          if (idx >= 0) {
            conversations.value[idx] = {
              ...conversations.value[idx],
              title: payload.title,
            }
          }
        }
        break

      case 'done':
        finalizeStreaming()
        // The end-of-turn output harvest may save files after the model's
        // final text. Refresh once on done so the file browser reflects them
        // even if an 'artifact' frame was missed (e.g. a dropped connection).
        loadArtifacts()
        break
    }
  }

  function appendToken(chunk) {
    if (!chunk) return
    if (!streamingMsgId.value) {
      const id = localId('a')
      streamingMsgId.value = id
      messages.value.push({
        id,
        role: 'assistant',
        content: chunk,
        isStreaming: true,
        created_at: new Date().toISOString(),
      })
      return
    }
    const idx = messages.value.findIndex(m => m.id === streamingMsgId.value)
    if (idx === -1) {
      streamingMsgId.value = null
      appendToken(chunk)
      return
    }
    messages.value[idx] = {
      ...messages.value[idx],
      content: (messages.value[idx].content || '') + chunk,
    }
  }

  function appendReasoning(chunk) {
    if (!chunk) return
    if (!streamingReasoningId.value) {
      const id = localId('r')
      streamingReasoningId.value = id
      messages.value.push({
        id,
        role: 'reasoning',
        content: chunk,
        isStreaming: true,
        created_at: new Date().toISOString(),
      })
      return
    }
    const idx = messages.value.findIndex(m => m.id === streamingReasoningId.value)
    if (idx === -1) {
      streamingReasoningId.value = null
      appendReasoning(chunk)
      return
    }
    messages.value[idx] = {
      ...messages.value[idx],
      content: (messages.value[idx].content || '') + chunk,
    }
  }

  function pushMessage(msg) {
    // Any structured event closes an active reasoning bubble so subsequent
    // reasoning starts a fresh, correctly-ordered block.
    streamingReasoningId.value = null
    messages.value.push({
      id: localId(msg.role),
      created_at: new Date().toISOString(),
      ...msg,
    })
  }

  function finalizeStreaming() {
    isStreaming.value = false
    abortController.value = null
    // Mark the currently-streaming bubble as final.
    if (streamingMsgId.value) {
      const idx = messages.value.findIndex(m => m.id === streamingMsgId.value)
      if (idx !== -1) {
        messages.value[idx] = { ...messages.value[idx], isStreaming: false }
      }
      streamingMsgId.value = null
    }
    // Mark a still-open reasoning bubble as final so it auto-collapses.
    if (streamingReasoningId.value) {
      const idx = messages.value.findIndex(m => m.id === streamingReasoningId.value)
      if (idx !== -1) {
        messages.value[idx] = { ...messages.value[idx], isStreaming: false }
      }
      streamingReasoningId.value = null
    }
    // Bump the conversation in the list so the UI ordering stays fresh.
    if (activeConversationId.value) {
      const convIdx = conversations.value.findIndex(c => c.id === activeConversationId.value)
      if (convIdx > 0) {
        const [conv] = conversations.value.splice(convIdx, 1)
        conv.updated_at = new Date().toISOString()
        conversations.value.unshift(conv)
      }
    }
  }

  function stopStreaming() {
    if (abortController.value) {
      abortController.value.abort()
    }
  }

  function clearMessages() {
    messages.value = []
    artifacts.value = []
    activeConversationId.value = null
    streamingMsgId.value = null
    streamingReasoningId.value = null
    contextTokens.value = 0
  }

  return {
    conversations,
    conversationsHasMore,
    conversationsLoadingMore,
    activeConversationId,
    activeConversation,
    messages,
    displayedMessages,
    artifacts,
    isStreaming,
    contextTokens,
    loadConversations,
    loadMoreConversations,
    loadArtifacts,
    selectConversation,
    createConversation,
    deleteConversation,
    sendMessage,
    stopStreaming,
    clearMessages,
    // exposed for tests
    processSSEChunk,
    handleSSEEvent,
  }
})
