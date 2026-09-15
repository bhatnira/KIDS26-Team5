<script setup>
import { ref, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { fetchLLMConfigs, addLLMConfig, updateLLMConfig, deleteLLMConfig, testLLMConnection } from '@/api/llm'
import { useBoolean } from '@/hooks'

const message = useMessage()

const { bool: loading, setTrue: startLoading, setFalse: endLoading } = useBoolean(true)
const { bool: saving, setTrue: startSaving, setFalse: endSaving } = useBoolean(false)
const { bool: testing, setTrue: startTesting, setFalse: endTesting } = useBoolean(false)
const { bool: showModal, setTrue: openModal, setFalse: closeModal } = useBoolean(false)

const configs = ref([])
const editingId = ref(null)

const defaultForm = () => ({
  name: '',
  provider: 'openai',
  api_key: '',
  model: 'gpt-4o',
  base_url: '',
  max_tokens: 4096,
  temperature: 0.7,
  thinking_enabled: false,
  thinking_effort: 'medium',
  is_default: false,
  insecure_skip_verify: false,
})

const form = ref(defaultForm())
const formRef = ref(null)

const providerOptions = [
  { label: 'OpenAI', value: 'openai' },
  { label: 'OpenAI-compatible', value: 'openai-compatible' },
  { label: 'Anthropic', value: 'anthropic' },
]

const effortOptions = [
  { label: 'Low', value: 'low' },
  { label: 'Medium', value: 'medium' },
  { label: 'High', value: 'high' },
  { label: 'X-High', value: 'xhigh' },
  { label: 'Max', value: 'max' },
]

const formRules = {
  name: { required: true, message: 'Please enter a name', trigger: 'blur' },
  provider: { required: true, message: 'Please select a provider', trigger: 'change' },
  api_key: { required: true, message: 'Please enter the API key', trigger: 'blur' },
  model: { required: true, message: 'Please enter the model name', trigger: 'blur' },
}

onMounted(loadConfigs)

async function loadConfigs() {
  startLoading()
  try {
    const { isSuccess, data } = await fetchLLMConfigs()
    if (isSuccess) {
      configs.value = data?.configs || []
    }
  } finally {
    endLoading()
  }
}

function handleAdd() {
  editingId.value = null
  form.value = defaultForm()
  openModal()
}

function handleEdit(cfg) {
  editingId.value = cfg.id
  form.value = {
    name: cfg.name,
    provider: cfg.provider,
    api_key: cfg.api_key,
    model: cfg.model,
    base_url: cfg.base_url || '',
    max_tokens: cfg.max_tokens || 4096,
    temperature: cfg.temperature ?? 0.7,
    thinking_enabled: cfg.thinking_enabled ?? false,
    thinking_effort: cfg.thinking_effort || 'medium',
    is_default: cfg.is_default,
    insecure_skip_verify: cfg.insecure_skip_verify ?? false,
  }
  openModal()
}

async function handleTestConnection() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  startTesting()
  try {
    const { isSuccess, message: msg } = await testLLMConnection(form.value)
    if (isSuccess) {
      message.success('Connection successful!')
    } else {
      message.error(msg || 'Connection failed')
    }
  } catch (err) {
    message.error('Test failed: ' + (err.message || 'Unknown error'))
  } finally {
    endTesting()
  }
}

async function handleSave() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  startSaving()
  try {
    let result
    if (editingId.value) {
      result = await updateLLMConfig(editingId.value, form.value)
    } else {
      result = await addLLMConfig(form.value)
    }
    const { isSuccess, message: msg } = result
    if (isSuccess) {
      message.success(editingId.value ? 'Configuration updated' : 'Configuration added')
      closeModal()
      await loadConfigs()
    } else {
      message.error(msg || 'Failed to save')
    }
  } catch (err) {
    message.error('Failed to save: ' + (err.message || 'Unknown error'))
  } finally {
    endSaving()
  }
}

async function handleDelete(id) {
  startSaving()
  try {
    const { isSuccess, message: msg } = await deleteLLMConfig(id)
    if (isSuccess) {
      message.success('Configuration deleted')
      await loadConfigs()
    } else {
      message.error(msg || 'Failed to delete')
    }
  } finally {
    endSaving()
  }
}
</script>

