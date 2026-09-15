<script setup>
import { h, ref, computed, onMounted } from 'vue'
import { NButton, NTag, NSpace, NPopconfirm, useMessage } from 'naive-ui'
import { useBoolean } from '@/hooks'
import { fetchApiKeys, createApiKey, revokeApiKey } from '@/api/apikey'

const message = useMessage()

const { bool: loading, setTrue: startLoading, setFalse: endLoading } = useBoolean(true)
const { bool: saving, setTrue: startSaving, setFalse: endSaving } = useBoolean(false)
const { bool: showModal, setTrue: openModal, setFalse: closeModal } = useBoolean(false)
const { bool: showReveal, setTrue: openReveal, setFalse: closeReveal } = useBoolean(false)

const items = ref([])
const formRef = ref(null)

const expireOptions = [
  { label: '30 days', value: 30 },
  { label: '60 days', value: 60 },
  { label: '90 days', value: 90 },
  { label: '180 days', value: 180 },
  { label: '365 days', value: 365 },
  { label: 'Custom…', value: 'custom' },
]

const defaultForm = () => ({ name: '', expire: 30, customDays: 30 })
const form = ref(defaultForm())

const isCustom = computed(() => form.value.expire === 'custom')

// The raw key is shown exactly once after creation.
const newKey = ref('')

const formRules = {
  name: { required: true, message: 'Name is required', trigger: ['input', 'blur'] },
}

function fmtDate(s) {
  if (!s) return '—'
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString()
}

const columns = [
  { title: 'Name', key: 'name' },
  {
    title: 'Key',
    key: 'prefix',
    render: (row) => h('code', null, row.prefix || '—'),
  },
  { title: 'Created', key: 'created_at', render: (row) => fmtDate(row.created_at) },
  {
    title: 'Expires',
    key: 'expires_at',
    render: (row) =>
      h(NSpace, { align: 'center', size: 'small' }, () => [
        fmtDate(row.expires_at),
        row.expired
          ? h(NTag, { type: 'error', size: 'small' }, () => 'Expired')
          : null,
      ]),
  },
  {
    title: 'Actions',
    key: 'actions',
    align: 'center',
    width: 140,
    render: (row) =>
      h(
        NPopconfirm,
        { onPositiveClick: () => handleRevoke(row.id) },
        {
          trigger: () =>
            h(NButton, { size: 'small', type: 'error', secondary: true }, () => 'Revoke'),
          default: () => 'Revoke this API key? Applications using it will stop working.',
        }
      ),
  },
]

async function load() {
  startLoading()
  try {
    const { isSuccess, data } = await fetchApiKeys()
    if (isSuccess) items.value = data?.items || []
  } finally {
    endLoading()
  }
}

onMounted(load)

function handleAdd() {
  form.value = defaultForm()
  openModal()
}

async function handleGenerate() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  const expireDays = isCustom.value ? Number(form.value.customDays) : Number(form.value.expire)
  if (!expireDays || expireDays < 1 || expireDays > 365) {
    message.warning('Expiration must be between 1 and 365 days')
    return
  }
  startSaving()
  try {
    const { isSuccess, data, message: msg } = await createApiKey({
      name: form.value.name.trim(),
      expire_days: expireDays,
    })
    if (isSuccess) {
      closeModal()
      newKey.value = data?.key || ''
      openReveal()
      await load()
    } else {
      message.error(msg || 'Failed to generate API key')
    }
  } finally {
    endSaving()
  }
}

async function handleRevoke(id) {
  const { isSuccess, message: msg } = await revokeApiKey(id)
  if (isSuccess) {
    message.success('API key revoked')
    await load()
  } else {
    message.error(msg || 'Failed to revoke API key')
  }
}

async function copyKey() {
  try {
    await navigator.clipboard.writeText(newKey.value)
    message.success('Copied to clipboard')
  } catch {
    message.error('Copy failed — please select and copy manually')
  }
}

function dismissReveal() {
  newKey.value = ''
  closeReveal()
}
</script>

<template>
  <div>
    <n-space vertical size="large">
      <n-alert type="info" title="API Keys">
        Use an API key to call the Antelope API programmatically. Send it as a Bearer token:
        <code>Authorization: Bearer &lt;key&gt;</code>. A key carries your identity and
        permissions. It is shown only once at creation — store it securely.
      </n-alert>

      <n-card title="Your API Keys">
        <template #header-extra>
          <n-button type="primary" @click="handleAdd">
            <template #icon><icon-park-outline-add /></template>
            Generate Key
          </n-button>
        </template>

        <n-spin :show="loading">
          <n-empty v-if="!items.length && !loading" description="No API keys yet">
            <template #extra>
              <n-button type="primary" @click="handleAdd">
                <template #icon><icon-park-outline-add /></template>
                Generate Key
              </n-button>
            </template>
          </n-empty>
          <n-data-table
            v-else
            :columns="columns"
            :data="items"
            :bordered="false"
            :pagination="false"
          />
        </n-spin>
      </n-card>
    </n-space>

    <!-- Generate modal -->
    <n-modal
      v-model:show="showModal"
      title="Generate API Key"
      preset="card"
      style="width: 480px"
      :mask-closable="false"
    >
      <n-form
        ref="formRef"
        :model="form"
        :rules="formRules"
        label-placement="left"
        label-width="120"
        require-mark-placement="right-hanging"
      >
        <n-form-item label="Name" path="name">
          <n-input v-model:value="form.name" placeholder="e.g., CI pipeline" />
        </n-form-item>
        <n-form-item label="Expiration" path="expire">
          <n-select v-model:value="form.expire" :options="expireOptions" />
        </n-form-item>
        <n-form-item v-if="isCustom" label="Custom days" path="customDays">
          <n-input-number
            v-model:value="form.customDays"
            :min="1"
            :max="365"
            style="width: 100%"
          />
        </n-form-item>
      </n-form>

      <template #footer>
        <n-space justify="end">
          <n-button @click="closeModal">Cancel</n-button>
          <n-button type="primary" :loading="saving" @click="handleGenerate">
            <template #icon><icon-park-outline-key /></template>
            Generate
          </n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Reveal-once modal -->
    <n-modal
      v-model:show="showReveal"
      title="API Key Created"
      preset="card"
      style="width: 560px"
      :mask-closable="false"
      :closable="false"
    >
      <n-space vertical size="large">
        <n-alert type="warning" :bordered="false">
          Copy your key now. For security reasons it will not be shown again.
        </n-alert>
        <n-input
          type="textarea"
          :value="newKey"
          readonly
          :autosize="{ minRows: 3, maxRows: 6 }"
        />
        <n-space justify="end">
          <n-button @click="copyKey">
            <template #icon><icon-park-outline-copy /></template>
            Copy
          </n-button>
          <n-button type="primary" @click="dismissReveal">Done</n-button>
        </n-space>
      </n-space>
    </n-modal>
  </div>
</template>
