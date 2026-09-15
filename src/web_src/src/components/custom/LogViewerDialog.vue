<script setup lang="ts">
import { NModal, NCard, NSpace, NButton, NSelect, NSpin, NEmpty } from 'naive-ui'
import { ref, computed, watch, onBeforeUnmount, nextTick } from 'vue'
import { useSSE } from 'alova/client'
import { fetchStreamJobLog } from '@/api/job'
import AnsiToHtml from 'ansi-to-html'

// Initialize ANSI to HTML converter
const ansiConverter = new AnsiToHtml({
  fg: '#e0e0e0',
  bg: '#1e1e1e',
  newline: false,
  escapeXML: true,
  stream: false,
})

interface LogViewerDialogProps {
  visible: boolean
  allocId: string
  taskName?: string
}

const props = withDefaults(defineProps<LogViewerDialogProps>(), {
  taskName: 'task',
})

const emit = defineEmits<{
  'update:visible': [value: boolean]
}>()

// Log type selection
const logType = ref<'stdout' | 'stderr'>('stdout')
const logTypes = [
  { label: 'stdout', value: 'stdout' },
  { label: 'stderr', value: 'stderr' },
]

// Log data
const logs = ref<string[]>([])
const autoScroll = ref(false)
const logScrollRef = ref<HTMLElement | null>(null)
const streamEnded = ref(false) // Track if stream has ended naturally

// SSE connection using Alova
const {
  data,
  readyState,
  send,
  close,
  onMessage,
  onError,
  onOpen
} = useSSE(
  () => fetchStreamJobLog(props.allocId, logType.value),
  {
    immediate: false, // We'll control when to start
    initialData: null,
    interceptByGlobalResponded: false, // Handle data manually
    reconnectionTime: 0, // DISABLE AUTO-RECONNECTION
  }
)

// Connection status based on readyState
const isConnected = computed(() => readyState.value === 1) // 1 = OPEN
const connectionStatus = computed(() => {
  if (streamEnded.value) return 'Stream Ended'
  switch (readyState.value) {
    case 0: return 'Connecting...'
    case 1: return 'Connected'
    case 2: return 'Disconnected'
    default: return 'Unknown'
  }
})

// Handle SSE messages
onMessage((event) => {
  try {
    const parsedData = JSON.parse(event.data)

    switch (parsedData.type) {
      case 'connected':
        streamEnded.value = false
        window.$message?.success(parsedData.message || 'Log stream connected')
        break

      case 'log':
        if (parsedData.data) {
          logs.value.push(parsedData.data)
        }
        break

      case 'error':
        window.$message?.error(parsedData.error || 'Stream error occurred')
        streamEnded.value = true
        close() // Close connection on error
        break

      case 'stream_ended':
        streamEnded.value = true
        window.$message?.info(parsedData.message || 'Log stream ended')
        close() // Close connection when stream ends
        break

      case 'allocation_terminated':
        streamEnded.value = true
        window.$message?.warning(parsedData.message || 'Allocation terminated')
        close() // Close connection when allocation terminates
        break

      default:
        console.log('Unknown message type:', parsedData)
    }
  } catch (error) {
    console.error('Failed to parse SSE message:', error, 'Raw data:', event.data)
  }
})

// Handle SSE errors
onError((event) => {
  console.error('SSE Error:', event.error)
  window.$message?.error('Log stream connection failed')
  streamEnded.value = true
  close() // Close connection on error
})

// Handle SSE open
onOpen((event) => {
  console.log('SSE connection opened')
  streamEnded.value = false
})

// Start SSE connection
const startSSEConnection = () => {
  if (!props.allocId) return

  logs.value = []
  streamEnded.value = false
  autoScroll.value = false
  send()
}

// Stop SSE connection
const stopSSEConnection = () => {
  close()
  streamEnded.value = true
  stopAutoScrollInterval()
}

// Scroll to the bottom of the log area
const scrollToBottom = () => {
  nextTick(() => {
    if (logScrollRef.value) {
      logScrollRef.value.scrollTop = logScrollRef.value.scrollHeight
    }
  })
}

// Interval handle for continuous auto-scroll (tail -f behaviour)
let autoScrollInterval: ReturnType<typeof setInterval> | null = null

const startAutoScrollInterval = () => {
  if (autoScrollInterval) return
  autoScrollInterval = setInterval(() => {
    if (autoScroll.value && logScrollRef.value) {
      logScrollRef.value.scrollTop = logScrollRef.value.scrollHeight
    }
  }, 100)
}

const stopAutoScrollInterval = () => {
  if (autoScrollInterval) {
    clearInterval(autoScrollInterval)
    autoScrollInterval = null
  }
}

// Computed
const statusColor = computed(() => {
  if (streamEnded.value) return '#909399'
  if (isConnected.value) return '#18a058'
  if (connectionStatus.value.includes('Error')) return '#d03050'
  return '#909399'
})

// Convert ANSI color codes to HTML
const logContentHtml = computed(() => {
  const rawLog = logs.value.join('')
  try {
    return ansiConverter.toHtml(rawLog)
  } catch (error) {
    console.error('Failed to convert ANSI to HTML:', error)
    return rawLog
  }
})

// Watch the rendered HTML — fires whenever logs change (logs.push mutates the
// array, which logContentHtml tracks). flush:'post' guarantees the DOM has
// been updated so scrollHeight is accurate.
watch(logContentHtml, () => {
  if (autoScroll.value) {
    scrollToBottom()
  }
}, { flush: 'post' })