<template>
  <div>
  <n-space vertical size="large">
    <n-alert type="info" title="LLM Configuration">
      Configure your own LLM API keys. Each user can add multiple configurations and select which model
      to use when chatting with Antelope AI.
    </n-alert>

    <n-card title="LLM Configurations">
      <template #header-extra>
        <n-button type="primary" @click="handleAdd">
          <template #icon>
            <icon-park-outline-add />
          </template>
          Add Configuration
        </n-button>
      </template>

      <n-spin :show="loading">
        <n-empty v-if="configs.length === 0 && !loading" description="No LLM configuration yet">
          <template #icon>
            <icon-park-outline-robot class="text-4xl text-gray-400" />
          </template>
          <template #extra>
            <n-button type="primary" @click="handleAdd">
              <template #icon>
                <icon-park-outline-add />
              </template>
              Add Configuration
            </n-button>
          </template>
        </n-empty>

        <n-list v-else bordered>
          <n-list-item v-for="cfg in configs" :key="cfg.id">
            <n-thing>
              <template #header>
                <n-space align="center">
                  <n-text strong>{{ cfg.name }}</n-text>
                  <n-tag v-if="cfg.is_default" type="success" size="small">Default</n-tag>
                </n-space>
              </template>
              <template #description>
                <n-space>
                  <n-text depth="3">Provider: {{ cfg.provider }}</n-text>
                  <n-text depth="3">Model: {{ cfg.model }}</n-text>
                  <n-text depth="3">API Key: {{ cfg.api_key }}</n-text>
                </n-space>
              </template>
            </n-thing>
            <template #suffix>
              <n-space>
                <n-button size="small" @click="handleEdit(cfg)">
                  <template #icon>
                    <icon-park-outline-edit />
                  </template>
                  Edit
                </n-button>
                <n-popconfirm @positive-click="handleDelete(cfg.id)">
                  <template #trigger>
                    <n-button size="small" type="error" :loading="saving">
                      <template #icon>
                        <icon-park-outline-delete />
                      </template>
                      Delete
                    </n-button>
                  </template>
                  Delete this LLM configuration?
                </n-popconfirm>
              </n-space>
            </template>
          </n-list-item>
        </n-list>
      </n-spin>
    </n-card>
  </n-space>

  <!-- Add / Edit modal -->
  <n-modal
    v-model:show="showModal"
    :title="editingId ? 'Edit LLM Configuration' : 'Add LLM Configuration'"
    preset="card"
    style="width: 520px"
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
        <n-input v-model:value="form.name" placeholder="e.g., My OpenAI key" />
      </n-form-item>
      <n-form-item label="Provider" path="provider">
        <n-select v-model:value="form.provider" :options="providerOptions" />
      </n-form-item>
      <n-form-item label="API Key" path="api_key">
        <n-input
          v-model:value="form.api_key"
          type="password"
          show-password-on="click"
          placeholder="sk-..."
        />
      </n-form-item>
      <n-form-item label="Model" path="model">
        <n-input v-model:value="form.model" placeholder="e.g., gpt-4o" />
      </n-form-item>
      <n-form-item label="Base URL" path="base_url">
        <n-input
          v-model:value="form.base_url"
          placeholder="e.g., https://your-llm-host/v1"
        />
        <template #feedback>
          Must include the <strong>/v1</strong> path prefix (e.g. <code>https://your-llm-host/v1</code>). Leave empty to use the default OpenAI endpoint.
        </template>
      </n-form-item>
      <n-form-item label="Max Tokens" path="max_tokens">
        <n-input-number v-model:value="form.max_tokens" :min="1" :max="128000" style="width: 100%" />
      </n-form-item>
      <n-form-item label="Temperature" path="temperature">
        <n-slider v-model:value="form.temperature" :min="0" :max="2" :step="0.1" style="width: 100%" />
        <n-input-number
          v-model:value="form.temperature"
          :min="0"
          :max="2"
          :step="0.1"
          :precision="1"
          style="width: 80px; margin-left: 12px; flex-shrink: 0"
        />
      </n-form-item>
      <n-form-item label="Enable Thinking" path="thinking_enabled">
        <n-switch v-model:value="form.thinking_enabled" />
        <span class="ml-2 text-gray-500 text-sm">Let reasoning models emit a thinking process before answering</span>
      </n-form-item>
      <n-form-item v-if="form.thinking_enabled" label="Thinking Effort" path="thinking_effort">
        <n-select v-model:value="form.thinking_effort" :options="effortOptions" />
        <template #feedback>
          Only applies to reasoning-capable models (e.g. Anthropic Claude, OpenAI o-series / GPT-5). Higher effort spends more on reasoning; <strong>xhigh</strong>/<strong>max</strong> are Anthropic-only, OpenAI accepts only low/medium/high.
        </template>
      </n-form-item>
      <n-form-item label="Set as Default" path="is_default">
        <n-switch v-model:value="form.is_default" />
        <span class="ml-2 text-gray-500 text-sm">Use this config by default in AI Chat</span>
      </n-form-item>
      <n-form-item label="Skip TLS Verify" path="insecure_skip_verify">
        <n-switch v-model:value="form.insecure_skip_verify" />
        <span class="ml-2 text-orange-500 text-sm">Skip TLS certificate verification (for local LLMs only)</span>
      </n-form-item>
    </n-form>

    <template #footer>
      <n-space justify="end">
        <n-button @click="handleTestConnection" :loading="testing">
          <template #icon>
            <icon-park-outline-connection-point />
          </template>
          Test Connection
        </n-button>
        <n-button type="primary" @click="handleSave" :loading="saving">
          <template #icon>
            <icon-park-outline-save />
          </template>
          Save
        </n-button>
        <n-button @click="closeModal">Cancel</n-button>
      </n-space>
    </template>
  </n-modal>
  </div>
</template>
