<script setup>
import { fetchJobDetails } from '@/api/job'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true,
  },
  type: {
    type: String,
    default: 'add',
  },
  modalData: {
    type: Object,
    default: null,
  },
})

const emit = defineEmits(['update:visible'])

const loading = ref(false)
const detail = ref(null)

const modalVisible = computed({
  get() {
    return props.visible
  },
  set(visible) {
    closeModal(visible)
  },
})

function closeModal(visible = false) {
  emit('update:visible', visible)
}

const title = computed(() => {
  const titles = {
    add: 'New Job',
    edit: 'Job Details',
  }
  return titles[props.type]
})

const tagType = {
  submitted: 'default',
  dispatch_success: 'info',
  pending: 'warning',
  running: 'info',
  completed: 'success',
  failed: 'error',
}

const formattedParams = computed(() => {
  const params = detail.value?.params
  if (params === null || params === undefined || params === '') {
    return 'No input parameters recorded for this job.'
  }
  try {
    const parsed = typeof params === 'string' ? JSON.parse(params) : params
    return JSON.stringify(parsed, null, 2)
  } catch {
    return String(params)
  }
})

async function loadDetails() {
  if (!props.modalData?.id) {
    return
  }
  loading.value = true
  detail.value = null
  try {
    const res = await fetchJobDetails(props.modalData.id)
    if (res.isSuccess) {
      detail.value = res.data.job
    } else {
      window.$message.error('get job details failed')
    }
  } catch {
    window.$message.error('get job details failed')
  } finally {
    loading.value = false
  }
}

async function copyParams() {
  try {
    await navigator.clipboard.writeText(formattedParams.value)
    window.$message.success('Parameters copied to clipboard')
  } catch {
    window.$message.error('copy failed')
  }
}

watch(
  () => props.visible,
  (newValue) => {
    if (!newValue) {
      return
    }
    loadDetails()
  },
)
</script>

<template>
  <n-modal
    v-model:show="modalVisible"
    :mask-closable="false"
    preset="card"
    :title="title"
    class="w-700px"
    :segmented="{
      content: true,
      action: true,
    }"
  >
    <n-spin :show="loading">
      <n-descriptions
        v-if="detail"
        label-placement="left"
        bordered
        :column="2"
        size="small"
      >
        <n-descriptions-item label="Pipeline Name">
          {{ detail.pipeline_name }}
        </n-descriptions-item>
        <n-descriptions-item label="Version">
          {{ detail.pipeline_version }}
        </n-descriptions-item>
        <n-descriptions-item label="Status">
          <n-tag :type="tagType[detail.status] || 'default'" size="small">
            {{ detail.status }}
          </n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="Create Time">
          {{ detail.created_at ? new Date(detail.created_at).toLocaleString() : '-' }}
        </n-descriptions-item>
        <n-descriptions-item label="Dispatch ID" :span="2">
          {{ detail.dispatch_id || '-' }}
        </n-descriptions-item>
      </n-descriptions>

      <div class="params-header">
        <span class="params-title">Input Parameters</span>
        <n-button size="tiny" secondary @click="copyParams">
          <template #icon>
            <icon-park-outline-copy />
          </template>
          Copy
        </n-button>
      </div>
      <pre class="params-content">{{ formattedParams }}</pre>
    </n-spin>

    <template #action>
      <n-space justify="center">
        <n-button @click="closeModal()"> Close </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<style scoped>
.params-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 16px 0 8px;
}

.params-title {
  font-weight: 600;
}

/* Light by default so the params are legible in light mode; the original dark
   surface is preserved under `html.dark` (the class VueUse's useColorMode sets
   on <html>). Palette matches the chat markdown code blocks for consistency. */
.params-content {
  max-height: 360px;
  overflow: auto;
  margin: 0;
  padding: 12px;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.5;
  color: #1f2328;
  background-color: #f6f8fa;
  border: 1px solid #d0d7de;
  border-radius: 4px;
  white-space: pre-wrap;
  word-wrap: break-word;
}

/* Dark mode: restore the original dark code surface, unchanged. */
html.dark .params-content {
  color: #e0e0e0;
  background-color: #1e1e1e;
  border-color: transparent;
}
</style>
