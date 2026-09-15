<script setup>
import { ref, computed, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { useBoolean } from '@/hooks'
import {
  fetchMCPConfigs,
  addMCPConfig,
  updateMCPConfig,
  deleteMCPConfig,
} from '@/api/agent'

const message = useMessage()

const { bool: loading, setTrue: startLoading, setFalse: endLoading } = useBoolean(true)
const { bool: saving, setTrue: startSaving, setFalse: endSaving } = useBoolean(false)
const { bool: showModal, setTrue: openModal, setFalse: closeModal } = useBoolean(false)

const items = ref([])
const editingId = ref(null)

const defaultForm = () => ({
  name: '',
  transport: 'streamable',
  server_url: '',
  command: '',
  args: [],
  headers: [],
  timeout_seconds: 30,
  enabled: true,
  description: '',
})

const form = ref(defaultForm())
const formRef = ref(null)

const transportOptions = [
  { label: 'streamable (HTTP streaming)', value: 'streamable' },
  { label: 'sse (HTTP Server-Sent Events)', value: 'sse' },
  { label: 'stdio (spawn local process)', value: 'stdio' },
]

const formRules = {
  name:      { required: true, message: 'Name is required', trigger: 'blur' },
  transport: { required: true, message: 'Transport is required', trigger: 'change' },
}

const isStdio = computed(() => form.value.transport === 'stdio')
const isHTTP = computed(() => form.value.transport !== 'stdio')

async function load() {
  startLoading()
  try {
    const { isSuccess, data } = await fetchMCPConfigs()
    if (isSuccess) items.value = data?.items || []
  } finally {
    endLoading()
  }
}

onMounted(load)

function handleAdd() {
  editingId.value = null
  form.value = defaultForm()
  openModal()
}

function handleEdit(row) {
  editingId.value = row.id
  form.value = {
    name: row.name || '',
    transport: row.transport || 'streamable',
    server_url: row.server_url || '',
    command: row.command || '',
    args: row.args || [],
    headers: Object.entries(row.headers || {}).map(([k, v]) => ({ k, v })),
    timeout_seconds: row.timeout_seconds || 30,
    enabled: !!row.enabled,
    description: row.description || '',
  }
  openModal()
}

function buildPayload() {
  const headers = {}
  for (const h of form.value.headers || []) {
    if (h.k) headers[h.k] = h.v || ''
  }
  return {
    name: form.value.name.trim(),
    transport: form.value.transport,
    server_url: form.value.server_url.trim(),
    command: form.value.command.trim(),
    args: form.value.args.filter(a => a && a.trim()),
    headers,
    timeout_seconds: Number(form.value.timeout_seconds) || 30,
    enabled: form.value.enabled,
    description: form.value.description.trim(),
  }
}

async function handleSave() {
  try { await formRef.value?.validate() } catch { return }
  if (isHTTP.value && !form.value.server_url.trim()) {
    message.warning('Server URL is required for sse/streamable transports')
    return
  }
  if (isStdio.value && !form.value.command.trim()) {
    message.warning('Command is required for stdio transport')
    return
  }
  startSaving()
  try {
    const payload = buildPayload()
    const result = editingId.value
      ? await updateMCPConfig(editingId.value, payload)
      : await addMCPConfig(payload)
    if (result.isSuccess) {
      message.success(editingId.value ? 'Updated' : 'Added')
      closeModal()
      await load()
    } else {
      message.error(result.message || 'Save failed')
    }
  } finally {
    endSaving()
  }
}

async function handleDelete(id) {
  startSaving()
  try {
    const { isSuccess, message: msg } = await deleteMCPConfig(id)
    if (isSuccess) {
      message.success('Deleted')
      await load()
    } else {
      message.error(msg || 'Delete failed')
    }
  } finally {
    endSaving()
  }
}

function addHeader() { form.value.headers.push({ k: '', v: '' }) }
function removeHeader(i) { form.value.headers.splice(i, 1) }
function addArg() { form.value.args.push('') }
function removeArg(i) { form.value.args.splice(i, 1) }
</script>

<template>
  <div>
    <n-space vertical size="large">
      <n-alert type="info" title="MCP Servers">
        Connect <a href="https://modelcontextprotocol.io" target="_blank" rel="noopener">Model Context Protocol</a>
        servers — tools from each connected server become callable by the agent. Globally-shared
        configs (administered platform-wide) appear read-only.
      </n-alert>

      <n-card title="MCP Configurations">
        <template #header-extra>
          <n-button type="primary" @click="handleAdd">
            <template #icon><icon-park-outline-add /></template>
            Add Server
          </n-button>
        </template>

        <n-spin :show="loading">
          <n-empty v-if="!items.length && !loading" description="No MCP servers yet">
            <template #icon><icon-park-outline-connection-point class="text-4xl text-gray-400" /></template>
            <template #extra>
              <n-button type="primary" @click="handleAdd">
                <template #icon><icon-park-outline-add /></template>
                Add Server
              </n-button>
            </template>
          </n-empty>

          <n-list v-else bordered>
            <n-list-item v-for="it in items" :key="it.id">
              <n-thing>
                <template #header>
                  <n-space align="center">
                    <n-text strong>{{ it.name }}</n-text>
                    <n-tag :type="it.enabled ? 'success' : 'default'" size="small">
                      {{ it.enabled ? 'Enabled' : 'Disabled' }}
                    </n-tag>
                    <n-tag v-if="it.is_global" type="info" size="small">Built-in</n-tag>
                    <n-tag size="small">{{ it.transport }}</n-tag>
                  </n-space>
                </template>
                <template #description>
                  <n-space size="small" vertical>
                    <n-text depth="3" v-if="it.transport !== 'stdio' && it.server_url">
                      {{ it.server_url }}
                    </n-text>
                    <n-text depth="3" v-if="it.transport === 'stdio' && it.command">
                      <code>{{ it.command }} {{ (it.args || []).join(' ') }}</code>
                    </n-text>
                    <n-text depth="3" v-if="it.description">{{ it.description }}</n-text>
                  </n-space>
                </template>
              </n-thing>
              <template #suffix>
                <n-space>
                  <n-button v-if="it.editable" size="small" @click="handleEdit(it)">
                    <template #icon><icon-park-outline-edit /></template>
                    Edit
                  </n-button>
                  <n-popconfirm v-if="it.editable" @positive-click="handleDelete(it.id)">
                    <template #trigger>
                      <n-button size="small" type="error" :loading="saving">
                        <template #icon><icon-park-outline-delete /></template>
                        Delete
                      </n-button>
                    </template>
                    Delete this MCP server?
                  </n-popconfirm>
                </n-space>
              </template>
            </n-list-item>
          </n-list>
        </n-spin>
      </n-card>
    </n-space>

    <n-modal
      v-model:show="showModal"
      :title="editingId ? 'Edit MCP Server' : 'Add MCP Server'"
      preset="card"
      style="width: 600px"
      :mask-closable="false"
    >
      <n-form
        ref="formRef"
        :model="form"
        :rules="formRules"
        label-placement="left"
        label-width="140"
        require-mark-placement="right-hanging"
      >
        <n-form-item label="Name" path="name">
          <n-input v-model:value="form.name" placeholder="e.g., GitHub MCP" />
        </n-form-item>

        <n-form-item label="Description" path="description">
          <n-input v-model:value="form.description" placeholder="Short capability summary" />
        </n-form-item>

        <n-form-item label="Transport" path="transport">
          <n-select v-model:value="form.transport" :options="transportOptions" />
        </n-form-item>

        <!-- HTTP transports -->
        <n-form-item v-if="isHTTP" label="Server URL" path="server_url">
          <n-input v-model:value="form.server_url" placeholder="https://mcp.example.com/sse" />
        </n-form-item>
        <n-form-item v-if="isHTTP" label="Headers">
          <n-space vertical style="width: 100%">
            <n-space v-for="(h, i) in form.headers" :key="i" align="center">
              <n-input v-model:value="h.k" placeholder="Header" style="width: 180px" />
              <n-input v-model:value="h.v" placeholder="Value" style="width: 240px" />
              <n-button quaternary circle size="small" @click="removeHeader(i)">
                <template #icon><icon-park-outline-close-small /></template>
              </n-button>
            </n-space>
            <n-button size="small" dashed @click="addHeader">
              <template #icon><icon-park-outline-add /></template>
              Add header
            </n-button>
          </n-space>
        </n-form-item>

        <!-- stdio transport -->
        <n-form-item v-if="isStdio" label="Command" path="command">
          <n-input v-model:value="form.command" placeholder="uvx" />
        </n-form-item>
        <n-form-item v-if="isStdio" label="Args">
          <n-space vertical style="width: 100%">
            <n-space v-for="(_, i) in form.args" :key="i" align="center">
              <n-input v-model:value="form.args[i]" placeholder="--flag or value" style="width: 380px" />
              <n-button quaternary circle size="small" @click="removeArg(i)">
                <template #icon><icon-park-outline-close-small /></template>
              </n-button>
            </n-space>
            <n-button size="small" dashed @click="addArg">
              <template #icon><icon-park-outline-add /></template>
              Add arg
            </n-button>
          </n-space>
        </n-form-item>

        <n-form-item label="Timeout (s)" path="timeout_seconds">
          <n-input-number v-model:value="form.timeout_seconds" :min="1" :max="600" style="width: 100%" />
        </n-form-item>

        <n-form-item label="Enabled" path="enabled">
          <n-switch v-model:value="form.enabled" />
        </n-form-item>
      </n-form>

      <template #footer>
        <n-space justify="end">
          <n-button type="primary" :loading="saving" @click="handleSave">
            <template #icon><icon-park-outline-save /></template>
            Save
          </n-button>
          <n-button @click="closeModal">Cancel</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>
