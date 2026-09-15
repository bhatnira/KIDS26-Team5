<script setup lang="tsx">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  NCard,
  NSpace,
  NInput,
  NButton,
  NDataTable,
  NTag,
  NSpin,
  NEmpty,
  useMessage
} from 'naive-ui'
import { fetchPipelineListAll as fetchPipelineListAPI } from '@/api/pipeline'

const router = useRouter()
const message = useMessage()

// State
const loading = ref(false)
const searchQuery = ref('')
const pipelines = ref([])

// Fetch pipeline list
const fetchPipelineList = async () => {
  loading.value = true
  try {
    const { isSuccess, data } = await fetchPipelineListAPI()

    if (isSuccess) {
      pipelines.value = data.pipelines || []
      message.success(`Loaded ${pipelines.value.length} pipelines`)
    } else {
      message.error('Failed to load pipelines')
    }
  } catch (error) {
    message.error('Error loading pipelines: ' + error.message)
    console.error('Error fetching pipelines:', error)
  } finally {
    loading.value = false
  }
}

// Computed filtered pipelines based on search
const filteredPipelines = computed(() => {
  if (!searchQuery.value) {
    return pipelines.value
  }

  const query = searchQuery.value.toLowerCase()
  return pipelines.value.filter(pipeline =>
    pipeline.name.toLowerCase().includes(query) ||
    pipeline.description.toLowerCase().includes(query) ||
    pipeline.author.toLowerCase().includes(query) ||
    pipeline.version.toLowerCase().includes(query)
  )
})

// Table columns
const columns = [
  {
    title: 'Pipeline Name',
    key: 'name',
    align: 'center',
    ellipsis: {
      tooltip: true
    },
    render: (row) => {
      return <div class="font-semibold text-primary">{row.name}</div>
    }
  },
  {
    title: 'Version',
    key: 'version',
    align: 'center',
    width: 100,
    render: (row) => {
      return (
        <NTag type="info" size="small">
          {row.version}
        </NTag>
      )
    }
  },
  {
    title: 'Author',
    key: 'author',
    align: 'center',
    width: 120
  },
  {
    title: 'Status',
    key: 'status',
    align: 'center',
    width: 100,
    render: (row) => {
      const statusMap = {
        ready: { type: 'success', label: 'Ready' },
        pending: { type: 'warning', label: 'Pending' },
        error: { type: 'error', label: 'Error' }
      }
      const status = statusMap[row.status] || { type: 'default', label: row.status }
      return (
        <NTag type={status.type} size="small">
          {status.label}
        </NTag>
      )
    }
  },
  {
    title: 'Description',
    key: 'description',
    align: 'center',
    ellipsis: {
      tooltip: true
    }
  },
  {
    title: 'Action',
    key: 'actions',
    width: 120,
    align: 'center',
    render: (row) => {
      return (
        <NButton
          type="primary"
          size="small"
          secondary
          onClick={() => handleLaunch(row)}
        >
          {{
            default: () => 'Launch',
            icon: () => <icon-park-outline-rocket />
          }}
        </NButton>
      )
    }
  }
]

// Handle launch pipeline
const handleLaunch = (pipeline) => {
  router.push({
    path: '/nextflow/launch/configuration',
    query: {
      id: pipeline.id,
      name: pipeline.name,
      version: pipeline.version,
      repository: pipeline.repository
    }
  })
}

// Lifecycle
onMounted(() => {
  fetchPipelineList()
})
</script>

<template>
  <NSpace vertical :size="20">
    <!-- Header Card -->
    <NCard>
      <NSpace vertical :size="16">
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-2xl font-semibold m-0">Pipeline Library</h2>
            <p class="text-gray-500 mt-1">Select and launch a pipeline for your analysis</p>
          </div>
          <NButton type="primary" @click="fetchPipelineList" :loading="loading">
            <template #icon>
              <icon-park-outline-refresh />
            </template>
            Refresh
          </NButton>
        </div>

        <!-- Search Bar -->
        <NInput
          v-model:value="searchQuery"
          placeholder="Search pipelines by name, author, version, or description..."
          clearable
          size="large"
        >
          <template #prefix>
            <icon-park-outline-search />
          </template>
        </NInput>

        <!-- Stats -->
        <div class="flex gap-4 text-sm">
          <span class="text-gray-600">
            Total Pipelines: <strong>{{ pipelines.length }}</strong>
          </span>
          <span class="text-gray-600">
            Filtered: <strong>{{ filteredPipelines.length }}</strong>
          </span>
        </div>
      </NSpace>
    </NCard>

    <!-- Pipelines Table -->
    <NCard>
      <NSpin :show="loading">
        <NDataTable
          v-if="filteredPipelines.length > 0"
          :columns="columns"
          :data="filteredPipelines"
          :bordered="false"
          :single-line="false"
          striped
          :row-key="(row) => row.id"
        />
        <NEmpty
          v-else-if="!loading && pipelines.length === 0"
          description="No pipelines available"
        />
        <NEmpty
          v-else-if="!loading && searchQuery"
          description="No pipelines match your search"
        >
          <template #extra>
            <NButton size="small" @click="searchQuery = ''">
              Clear Search
            </NButton>
          </template>
        </NEmpty>
      </NSpin>
    </NCard>
  </NSpace>
</template>

<style scoped>
:deep(.n-data-table-td) {
  padding: 12px 16px;
}

:deep(.n-data-table-th) {
  font-weight: 600;
}
</style>
