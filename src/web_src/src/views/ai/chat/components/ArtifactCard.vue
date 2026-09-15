<script setup>
import { computed, ref, onMounted, watch } from 'vue'
import { useChatStore } from '@/stores/chat'
import { fetchDownloadArtifact } from '@/api/agent'
import AgentEventCard from './AgentEventCard.vue'

const props = defineProps({
  message: { type: Object, required: true },
})

const chatStore = useChatStore()
const downloadUrl = ref(props.message.download_url || '')
const loading = ref(false)
const error = ref('')

// pending = the file is still uploading (end-of-turn harvest). The card shows
// a skeleton until the real 'artifact' event replaces it in place.
const pending = computed(() => !!props.message.pending)
const name = computed(() => props.message.name || props.message.saved_as || 'artifact')
const mime = computed(() => props.message.mime_type || '')
const isImage = computed(() => mime.value.startsWith('image/'))
const isCSV = computed(() => mime.value === 'text/csv' || (name.value || '').endsWith('.csv'))
const size = computed(() => props.message.size_bytes)

const subtitle = computed(() => {
  const parts = []
  if (mime.value) parts.push(mime.value)
  if (size.value) parts.push(formatBytes(size.value))
  if (typeof props.message.version === 'number') parts.push(`v${props.message.version}`)
  if (pending.value) parts.push('syncing…')
  return parts.join(' · ')
})

function formatBytes(n) {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`
  return `${(n / 1024 / 1024 / 1024).toFixed(1)} GB`
}

async function resolveUrl() {
  if (downloadUrl.value) return
  if (!chatStore.activeConversationId) return
  loading.value = true
  try {
    const ver = typeof props.message.version === 'number' ? props.message.version : -1
    const key = props.message.saved_as || props.message.name
    const { isSuccess, data } = await fetchDownloadArtifact(
      chatStore.activeConversationId,
      key,
      ver,
    )
    if (isSuccess && data?.url) {
      downloadUrl.value = data.url
    } else {
      error.value = 'failed to load'
    }
  } catch (e) {
    error.value = e.message || 'failed to load'
  } finally {
    loading.value = false
  }
}

// Eagerly resolve for image artifacts so we can preview inline. Skip while
// pending — there is nothing to download yet.
onMounted(() => {
  if (!pending.value && isImage.value && !downloadUrl.value) resolveUrl()
})

// When a skeleton flips to a ready image (its upload completed), resolve the
// preview URL — onMounted already ran for this reused component instance.
watch(
  () => props.message.pending,
  (now, was) => {
    if (was && !now && isImage.value && !downloadUrl.value) resolveUrl()
  },
)
</script>

<template>
  <AgentEventCard
    tone="info"
    icon="icon-park-outline-folder-success"
    :title="name"
    :subtitle="subtitle"
    :collapsible="false"
  >
    <!-- Pending: still uploading — show a shimmering placeholder. -->
    <div v-if="pending" class="artifact__pending">
      <n-skeleton height="60px" width="100%" :sharp="false" />
      <span class="artifact__pending-label">
        <n-spin :size="12" /> Syncing to storage…
      </span>
    </div>

    <!-- Image preview -->
    <div v-else-if="isImage" class="artifact__preview">
      <img v-if="downloadUrl" :src="downloadUrl" :alt="name" />
      <n-spin v-else-if="loading" :size="20" />
      <span v-else class="artifact__error">{{ error }}</span>
    </div>

    <!-- CSV / file row -->
    <div v-else class="artifact__row">
      <span class="artifact__meta">{{ isCSV ? 'CSV file' : 'File' }}</span>
      <n-button
        v-if="downloadUrl"
        size="tiny"
        tag="a"
        :href="downloadUrl"
        target="_blank"
        download
      >
        Download
      </n-button>
      <n-button v-else size="tiny" :loading="loading" @click="resolveUrl">
        Get download URL
      </n-button>
    </div>
  </AgentEventCard>
</template>

<style scoped>
.artifact__pending {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.artifact__pending-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--n-text-color-3, #888);
}

.artifact__preview {
  display: flex;
  justify-content: center;
  align-items: center;
  max-height: 320px;
  overflow: hidden;
  border-radius: 6px;
  background: rgba(127,127,127,.05);
  padding: 4px;
}

.artifact__preview img {
  max-width: 100%;
  max-height: 312px;
  object-fit: contain;
  border-radius: 4px;
}

.artifact__row {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
}

.artifact__meta {
  color: var(--n-text-color-3);
}

.artifact__error {
  font-size: 12px;
  color: #d03050;
}
</style>
