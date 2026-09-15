<script setup>
import { ref, computed, onMounted } from 'vue'
import { fetchUploadURL } from '@/api/storage'
import { fetchAgentWorkspaceConfig } from '@/api/agent'

/**
 * AttachmentUploader manages a small queue of files the user wants to send
 * with the next chat message. Files are uploaded immediately on selection
 * (PUT to a presigned URL inside the user's workspace bucket); only
 * lightweight refs travel with the message.
 *
 * v-model:attachments  → [{bucket, key, name, mime_type, size}, ...]
 */
const props = defineProps({
  attachments: { type: Array, default: () => [] },
  disabled: { type: Boolean, default: false },
  // Filenames already attached in EARLIER turns of this conversation. A new
  // attachment with one of these names would collide on the staged path
  // (work/inputs/<name>) and silently overwrite the previous file server-side,
  // so we block it here. Case-sensitive, matching the sandbox filesystem.
  reservedNames: { type: Array, default: () => [] },
})
const emit = defineEmits(['update:attachments'])

const fileInput = ref(null)
const bucket = ref('')
const bucketLoading = ref(true)
const uploading = ref(false)
const errorMsg = ref('')

const hasBucket = computed(() => !!bucket.value)

async function loadBucket() {
  bucketLoading.value = true
  try {
    const { isSuccess, data } = await fetchAgentWorkspaceConfig()
    if (isSuccess && data?.configured && data.bucket) {
      bucket.value = data.bucket
    }
  } catch { /* ignore */ } finally {
    bucketLoading.value = false
  }
}

onMounted(loadBucket)

function openPicker() {
  if (props.disabled || !hasBucket.value) return
  fileInput.value?.click()
}

async function onFiles(e) {
  const files = Array.from(e.target.files || [])
  e.target.value = '' // allow re-selecting the same file
  if (!files.length) return

  errorMsg.value = ''

  // Fence cross-turn name collisions: a file whose name was attached in an
  // earlier turn maps to the same staged path and would overwrite the prior
  // one (and desync the paths the model already saw). Block it so the user
  // renames or drops it. Duplicates within THIS turn are fine — the backend
  // disambiguates them (data.csv, data_2.csv) — so we only check prior turns.
  const reserved = new Set(props.reservedNames)
  const accepted = []
  const blocked = []
  for (const file of files) {
    if (reserved.has(file.name)) blocked.push(file.name)
    else accepted.push(file)
  }
  if (blocked.length) {
    const msg = `Already attached earlier in this conversation: ${blocked.join(', ')}.`
      + ' Rename the file before re-attaching.'
    errorMsg.value = msg
    window.$message?.warning(msg)
  }
  if (!accepted.length) return

  uploading.value = true
  const refs = [...props.attachments]
  try {
    for (const file of accepted) {
      const ref = await uploadOne(file)
      if (ref) refs.push(ref)
    }
    emit('update:attachments', refs)
  } finally {
    uploading.value = false
  }
}

async function uploadOne(file) {
  // Namespace inside the bucket so chat uploads stay tidy and don't
  // collide with the agent's own artifact tree (agent/{sessionID}/...).
  const key = `chat-uploads/${Date.now()}-${file.name}`
  try {
    const { isSuccess, data } = await fetchUploadURL(bucket.value, key)
    if (!isSuccess || !data?.urls?.uploadUrl) {
      errorMsg.value = `Failed to get upload URL for ${file.name}`
      return null
    }
    const url = data.urls.uploadUrl
    const res = await fetch(url, {
      method: 'PUT',
      body: file,
      headers: file.type ? { 'Content-Type': file.type } : {},
    })
    if (!res.ok) {
      errorMsg.value = `Upload failed for ${file.name} (HTTP ${res.status})`
      return null
    }
    return {
      bucket: bucket.value,
      key,
      name: file.name,
      mime_type: file.type || '',
      size: file.size,
    }
  } catch (e) {
    errorMsg.value = `Upload failed: ${e.message}`
    return null
  }
}

function removeAttachment(idx) {
  const next = [...props.attachments]
  next.splice(idx, 1)
  emit('update:attachments', next)
}

function formatBytes(n) {
  if (!n) return ''
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}
</script>

<template>
  <div class="att-uploader">
    <input
      ref="fileInput"
      type="file"
      multiple
      hidden
      @change="onFiles"
    >

    <n-tooltip placement="top">
      <template #trigger>
        <button
          class="att-uploader__btn"
          :disabled="disabled || !hasBucket || uploading"
          tabindex="-1"
          @click="openPicker"
        >
          <n-spin v-if="uploading || bucketLoading" :size="14" />
          <icon-park-outline-paperclip v-else />
        </button>
      </template>
      <template v-if="bucketLoading">Loading workspace…</template>
      <template v-else-if="!hasBucket">
        Configure a workspace bucket in Settings → Agent Workspace
      </template>
      <template v-else>Attach file (uploads to {{ bucket }})</template>
    </n-tooltip>

    <!-- Attachment chips -->
    <div v-if="attachments.length" class="att-uploader__chips">
      <div
        v-for="(a, i) in attachments"
        :key="a.key"
        class="att-uploader__chip"
      >
        <icon-park-outline-file-code />
        <span class="att-uploader__name">{{ a.name }}</span>
        <span v-if="a.size" class="att-uploader__size">{{ formatBytes(a.size) }}</span>
        <button class="att-uploader__remove" @click="removeAttachment(i)">
          <icon-park-outline-close-small />
        </button>
      </div>
    </div>

    <div v-if="errorMsg" class="att-uploader__error">{{ errorMsg }}</div>
  </div>
</template>

<style scoped>
.att-uploader {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}

.att-uploader__btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: none;
  background: transparent;
  border-radius: 8px;
  color: var(--n-text-color-3, #999);
  font-size: 16px;
  cursor: pointer;
  transition: background .15s, opacity .15s;
  padding: 0;
}

.att-uploader__btn:not(:disabled):hover {
  background: var(--n-color-embedded, rgba(0,0,0,.05));
  color: var(--n-text-color, #333);
}

.att-uploader__btn:disabled {
  opacity: .45;
  cursor: not-allowed;
}

.att-uploader__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.att-uploader__chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  padding: 2px 4px 2px 8px;
  border-radius: 999px;
  background: var(--n-color-embedded, rgba(0,0,0,.05));
  color: var(--n-text-color-2);
  border: 1px solid var(--n-border-color, rgba(0,0,0,.06));
}

.att-uploader__name { font-weight: 500; }

.att-uploader__size {
  color: var(--n-text-color-3);
  font-size: 10px;
}

.att-uploader__remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: transparent;
  color: var(--n-text-color-3);
  cursor: pointer;
  padding: 2px;
  border-radius: 999px;
}

.att-uploader__remove:hover {
  background: rgba(0,0,0,.08);
  color: #d03050;
}

.att-uploader__error {
  font-size: 11px;
  color: #d03050;
  flex-basis: 100%;
}
</style>
