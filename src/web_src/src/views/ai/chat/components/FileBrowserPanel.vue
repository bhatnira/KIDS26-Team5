<script setup>
import { computed, ref } from 'vue'
import { useChatStore } from '@/stores/chat'
import { fetchDownloadArtifact } from '@/api/agent'
import FilePreviewDialog from './FilePreviewDialog.vue'

const chatStore = useChatStore()

const refreshing = ref(false)
const items = computed(() => chatStore.artifacts)

const previewOpen = ref(false)
const previewItem = ref(null)
const previewUrl = ref('')
const previewLoading = ref(false)
const previewVersion = ref(null)

async function refresh() {
  refreshing.value = true
  try {
    await chatStore.loadArtifacts()
  } finally {
    refreshing.value = false
  }
}

async function resolveUrl(item, version) {
  if (!chatStore.activeConversationId) return ''
  const ver = typeof version === 'number' ? version : item.latest_version
  const { isSuccess, data } = await fetchDownloadArtifact(
    chatStore.activeConversationId,
    item.name,
    ver,
  )
  if (isSuccess && data?.url) return data.url
  return ''
}

async function openPreview(item) {
  previewItem.value = item
  previewVersion.value =
    typeof item.latest_version === 'number'
      ? item.latest_version
      : (item.versions?.[item.versions.length - 1] ?? null)
  previewUrl.value = ''
  previewOpen.value = true
  previewLoading.value = true
  try {
    previewUrl.value = await resolveUrl(item, previewVersion.value)
  } finally {
    previewLoading.value = false
  }
}

// Switch the previewed version: re-resolve the URL so the dialog reloads
// the content for the chosen version.
async function changeVersion(version) {
  previewVersion.value = version
  previewUrl.value = ''
  previewLoading.value = true
  try {
    previewUrl.value = await resolveUrl(previewItem.value, version)
  } finally {
    previewLoading.value = false
  }
}

function downloadPreview() {
  download(previewItem.value, previewVersion.value)
}

async function download(item, version) {
  const url = await resolveUrl(item, version)
  if (url) window.open(url, '_blank', 'noopener')
}

function formatVersions(item) {
  return item.versions?.length
    ? `${item.versions.length} version${item.versions.length > 1 ? 's' : ''}`
    : ''
}

function fileIcon(item) {
  const n = (item.name || '').toLowerCase()
  const ext = n.includes('.') ? n.slice(n.lastIndexOf('.') + 1) : ''
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'avif'].includes(ext)) {
    return 'icon-park-outline-pic'
  }
  if (['csv', 'tsv', 'xlsx'].includes(ext)) return 'icon-park-outline-data'
  if (['md', 'markdown', 'txt', 'log', 'rst'].includes(ext)) return 'icon-park-outline-file-text'
  if (['json', 'yaml', 'yml', 'toml', 'xml'].includes(ext)) return 'icon-park-outline-code'
  if (['py', 'r', 'js', 'ts', 'go', 'rs', 'sh', 'bash', 'java', 'c', 'cpp'].includes(ext)) {
    return 'icon-park-outline-file-code'
  }
  if (['pdf'].includes(ext)) return 'icon-park-outline-file-pdf'
  return 'icon-park-outline-file-code'
}
</script>

<template>
  <div class="fb-panel">
    <div class="fb-panel__header">
      <icon-park-outline-folder />
      <span class="fb-panel__title">Session files</span>
      <span class="fb-panel__spacer" />
      <n-button quaternary circle size="tiny" :loading="refreshing" @click="refresh">
        <template #icon><icon-park-outline-refresh /></template>
      </n-button>
    </div>

    <div v-if="!chatStore.activeConversationId" class="fb-panel__empty">
      Open a conversation to see its files.
    </div>
    <div v-else-if="!items.length" class="fb-panel__empty">
      No artifacts yet. The agent will save outputs here as it runs.
    </div>

    <ul v-else class="fb-panel__list">
      <li v-for="item in items" :key="item.name" class="fb-panel__item">
        <button class="fb-panel__row" @click="openPreview(item)">
          <component :is="fileIcon(item)" class="fb-panel__file-icon" />
          <div class="fb-panel__item-meta">
            <span class="fb-panel__name">{{ item.name }}</span>
            <span v-if="formatVersions(item)" class="fb-panel__sub">{{ formatVersions(item) }}</span>
          </div>
          <span
            class="fb-panel__download"
            title="Download"
            @click.stop="download(item)"
          >
            <icon-park-outline-download />
          </span>
        </button>
      </li>
    </ul>

    <FilePreviewDialog
      v-model:show="previewOpen"
      :item="previewItem"
      :url="previewUrl"
      :version="previewVersion"
      @update:version="changeVersion"
      @download="downloadPreview"
    />
  </div>
</template>

<style scoped>
.fb-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--n-color, #fff);
  border-left: 1px solid var(--n-border-color, rgba(0,0,0,.08));
}

.fb-panel__header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 11px 14px;
  border-bottom: 1px solid var(--n-border-color, rgba(0,0,0,.08));
  font-size: 13px;
  font-weight: 600;
  color: var(--n-text-color, #333);
  min-height: 50px;
  flex-shrink: 0;
}

.fb-panel__spacer { flex: 1; }

.fb-panel__empty {
  padding: 24px 16px;
  text-align: center;
  font-size: 12px;
  color: var(--n-text-color-3, #888);
  line-height: 1.5;
}

.fb-panel__list {
  list-style: none;
  margin: 0;
  padding: 6px 6px 16px;
  overflow-y: auto;
}

.fb-panel__item {
  border-radius: 8px;
}

.fb-panel__row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  width: 100%;
  border: none;
  background: transparent;
  text-align: left;
  font: inherit;
  color: inherit;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.15s;
}

.fb-panel__row:hover {
  background: var(--n-color-embedded, rgba(0,0,0,.04));
}

.fb-panel__file-icon {
  flex-shrink: 0;
  color: var(--n-text-color-2, #666);
}

.fb-panel__item-meta {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}

.fb-panel__name {
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.fb-panel__sub {
  font-size: 10px;
  color: var(--n-text-color-3);
}

.fb-panel__download {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 4px;
  color: var(--n-text-color-3, #999);
  transition: background 0.15s, color 0.15s;
}

.fb-panel__download:hover {
  background: var(--n-color-embedded, rgba(0,0,0,.07));
  color: var(--n-text-color, #333);
}
</style>