// Watch log type changes
watch(logType, () => {
  if (props.visible && props.allocId) {
    stopSSEConnection()
    // Small delay to ensure connection is fully closed
    setTimeout(() => {
      startSSEConnection()
    }, 100)
  }
})

// Watch visibility
watch(
  () => props.visible,
  (newVal) => {
    if (newVal && props.allocId) {
      startSSEConnection()
    } else {
      stopSSEConnection()
    }
  },
)

// Cleanup on unmount
onBeforeUnmount(() => {
  stopSSEConnection()
  stopAutoScrollInterval()
})

// Modal actions
const handleClose = () => {
  stopSSEConnection()
  emit('update:visible', false)
}

const handleClearLogs = () => {
  logs.value = []
}

const handleDownloadLogs = () => {
  const logContent = logs.value.join('')
  const blob = new Blob([logContent], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${props.allocId}-${logType.value}-${Date.now()}.log`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
  window.$message?.success('Logs downloaded successfully')
}

const toggleAutoScroll = () => {
  autoScroll.value = !autoScroll.value
  if (autoScroll.value) {
    startAutoScrollInterval()
    scrollToBottom()
  } else {
    stopAutoScrollInterval()
  }
}
</script>

<template>
  <NModal
    :show="visible"
    :mask-closable="false"
    preset="card"
    title="Job Logs"
    style="width: 90%; max-width: 1400px; height: 80vh"
    :content-style="{ paddingBottom: '8px', display: 'flex', flexDirection: 'column', minHeight: 0 }"
    @update:show="handleClose"
  >
    <template #header-extra>
      <NSpace align="center" :size="12">
        <div class="status-indicator" :style="{ backgroundColor: statusColor }" />
        <span class="status-text">{{ connectionStatus }}</span>
      </NSpace>
    </template>

    <div class="modal-content-wrapper">
      <!-- Toolbar -->
      <div class="toolbar-section">
        <NSpace justify="space-between" align="center">
          <NSpace align="center">
            <span class="toolbar-label">Log Type:</span>
            <NSelect v-model:value="logType" :options="logTypes" style="width: 120px" size="small" />

            <span class="toolbar-label ml-4">Alloc ID:</span>
            <span class="alloc-id">{{ allocId }}</span>
          </NSpace>

          <NSpace>
            <NButton
              size="small"
              :type="autoScroll ? 'primary' : 'default'"
              @click="toggleAutoScroll"
            >
              <template #icon>
                <icon-park-outline-pause v-if="autoScroll" />
                <icon-park-outline-arrow-down v-else />
              </template>
              {{ autoScroll ? 'Pause' : 'Auto Scroll' }}
            </NButton>

            <NButton size="small" secondary @click="handleClearLogs">
              <template #icon>
                <icon-park-outline-delete />
              </template>
              Clear
            </NButton>

            <NButton size="small" secondary @click="handleDownloadLogs" :disabled="logs.length === 0">
              <template #icon>
                <icon-park-outline-download />
              </template>
              Download
            </NButton>

            <NButton size="small" secondary @click="scrollToBottom">
              <template #icon>
                <icon-park-outline-to-bottom />
              </template>
              Bottom
            </NButton>
          </NSpace>
        </NSpace>
      </div>

      <!-- Log viewer with fixed height -->
      <div class="log-viewer-section">
        <div ref="logScrollRef" class="log-scrollbar">
          <div v-if="logs.length === 0" class="empty-container">
            <NSpin v-if="isConnected && !streamEnded" size="large">
              <template #description> Waiting for logs... </template>
            </NSpin>
            <NEmpty v-else description="No logs available" />
          </div>
          <pre v-else class="log-content" v-html="logContentHtml"></pre>
        </div>
      </div>

      <!-- Footer inside body so it stays within the bounded wrapper -->
      <div class="modal-footer">
        <NButton @click="handleClose" class="close-btn">
          <template #icon>
            <icon-park-outline-close-one class="close-icon" />
          </template>
          Close
        </NButton>
      </div>
    </div>

  </NModal>
</template>

<style scoped>
.modal-content-wrapper {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.toolbar-section {
  flex-shrink: 0;
  margin-bottom: 12px;
}

.log-viewer-section {
  flex: 1;
  min-height: 0; /* Important for flex child to allow scrolling */
  background-color: #1e1e1e;
  border-radius: 3px;
  overflow: hidden;
}

.log-scrollbar {
  height: 100%;
  overflow-y: auto;
  overflow-x: auto;
}

.status-indicator {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
}

.status-text {
  font-size: 14px;
  color: #606266;
}

.toolbar-label {
  font-size: 14px;
  color: #606266;
  font-weight: 500;
}

.alloc-id {
  font-family: 'Courier New', monospace;
  font-size: 13px;
  color: #303133;
  background-color: #f4f4f5;
  padding: 2px 8px;
  border-radius: 4px;
}

.log-content {
  margin: 0;
  padding: 16px;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.5;
  color: #e0e0e0;
  background-color: #1e1e1e;
  white-space: pre-wrap;
  word-wrap: break-word;
  overflow-wrap: break-word;
  min-height: 100%;
}

.empty-container {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  min-height: 400px;
}

.ml-4 {
  margin-left: 16px;
}

.modal-footer {
  flex-shrink: 0;
  padding-top: 6px;
  display: flex;
  justify-content: flex-end;
}

.close-icon {
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
