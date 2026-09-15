<script setup>
import { ref, onMounted, computed } from 'vue'
import { useMessage } from 'naive-ui'
import { useBoolean } from '@/hooks'
import { fetchAgentWorkspaceConfig, saveAgentWorkspaceConfig } from '@/api/agent'
import { fetchBuckets } from '@/api/storage'

const message = useMessage()

const { bool: loading, setTrue: startLoading, setFalse: endLoading } = useBoolean(true)
const { bool: saving, setTrue: startSaving, setFalse: endSaving } = useBoolean(false)

const buckets = ref([])
const configured = ref(false)
const daytonaKeySet = ref(false)
const updatedAt = ref(null)

// Credential-input sentinels:
//   ''         → leave existing untouched
//   '<clear>'  → wipe
//   any other  → set
const form = ref({
  bucket: '',
  daytona_api_key: '',
  daytona_api_url: '',
})

const bucketOptions = computed(() =>
  buckets.value.map(b => ({ label: b.name, value: b.name }))
)

const formRules = {
  bucket: { required: true, message: 'Choose a workspace bucket', trigger: 'change' },
}

const formRef = ref(null)

async function loadAll() {
  startLoading()
  try {
    const [cfgRes, bucketRes] = await Promise.all([
      fetchAgentWorkspaceConfig(),
      fetchBuckets(),
    ])
    if (cfgRes.isSuccess && cfgRes.data) {
      configured.value = !!cfgRes.data.configured
      form.value.bucket = cfgRes.data.bucket || ''
      form.value.daytona_api_url = cfgRes.data.daytona_api_url || ''
      daytonaKeySet.value = !!cfgRes.data.daytona_api_key_set
      updatedAt.value = cfgRes.data.updated_at || null
    }
    if (bucketRes.isSuccess) {
      buckets.value = bucketRes.data?.buckets || bucketRes.data?.items || bucketRes.data || []
    }
  } finally {
    endLoading()
  }
}

onMounted(loadAll)

async function handleSave() {
  try {
    await formRef.value?.validate()
  } catch { return }
  startSaving()
  try {
    const { isSuccess, message: msg } = await saveAgentWorkspaceConfig(form.value)
    if (isSuccess) {
      message.success('Workspace saved')
      // Reset key-input sentinel; user must re-type to change again.
      form.value.daytona_api_key = ''
      await loadAll()
    } else {
      message.error(msg || 'Failed to save')
    }
  } finally {
    endSaving()
  }
}

function handleClearDaytonaKey() {
  form.value.daytona_api_key = '<clear>'
  message.info('Daytona key will be cleared on save')
}
</script>

<template>
  <div>
    <n-space vertical size="large">
      <n-alert type="info" title="Agent Workspace">
        Pick a bucket the agent will use as scratch space and store its artifacts
        (under <code>agent/{sessionID}/</code>), then provide your Daytona
        credentials for code execution.
      </n-alert>

      <n-card title="Workspace">
        <template #header-extra>
          <n-tag v-if="configured" type="success" size="small">Configured</n-tag>
          <n-tag v-else type="warning" size="small">Not configured</n-tag>
        </template>

        <n-spin :show="loading">
          <n-form
            ref="formRef"
            :model="form"
            :rules="formRules"
            label-placement="left"
            label-width="180"
            require-mark-placement="right-hanging"
          >
            <n-form-item label="Workspace bucket" path="bucket">
              <n-select
                v-model:value="form.bucket"
                :options="bucketOptions"
                placeholder="Select a bucket"
                filterable
              />
              <template #feedback>
                The bucket must already exist in your storage. Create one in
                <router-link to="/storage/minio">Storage → MinIO</router-link>.
              </template>
            </n-form-item>

            <n-form-item label="Daytona API URL" path="daytona_api_url">
              <n-input
                v-model:value="form.daytona_api_url"
                placeholder="https://app.daytona.io/api (leave blank for SDK default)"
              />
              <template #feedback>
                Optional. Set this when running self-hosted Daytona; otherwise the
                SDK uses its default endpoint or the <code>DAYTONA_API_URL</code> env var.
              </template>
            </n-form-item>

            <n-form-item label="Daytona API key" path="daytona_api_key">
              <n-input
                v-model:value="form.daytona_api_key"
                type="password"
                show-password-on="click"
                :placeholder="daytonaKeySet ? '••••••••  (leave empty to keep current)' : 'dtn_...'"
              />
              <template #feedback>
                <span v-if="daytonaKeySet">
                  A key is stored.
                  <a href="#" class="link" @click.prevent="handleClearDaytonaKey">Clear it</a>
                  or type a new value to replace.
                </span>
                <span v-else>Required for code execution.</span>
              </template>
            </n-form-item>

            <n-form-item v-if="updatedAt" label="Last updated">
              <n-text depth="3">{{ new Date(updatedAt).toLocaleString() }}</n-text>
            </n-form-item>
          </n-form>
        </n-spin>

        <template #action>
          <n-space justify="end">
            <n-button type="primary" :loading="saving" @click="handleSave">
              <template #icon><icon-park-outline-save /></template>
              Save
            </n-button>
          </n-space>
        </template>
      </n-card>
    </n-space>
  </div>
</template>

<style scoped>
.link {
  color: var(--n-primary-color, #18a058);
  text-decoration: none;
}
.link:hover { text-decoration: underline; }
</style>
