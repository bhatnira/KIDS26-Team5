<script setup>
import { ref, onMounted } from 'vue'
import { NSpace, NCard, NButton, useMessage, NAlert } from 'naive-ui'
import { useRouter } from 'vue-router'
import S3FileBrowser from '@/components/custom/S3FileBrowser.vue'
import { fetchStorageConfig } from '@/api/storage'
import { useBoolean } from '@/hooks'

const message = useMessage()
const router = useRouter()
const browserRef = ref(null)

// Storage configuration state
const { bool: loading, setTrue: startLoading, setFalse: endLoading } = useBoolean(true)
const storageConfigured = ref(false)
const storageHost = ref('')

// Load storage configuration on mount
onMounted(async () => {
  await loadStorageConfig()
})

async function loadStorageConfig() {
  startLoading()
  try {
    const { isSuccess, data } = await fetchStorageConfig()
    if (isSuccess && data?.config) {
      storageConfigured.value = data.config.configured
      if (data.config.configured) {
        storageHost.value = data.config.host
      }
    }
  } catch (error) {
    console.error('Failed to load storage config:', error)
  } finally {
    endLoading()
  }
}

function goToStorageSettings() {
  router.push('/user-setting/storage')
}

const handleRefresh = () => {
  browserRef.value?.refresh()
  message.success('Refreshed')
}

const handleExport = () => {
  message.info('Export functionality to be implemented')
}

const handleUpload = (data) => {
  console.log('File uploaded:', data)
}

const handleDownload = (data) => {
  console.log('File downloaded:', data)
}

const handleDelete = (data) => {
  console.log('File deleted:', data)
}
</script>

<template>
  <NSpace vertical size="large">
    <n-spin :show="loading">
      <!-- Not Configured State -->
      <template v-if="!loading && !storageConfigured">
        <n-card title="S3 File Browser">
          <n-empty size="large" description="No Data Source Configured">
            <template #icon>
              <icon-park-outline-cloud-storage class="text-5xl text-gray-400" />
            </template>
            <template #extra>
              <n-space vertical align="center">
                <n-text depth="3" class="text-center">
                  You need to configure your S3/MinIO storage before browsing files.
                </n-text>
                <n-button type="primary" @click="goToStorageSettings">
                  <template #icon>
                    <icon-park-outline-setting-two />
                  </template>
                  Configure Storage
                </n-button>
              </n-space>
            </template>
          </n-empty>
        </n-card>
      </template>

      <!-- Configured State - Show File Browser -->
      <template v-else-if="!loading && storageConfigured">
        <n-card title="S3 File Browser">
          <template #header-extra>
            <n-space>
              <n-tag type="success" size="small">
                <template #icon>
                  <icon-park-outline-link-cloud />
                </template>
                {{ storageHost }}
              </n-tag>
              <n-button type="primary" size="small" @click="handleRefresh">
                <template #icon>
                  <icon-park-outline-refresh />
                </template>
                Refresh
              </n-button>
              <n-button size="small" @click="goToStorageSettings">
                <template #icon>
                  <icon-park-outline-setting-two />
                </template>
                Settings
              </n-button>
            </n-space>
          </template>

          <S3FileBrowser
            ref="browserRef"
            mode="inline"
            :show-upload="true"
            :pagination="false"
            @upload="handleUpload"
            @download="handleDownload"
            @delete="handleDelete"
          />
        </n-card>
      </template>
    </n-spin>
  </NSpace>
</template>

