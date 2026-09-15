<script setup>
import { ref, computed, nextTick, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import { useChatStore } from '@/stores/chat'
import { useAppStore } from '@/stores'
import { fetchLLMConfigs } from '@/api/llm'
import { fetchAgentWorkspaceConfig } from '@/api/agent'
import ConversationList from './components/ConversationList.vue'
import MessageBubble from './components/MessageBubble.vue'
import AttachmentUploader from './components/AttachmentUploader.vue'
import FileBrowserPanel from './components/FileBrowserPanel.vue'

const chatStore = useChatStore()
const appStore = useAppStore()
const router = useRouter()

const primaryColor = computed(() => appStore.primaryColor || '#18a058')

// Code-block palette: always dark, even when the rest of the chat is in
// light mode. Matches the claude.ai pattern (dark blocks read better and
// match the imported atom-one-dark hljs theme).
const codeBg = computed(() => '#1e1e2e')
const codeFg = computed(() => '#cdd6f4')

const inputText = ref('')
const pendingAttachments = ref([])
const messageContainerRef = ref(null)
const messageInnerRef = ref(null)
// Whether the view should keep pinning to the newest output. Flipped off when
// the user scrolls up to read earlier content, back on when they return to the
// bottom — so auto-scroll follows streaming without fighting the user.
const stickToBottom = ref(true)
let resizeObserver = null
// Set true right before a programmatic scroll so the resulting scroll event is
// ignored by onMessagesScroll (only real user scrolls should toggle follow mode).
let programmaticScroll = false
// Coalesce pins to one per animation frame so bursty resize callbacks (token
// growth + async Shiki highlight in the same frame) produce a single, smooth
// scroll adjustment instead of several competing jumps (the source of jitter).
let pinScheduled = false
const inputRef = ref(null)
const conversationsLoading = ref(false)

// Right-side file browser panel toggle.
const filesPanelOpen = ref(true)

// LLM config — selection is purely UI; model is resolved per-user
// server-side via llmconfig.Manager.GetDefaultConfig(userID).
const llmConfigs = ref([])
const llmLoading = ref(true)
const selectedConfigId = ref('')

// At least one LLM provider saved for this user?
const llmConfigured = computed(() => llmConfigs.value.length > 0)

const llmConfigOptions = computed(() =>
  llmConfigs.value.map(c => ({ label: c.name, value: c.id }))
)

const currentModelLabel = computed(() => {
  const cfg = llmConfigs.value.find(c => c.id === selectedConfigId.value)
  return cfg ? cfg.model : (llmConfigs.value.length ? llmConfigs.value[0].model : 'No model')
})

// ── Context-usage meter ──────────────────────────────────────────────────────
// The agent reports prompt-token usage per turn (SSE 'usage' event, also
// surfaced on conversation reload). We divide that by the model's context
// window to show how full the context is. No provider exposes the window size
// programmatically, so we infer it from the model name with a sane default.
function contextWindowFor(model) {
  const m = (model || '').toLowerCase()
  if (m.includes('gemini')) return 1000000
  if (m.includes('claude') || m.includes('sonnet') || m.includes('opus') || m.includes('haiku')) return 200000
  if (m.includes('deepseek')) return 128000
  if (m.includes('gpt-3.5')) return 16385
  if (m.includes('gpt-4o') || m.includes('gpt-4.1') || m.includes('gpt-5')
    || m.includes('gpt-4') || m.startsWith('o1') || m.startsWith('o3')) return 128000
  return 128000
}

const contextWindow = computed(() => contextWindowFor(currentModelLabel.value))

const contextPercent = computed(() => {
  if (!chatStore.contextTokens || !contextWindow.value) return 0
  return Math.min(100, Math.round((chatStore.contextTokens / contextWindow.value) * 100))
})

const contextColor = computed(() => {
  const p = contextPercent.value
  if (p >= 90) return '#d03050'
  if (p >= 70) return '#f0a020'
  return primaryColor.value
})

const contextTooltip = computed(() => {
  if (!chatStore.contextTokens) return 'Context usage — appears after the first response'
  return `Context: ${contextPercent.value}% · ${chatStore.contextTokens.toLocaleString()}`
    + ` / ${contextWindow.value.toLocaleString()} tokens`
})

// Workspace configured? Drives a banner + disables attachments/sends.
const workspaceConfigured = ref(false)
const workspaceLoading = ref(true)

async function loadWorkspaceStatus() {
  workspaceLoading.value = true
  try {
    const { isSuccess, data } = await fetchAgentWorkspaceConfig()
    workspaceConfigured.value = !!(isSuccess && data?.configured)
  } catch { workspaceConfigured.value = false } finally {
    workspaceLoading.value = false
  }
}

// Reorganised message list: detail events grouped into thinking_cluster
// entries and artifacts deferred to the end of each turn (see chat
// store's displayedMessages for the exact rules).
const displayedMessages = computed(() => chatStore.displayedMessages)

// Centered layout when no messages; bottom layout once conversation has content
const hasMessages = computed(() =>
  chatStore.messages.length > 0 || chatStore.isStreaming
)

// ── Agent setup gating ───────────────────────────────────────────────────────
// The backend agent factory needs BOTH an LLM provider and an agent workspace
// to run — even a text-only turn fails at build time without a workspace
// (internal/modules/agent/factory.go). So we gate Send on both, list the missing
// pieces in the header banner, and explain the disabled Send via a tooltip.
const setupLoading = computed(() => llmLoading.value || workspaceLoading.value)
const setupComplete = computed(() => llmConfigured.value && workspaceConfigured.value)

// Missing-setup items for the header banner (empty once fully configured or
// while the statuses are still loading, to avoid a warning flash on mount).
const missingSetup = computed(() => {
  if (setupLoading.value) return []
  const items = []
  if (!llmConfigured.value) {
    items.push({ label: 'LLM provider', hint: 'not configured', to: '/user-setting/llm' })
  }
  if (!workspaceConfigured.value) {
    items.push({ label: 'Agent workspace', hint: 'not set up', to: '/user-setting/agent-workspace' })
  }
  return items
})

// Tooltip shown on the disabled Send button while setup is incomplete.
const sendDisabledReason = computed(() => {
  if (setupLoading.value) return 'Checking your configuration…'
  const missing = []
  if (!llmConfigured.value) missing.push('an LLM provider')
  if (!workspaceConfigured.value) missing.push('an agent workspace')
  return missing.length ? `Configure ${missing.join(' and ')} to start chatting` : ''
})

const canSend = computed(() =>
  inputText.value.trim().length > 0
  && !chatStore.isStreaming
  && setupComplete.value
)

// Filenames attached in earlier turns of the active conversation. Used to
// fence cross-turn same-name attachments (which would collide on the staged
// path). Sourced from committed user messages, so it survives a page reload —
// the backend re-emits attachment names on conversation load.
const reservedAttachmentNames = computed(() => {
  const names = []
  for (const m of chatStore.messages) {
    if (m.role === 'user' && Array.isArray(m.attachments)) {
      for (const a of m.attachments) {
        if (a?.name) names.push(a.name)
      }
    }
  }
  return names
})

// Pin the view to the very bottom. We set a guard flag so the scroll event
// this triggers isn't mistaken for the user scrolling away (see onMessagesScroll).
function pinToBottom() {
  const el = messageContainerRef.value
  if (!el) return
  const target = el.scrollHeight - el.clientHeight
  // Already at the bottom → nothing to scroll; don't arm the guard (an unused
  // guard would swallow the user's next real scroll event).
  if (Math.abs(el.scrollTop - target) < 1) return
  programmaticScroll = true
  el.scrollTop = target
}

// Pin at most once per animation frame.
function schedulePin() {
  if (pinScheduled) return
  pinScheduled = true
  requestAnimationFrame(() => {
    pinScheduled = false
    if (stickToBottom.value) pinToBottom()
  })
}

async function scrollToBottom() {
  await nextTick()
  pinToBottom()
}

// Distinguish the user's own scrolling from our programmatic pins and from the
// browser's scroll-anchoring reflows (which fire spurious scroll events when
// streamdown replaces nodes on a big chunk). ONLY a genuine user scroll changes
// follow mode: away from the bottom pauses it, back to the bottom resumes it.
function onMessagesScroll() {
  if (programmaticScroll) {
    programmaticScroll = false
    return
  }
  const el = messageContainerRef.value
  if (!el) return
  stickToBottom.value = el.scrollHeight - el.scrollTop - el.clientHeight < 60
}

// Re-pin and jump on a NEW message (send / new turn / conversation reload).
// Per-token and big-chunk growth within a message is handled by the
// ResizeObserver below (markstream/streamdown re-render and highlight code
// asynchronously, so a one-shot scroll on token change lands before the DOM
// has grown and the view falls behind on long or bursty replies).
watch(() => chatStore.messages.length, () => {
  stickToBottom.value = true
  scrollToBottom()
})

async function handleSelectConversation(id) {
  await chatStore.selectConversation(id)
  stickToBottom.value = true
  await scrollToBottom()
}

async function handleDeleteConversation(id) {
  const ok = await chatStore.deleteConversation(id)
  if (ok) window.$message.success('Conversation deleted')
}

async function handleNewConversation() {
  await chatStore.createConversation()
  await nextTick()
  inputRef.value?.focus()
}

async function handleSend() {
  const content = inputText.value.trim()
  if (!content || !canSend.value) return

  const attachments = pendingAttachments.value
  inputText.value = ''
  pendingAttachments.value = []

  if (!chatStore.activeConversationId) {
    await chatStore.createConversation()
  }

  await chatStore.sendMessage(content, attachments)
}

function handleKeydown(e) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    handleSend()
  }
}

function handleStop() {
  chatStore.stopStreaming()
}

async function loadLLMConfigs() {
  llmLoading.value = true
  try {
    const { isSuccess, data } = await fetchLLMConfigs()
    if (isSuccess && data?.configs) {
      llmConfigs.value = data.configs
      const def = data.configs.find(c => c.is_default)
      selectedConfigId.value = def?.id || data.configs[0]?.id || ''
    }
  } catch { /* ignore */ } finally {
    llmLoading.value = false
  }
}

function goConfigureLLM() {
  router.push('/user-setting/llm')
}

onMounted(async () => {
  conversationsLoading.value = true
  await Promise.all([
    chatStore.loadConversations(),
    loadLLMConfigs(),
    loadWorkspaceStatus(),
  ])
  conversationsLoading.value = false
  nextTick(() => inputRef.value?.focus())
})

// Keep the newest streamed output in view. The message area lives behind a
// v-if (State B), so its content wrapper is NOT present at onMounted time —
// watch the template ref and (re)attach a ResizeObserver whenever it mounts.
// The observer re-pins to the bottom on every height change (each streamed
// token, plus async Shiki code-block highlighting), as long as the user
// hasn't scrolled up to read earlier content.
watch(messageInnerRef, (el) => {
  resizeObserver?.disconnect()
  resizeObserver = null
  if (el && 'ResizeObserver' in window) {
    resizeObserver = new ResizeObserver(() => {
      if (stickToBottom.value) schedulePin()
    })
    resizeObserver.observe(el)
  }
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
})
</script>

<template>
  <div
    class="chat-page"
    :style="{
      '--chat-primary': primaryColor,
      '--chat-code-bg': codeBg,
      '--chat-code-fg': codeFg,
    }"
  >
    <!-- ── Left Sidebar ────────────────────────────────────────── -->
    <div class="chat-sidebar">
      <ConversationList
        :conversations="chatStore.conversations"
        :active-id="chatStore.activeConversationId"
        :loading="conversationsLoading"
        :has-more="chatStore.conversationsHasMore"
        :loading-more="chatStore.conversationsLoadingMore"
        @select="handleSelectConversation"
        @delete="handleDeleteConversation"
        @new="handleNewConversation"
        @load-more="chatStore.loadMoreConversations"
      />
    </div>

    <!-- ── Middle: Chat ──────────────────────────────────────── -->
    <div class="chat-main">

      <!-- Setup guard banner: shown until BOTH the LLM provider and the agent
           workspace are configured. Lists each missing piece with a link. -->
      <div v-if="missingSetup.length" class="chat-banner">
        <icon-park-outline-attention class="chat-banner__icon" />
        <div class="chat-banner__body">
          <span class="chat-banner__title">Finish setup to start chatting</span>
          <ul class="chat-banner__list">
            <li v-for="item in missingSetup" :key="item.to">
              <span class="chat-banner__item-label">{{ item.label }}</span>
              <span class="chat-banner__item-hint">— {{ item.hint }}</span>
              <router-link :to="item.to" class="chat-banner__link">Configure</router-link>
            </li>
          </ul>
        </div>
      </div>

      <!-- ════ STATE A: No messages → input centered ════ -->
      <div v-if="!hasMessages" class="chat-center-state">
        <div class="chat-welcome">
          <n-text class="chat-welcome__title">Antelope Agent</n-text>
          <n-text depth="3" class="chat-welcome__sub">
            Ask about pipelines, run analyses on uploaded data, or browse storage.
          </n-text>
        </div>

        <div class="chat-input-wrap">
          <div class="chat-input-box">
            <n-input
              ref="inputRef"
              v-model:value="inputText"
              type="textarea"
              placeholder="Ask anything…"
              :autosize="{ minRows: 2, maxRows: 10 }"
              :disabled="chatStore.isStreaming"
              class="chat-textarea"
              @keydown="handleKeydown"
            />
            <div class="chat-bottom-row">
              <div class="chat-bottom-left">
                <AttachmentUploader
                  v-model:attachments="pendingAttachments"
                  :disabled="chatStore.isStreaming"
                  :reserved-names="reservedAttachmentNames"
                />
              </div>
              <div class="chat-bottom-right">
                <n-popselect
                  v-if="llmConfigured"
                  v-model:value="selectedConfigId"
                  :options="llmConfigOptions"
                  trigger="click"
                  placement="top-end"
                >
                  <button class="model-btn">
                    <span>{{ currentModelLabel }}</span>
                    <icon-park-outline-up class="model-btn__chevron" />
                  </button>
                </n-popselect>
                <button
                  v-else
                  class="model-btn model-btn--cta"
                  title="Configure an LLM provider"
                  @click="goConfigureLLM"
                >
                  <icon-park-outline-setting-two class="model-btn__icon" />
                  <span>Configure LLM</span>
                </button>
                <n-tooltip placement="top">
                  <template #trigger>
                    <div class="context-meter">
                      <n-progress
                        type="circle"
                        :percentage="contextPercent"
                        :color="contextColor"
                        :stroke-width="14"
                        :show-indicator="false"
                      />
                    </div>
                  </template>
                  {{ contextTooltip }}
                </n-tooltip>
                <n-tooltip v-if="chatStore.isStreaming" placement="top">
                  <template #trigger>
                    <button class="send-btn send-btn--stop" @click="handleStop">
                      <icon-park-outline-square />
                    </button>
                  </template>
                  Stop Antelope response
                </n-tooltip>
                <n-tooltip v-else-if="!setupComplete" placement="top">
                  <template #trigger>
                    <span class="send-btn-wrap">
                      <button class="send-btn" disabled>
                        <icon-park-outline-arrow-up />
                      </button>
                    </span>
                  </template>
                  {{ sendDisabledReason }}
                </n-tooltip>
                <button
                  v-else
                  class="send-btn"
                  :disabled="!canSend"
                  title="Send"
                  @click="handleSend"
                >
                  <icon-park-outline-arrow-up />
                </button>
              </div>
            </div>
          </div>
          <p class="chat-hint">Enter to send · Shift+Enter for new line</p>
        </div>
      </div>

      <!-- ════ STATE B: Has messages → scroll + bottom input ════ -->
      <template v-else>
        <div class="chat-header">
          <div class="chat-header__left">
            <icon-park-outline-robot class="chat-header__icon" />
            <n-text strong class="chat-header__title">
              {{ chatStore.activeConversation?.title || 'AI Chat' }}
            </n-text>
          </div>
          <div class="chat-header__right">
            <n-tooltip placement="bottom">
              <template #trigger>
                <n-button
                  size="small"
                  quaternary
                  circle
                  @click="filesPanelOpen = !filesPanelOpen"
                >
                  <template #icon>
                    <icon-park-outline-folder v-if="filesPanelOpen" />
                    <icon-park-outline-folder-close v-else />
                  </template>
                </n-button>
              </template>
              {{ filesPanelOpen ? 'Hide files' : 'Show files' }}
            </n-tooltip>
          </div>
        </div>

        <div ref="messageContainerRef" class="chat-messages" @scroll.passive="onMessagesScroll">
          <div ref="messageInnerRef" class="chat-messages__inner">
            <MessageBubble
              v-for="msg in displayedMessages"
              :key="msg.id"
              :message="msg"
            />
            <div
              v-if="chatStore.isStreaming
                && displayedMessages.at(-1)?.role !== 'assistant'
                && displayedMessages.at(-1)?.role !== 'thinking_cluster'"
              class="chat-typing"
            >
              <n-avatar round :size="30" :color="primaryColor">
                <icon-park-outline-robot />
              </n-avatar>
              <div class="chat-typing__bubble">
                <n-spin :size="16" />
                <span class="chat-typing__text">Thinking…</span>
              </div>
            </div>
          </div>
        </div>

        <div class="chat-input-area">
          <div class="chat-input-wrap">
            <div class="chat-input-box">
              <n-input
                ref="inputRef"
                v-model:value="inputText"
                type="textarea"
                placeholder="Message follow-up questions..."
                :autosize="{ minRows: 2, maxRows: 10 }"
                :disabled="chatStore.isStreaming"
                class="chat-textarea"
                @keydown="handleKeydown"
              />
              <div class="chat-bottom-row">
                <div class="chat-bottom-left">
                  <AttachmentUploader
                    v-model:attachments="pendingAttachments"
                    :disabled="chatStore.isStreaming"
                    :reserved-names="reservedAttachmentNames"
                  />
                </div>
                <div class="chat-bottom-right">
                  <n-popselect
                    v-if="llmConfigured"
                    v-model:value="selectedConfigId"
                    :options="llmConfigOptions"
                    trigger="click"
                    placement="top-end"
                  >
                    <button class="model-btn">
                      <span>{{ currentModelLabel }}</span>
                      <icon-park-outline-up class="model-btn__chevron" />
                    </button>
                  </n-popselect>
                  <button
                    v-else
                    class="model-btn model-btn--cta"
                    title="Configure an LLM provider"
                    @click="goConfigureLLM"
                  >
                    <icon-park-outline-setting-two class="model-btn__icon" />
                    <span>Configure LLM</span>
                  </button>
                  <n-tooltip placement="top">
                    <template #trigger>
                      <div class="context-meter">
                        <n-progress
                          type="circle"
                          :percentage="contextPercent"
                          :color="contextColor"
                          :stroke-width="14"
                          :show-indicator="false"
                        />
                      </div>
                    </template>
                    {{ contextTooltip }}
                  </n-tooltip>
                  <n-tooltip v-if="chatStore.isStreaming" placement="top">
                    <template #trigger>
                      <button class="send-btn send-btn--stop" @click="handleStop">
                        <icon-park-outline-square />
                      </button>
                    </template>
                    Stop Antelope response
                  </n-tooltip>
                  <n-tooltip v-else-if="!setupComplete" placement="top">
                    <template #trigger>
                      <span class="send-btn-wrap">
                        <button class="send-btn" disabled>
                          <icon-park-outline-arrow-up />
                        </button>
                      </span>
                    </template>
                    {{ sendDisabledReason }}
                  </n-tooltip>
                  <button
                    v-else
                    class="send-btn"
                    :disabled="!canSend"
                    title="Send"
                    @click="handleSend"
                  >
                    <icon-park-outline-arrow-up />
                  </button>
                </div>
              </div>
            </div>
            <p class="chat-hint">Enter to send · Shift+Enter for new line</p>
          </div>
        </div>
      </template>

    </div>

    <!-- ── Right: File browser ───────────────────────────────────── -->
    <div v-if="filesPanelOpen && hasMessages" class="chat-files">
      <FileBrowserPanel />
    </div>
  </div>
</template>

<style scoped>
.chat-page {
  display: flex;
  height: 100%;
  max-height: calc(100vh - 185px);
  overflow: hidden;
  background: var(--n-body-color);
}

.chat-sidebar {
  width: 260px;
  flex-shrink: 0;
  border-right: 1px solid var(--n-border-color, rgba(0,0,0,.09));
  background: var(--n-color, #fff);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.chat-files {
  width: 280px;
  flex-shrink: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.chat-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  min-width: 0;
  position: relative;
}

.chat-banner {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 14px;
  background: rgba(240, 160, 32, 0.10);
  color: #b86c00;
  font-size: 12px;
  border-bottom: 1px solid rgba(240, 160, 32, 0.3);
}

.chat-banner__icon {
  flex-shrink: 0;
  margin-top: 1px;
  font-size: 14px;
}

.chat-banner__body {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.chat-banner__title { font-weight: 600; }

.chat-banner__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.chat-banner__list li {
  display: flex;
  align-items: center;
  gap: 5px;
}

.chat-banner__item-hint { opacity: 0.8; }

.chat-banner__link {
  color: var(--chat-primary, #18a058);
  font-weight: 600;
  margin-left: 4px;
}

.chat-center-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 24px 20px 40px;
  gap: 28px;
  overflow: hidden;
}

.chat-welcome {
  text-align: center;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.chat-welcome__title {
  font-size: 26px;
  font-weight: 600;
  letter-spacing: -0.3px;
}

.chat-welcome__sub {
  font-size: 14px;
  line-height: 1.6;
}

.chat-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 11px 20px;
  border-bottom: 1px solid var(--n-border-color, rgba(0,0,0,.09));
  background: var(--n-color);
  flex-shrink: 0;
  min-height: 50px;
}

.chat-header__left {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--chat-primary, #18a058);
  font-size: 17px;
}

.chat-header__icon { flex-shrink: 0; }
.chat-header__title { font-size: 14px; }
.chat-header__right {
  display: flex;
  align-items: center;
  gap: 6px;
}

.chat-messages {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  /* Don't let the browser re-anchor scroll position when streamdown replaces
     nodes mid-stream — that fights our auto-follow on bursty output. */
  overflow-anchor: none;
}

.chat-messages__inner {
  max-width: 960px;
  margin: 0 auto;
  padding: 24px 20px 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.chat-typing {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 0;
}

.chat-typing__bubble {
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--n-color-embedded, rgba(0,0,0,.04));
  border: 1px solid var(--n-border-color);
  border-radius: 12px;
  border-bottom-left-radius: 3px;
  padding: 10px 16px;
}

.chat-typing__text {
  font-size: 13px;
  color: var(--n-text-color-3, #999);
}

.chat-input-area {
  flex-shrink: 0;
  padding: 14px 20px 18px;
  background: var(--n-color, #fff);
  border-top: 1px solid var(--n-border-color, rgba(0,0,0,.07));
}

.chat-input-wrap {
  width: 100%;
  max-width: 960px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.chat-input-box {
  border: 1.5px solid var(--n-border-color, rgba(0,0,0,.15));
  border-radius: 18px;
  background: var(--n-color, #fff);
  padding: 12px 14px 8px;
  transition: border-color .15s, background .15s;
}
:deep(.chat-input-box .n-input) { background: transparent !important; }

.chat-input-box:focus-within {
  border-color: var(--chat-primary, #18a058);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--chat-primary, #18a058) 15%, transparent);
}

:deep(.chat-textarea .n-input__border),
:deep(.chat-textarea .n-input__state-border) { display: none !important; }
:deep(.chat-textarea .n-input-wrapper) {
  padding: 0;
  background: transparent !important;
}
:deep(.chat-textarea.n-input--disabled .n-input-wrapper) {
  background: transparent !important;
}
:deep(.chat-textarea .n-input__textarea-el) {
  resize: none;
  background: transparent !important;
  font-size: 14px;
  line-height: 1.65;
}
:deep(.chat-textarea.n-input--disabled .n-input__textarea-el) {
  background: transparent !important;
  color: var(--n-text-color, inherit);
  -webkit-text-fill-color: var(--n-text-color, inherit);
  opacity: 1;
}

.chat-bottom-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 6px;
}

.chat-bottom-left,
.chat-bottom-right {
  display: flex;
  align-items: center;
  gap: 2px;
}

.model-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: none;
  background: transparent;
  border-radius: 8px;
  padding: 3px 7px;
  font-size: 12px;
  color: var(--n-text-color-3, #888);
  cursor: pointer;
  transition: background .15s, color .15s;
  white-space: nowrap;
}
.model-btn:hover:not(:disabled) {
  background: var(--n-color-embedded, rgba(0,0,0,.05));
  color: var(--n-text-color, #333);
}
.model-btn:disabled { opacity: .45; cursor: default; }
.model-btn__chevron { font-size: 11px; flex-shrink: 0; }
.model-btn__icon { font-size: 13px; flex-shrink: 0; }

/* When no LLM provider exists, the model pill becomes a shortcut to Settings. */
.model-btn--cta { color: var(--chat-primary, #18a058); font-weight: 600; }
.model-btn--cta:hover:not(:disabled) { color: var(--chat-primary, #18a058); }

.send-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  margin-left: 6px;
  border: none;
  border-radius: 9px;
  background: var(--chat-primary, #18a058);
  color: #fff;
  font-size: 18px;
  cursor: pointer;
  flex-shrink: 0;
  transition: opacity .15s, background .15s, border-color .15s, transform .05s;
}
/* Wrapper so the tooltip trigger still receives hover when the button inside
   is disabled (native disabled buttons don't emit mouse events). */
.send-btn-wrap { display: inline-flex; }

.send-btn:hover:not(:disabled) { opacity: .85; }
.send-btn:active:not(:disabled) { transform: scale(.93); }
.send-btn:disabled { opacity: .35; cursor: default; }

/* Stop state: outlined square button (Claude-style), not a filled accent. */
.send-btn--stop {
  background: transparent;
  border: 1.5px solid var(--n-border-color, rgba(0,0,0,.2));
  color: var(--n-text-color-2, #555);
  font-size: 14px;
}
.send-btn--stop:hover:not(:disabled) {
  opacity: 1;
  border-color: var(--n-error-color, #d03050);
  color: var(--n-error-color, #d03050);
}

/* Context-usage circular meter occupying the old mic slot. */
.context-meter {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  cursor: default;
}
:deep(.context-meter .n-progress) { width: 20px; }

.chat-hint {
  text-align: center;
  font-size: 11px;
  color: var(--n-text-color-3, #aaa);
  margin: 0;
  line-height: 1;
}
</style>
