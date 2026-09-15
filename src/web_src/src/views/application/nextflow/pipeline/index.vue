<script setup lang="tsx">
import { NButton, NPopconfirm, NSpace, NTag, NCard, NTooltip } from 'naive-ui'
import { useBoolean } from '@/hooks'
import { fetchPipelineList, fetchDeletePipeline } from '@/api/pipeline'
import PipelineModal from './components/PipelineModal.vue'
import { useRouter } from 'vue-router'
import { usePermission } from '@/utils/permission'

const { bool: loading, setTrue: startLoading, setFalse: endLoading } = useBoolean(false)
const { bool: visible, setTrue: openModal, setFalse: closeModalState } = useBoolean(false)

// Permission check for admin/super users
const { isAdminOrSuper } = usePermission()
const canAddPipeline = computed(() => isAdminOrSuper())

const initialModel = {
  name: '',
  version: '',
  status: '',
  author: '',
}
const model = ref({ ...initialModel })
const router = useRouter()

const formRef = ref(null)

// View mode: 'card' or 'table'
const viewMode = ref('table')

// Pagination state
const pagination = ref({
  page: 1,
  pageSize: 20,
  total: 0,
})

async function deletePipeline(name, version) {
  const res = await fetchDeletePipeline(name, version)
  if (res.isSuccess) {
    window.$message.success(`delete pipeline success: ${name} v${version}`)
    getPipelineList()
  }
}

const columns = [
  {
    title: 'ID',
    align: 'center',
    key: 'id',
    width: 72,
  },
  {
    title: 'Name',
    align: 'center',
    key: 'name',
    width: 168,
    ellipsis: {
      tooltip: true,
    },
  },
  {
    title: 'Version',
    align: 'center',
    key: 'version',
    width: 88,
  },
  {
    title: 'Status',
    align: 'center',
    key: 'status',
    width: 104,
    render: (row) => {
      const tagType = {
        ready: 'success',
        pending: 'warning',
        failed: 'error',
      }

      return (
        <NTag type={tagType[row.status]}>
          {row.status}
        </NTag>
      )
    },
  },
  {
    title: 'Author',
    align: 'center',
    key: 'author',
    width: 112,
  },
  {
    title: 'Repository',
    align: 'center',
    key: 'repository',
    width: 204,
    ellipsis: {
      tooltip: true,
    },
    render: (row) => {
      return (
        <a href={row.repository} target="_blank" rel="noopener noreferrer" class="text-blue-500 hover:underline">
          {row.repository}
        </a>
      )
    },
  },
  {
    title: 'Description',
    align: 'center',
    key: 'description',
    minWidth: 180,
    ellipsis: {
      tooltip: true,
    },
  },
  {
    title: 'Actions',
    align: 'center',
    key: 'actions',
    width: 128,
    fixed: 'right',
    render: (row) => {
      return (
        <NSpace justify="center" size="small">
          <NTooltip>
            {{
              trigger: () => (
                <NButton quaternary circle size="small" onClick={() => handleEditPipeline(row)}>
                  <icon-park-outline-edit />
                </NButton>
              ),
              default: () => 'Edit',
            }}
          </NTooltip>
          <NTooltip>
            {{
              trigger: () => (
                <NButton
                  quaternary
                  circle
                  size="small"
                  type="primary"
                  onClick={() => handleLaunchPipeline(row)}
                >
                  <icon-park-outline-play-one />
                </NButton>
              ),
              default: () => 'Launch',
            }}
          </NTooltip>
          {canAddPipeline.value && (
            <NPopconfirm onPositiveClick={() => deletePipeline(row.name, row.version)}>
              {{
                default: () => 'Confirm Delete？',
                trigger: () => (
                  <NTooltip>
                    {{
                      trigger: () => (
                        <NButton quaternary circle size="small" type="error">
                          <icon-park-outline-delete />
                        </NButton>
                      ),
                      default: () => 'Delete',
                    }}
                  </NTooltip>
                ),
              }}
            </NPopconfirm>
          )}
        </NSpace>
      )
    },
  },
]

const listData = ref([])
const allPipelinesData = ref([])

onMounted(() => {
  getPipelineList()
})

async function getPipelineList() {
  startLoading()

  try {
    const res = await fetchPipelineList(pagination.value.page, pagination.value.pageSize)
    allPipelinesData.value = res.data.pipelines || []
    applySearchFilter()
    pagination.value.total = res.data.pagination.total || 0
    endLoading()
  } catch (error) {
    window.$message.error('获取管道列表失败')
    endLoading()
  }
}

function applySearchFilter() {
  let filteredData = [...allPipelinesData.value]

  // Filter by name
  if (model.value.name) {
    filteredData = filteredData.filter(pipeline =>
      pipeline.name.toLowerCase().includes(model.value.name.toLowerCase())
    )
  }

  // Filter by version
  if (model.value.version) {
    filteredData = filteredData.filter(pipeline =>
      pipeline.version.toLowerCase().includes(model.value.version.toLowerCase())
    )
  }

  // Filter by status
  if (model.value.status) {
    filteredData = filteredData.filter(pipeline =>
      pipeline.status === model.value.status
    )
  }

  // Filter by author
  if (model.value.author) {
    filteredData = filteredData.filter(pipeline =>
      pipeline.author.toLowerCase().includes(model.value.author.toLowerCase())
    )
  }

  listData.value = filteredData
}

function changePage(page, pageSize) {
  pagination.value.page = page
  pagination.value.pageSize = pageSize
  getPipelineList()
}

function handleResetSearch() {
  model.value = { ...initialModel }
  getPipelineList()
}

function handleSearch() {
  applySearchFilter()
}

const modalType = ref('add')
function setModalType(type) {
  modalType.value = type
}

const editData = ref(null)
function setEditData(data) {
  editData.value = data
}

function handleEditPipeline(row) {
  setEditData(row)
  setModalType('edit')
  openModal()
}

function handleAddPipeline() {
  setEditData(null)
  setModalType('add')
  openModal()
}

function handleLaunchPipeline(pipeline) {
  // TODO: Implement launch logic in the future
  // window.$message.info(`启动管道: ${pipeline.name} v${pipeline.version} (功能开发中)`)
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

function handleModalSuccess() {
  closeModalState()
  getPipelineList()
}

function getStatusColor(status) {
  const colors = {
    ready: '#52c41a',
    pending: '#faad14',
    failed: '#f5222d',
  }
  return colors[status] || '#d9d9d9'
}
</script>

<template>
  <NSpace vertical size="large">
    <n-card>
      <n-form ref="formRef" :model="model" label-placement="left" inline :show-feedback="false">
        <n-flex>
          <n-form-item label="Name" path="name">
            <n-input v-model:value="model.name" placeholder="Pipeline Name" />
          </n-form-item>
          <n-form-item label="Version" path="version">
            <n-input v-model:value="model.version" placeholder="Version" />
          </n-form-item>
          <n-form-item label="Status" path="status">
            <n-select
              v-model:value="model.status"
              placeholder="Choose Status"
              :options="[
                { label: 'All', value: '' },
                { label: 'Ready', value: 'ready' },
                { label: 'Pending', value: 'pending' },
                { label: 'Failed', value: 'failed' },
              ]"
              clearable
              style="width: 150px"
            />
          </n-form-item>
          <n-form-item label="Author" path="author">
            <n-input v-model:value="model.author" placeholder="Author" />
          </n-form-item>
          <n-flex class="ml-auto">
            <NButton type="primary" @click="handleSearch">
              <template #icon>
                <icon-park-outline-search />
              </template>
              Search
            </NButton>
            <NButton strong secondary @click="handleResetSearch">
              <template #icon>
                <icon-park-outline-redo />
              </template>
              Reset
            </NButton>
          </n-flex>
        </n-flex>
      </n-form>
    </n-card>
    <n-card>
      <NSpace vertical size="large">
        <div class="flex gap-4 items-center">
          <NButton v-if="canAddPipeline" type="primary" @click="handleAddPipeline">
            <template #icon>
              <icon-park-outline-add-one />
            </template>
            New Pipeline
          </NButton>
          <NButton strong secondary :loading="loading" @click="getPipelineList">
            <template #icon>
              <icon-park-outline-refresh />
            </template>
            Refresh
          </NButton>
          <div class="ml-auto flex gap-2">
            <NButton
              :type="viewMode === 'card' ? 'primary' : 'default'"
              @click="viewMode = 'card'"
            >
              <template #icon>
                <icon-park-outline-grid-four />
              </template>
              Card View
            </NButton>
            <NButton
              :type="viewMode === 'table' ? 'primary' : 'default'"
              @click="viewMode = 'table'"
            >
              <template #icon>
                <icon-park-outline-list />
              </template>
              Table View
            </NButton>
          </div>
        </div>

        <!-- Table View -->
        <n-data-table
          v-if="viewMode === 'table'"
          :columns="columns"
          :data="listData"
          :loading="loading"
          :scroll-x="876"
        />

        <!-- Card View -->
        <div v-else class="grid grid-cols-1 md:grid-cols-3 lg:grid-cols-5 gap-4">
          <n-card
            v-for="pipeline in listData"
            :key="pipeline.id"
            :title="pipeline.name"
            hoverable
            class="cursor-pointer"
          >
            <template #header-extra>
              <NTag :type="pipeline.status === 'ready' ? 'success' : pipeline.status === 'pending' ? 'warning' : 'error'">
                {{ pipeline.status }}
              </NTag>
            </template>

            <div class="space-y-3">
              <div class="flex items-center gap-2">
                <span class="text-gray-500 font-medium">Version:</span>
                <span>{{ pipeline.version }}</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="text-gray-500 font-medium">Author:</span>
                <span>{{ pipeline.author }}</span>
              </div>
              <div class="flex flex-col gap-1">
                <span class="text-gray-500 font-medium">Repository:</span>
                <a
                  :href="pipeline.repository"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="text-blue-500 hover:underline text-sm break-all"
                >
                  {{ pipeline.repository }}
                </a>
              </div>
              <div class="flex flex-col gap-1">
                <span class="text-gray-500 font-medium">Description:</span>
                <span class="text-sm text-gray-600">{{ pipeline.description || 'No Description' }}</span>
              </div>
            </div>

            <template #action>
              <NSpace justify="space-between">
                <NButton
                  size="small"
                  @click="handleEditPipeline(pipeline)"
                >
                  Edit
                </NButton>
                <NButton
                  size="small"
                  type="primary"
                  @click="handleLaunchPipeline(pipeline)"
                >
                  Launch
                </NButton>
                <NPopconfirm v-if="canAddPipeline" @positive-click="deletePipeline(pipeline.name, pipeline.version)">
                  <template #trigger>
                    <NButton size="small" type="error">
                      Delete
                    </NButton>
                  </template>
                  Delete Pipeline Confirm？
                </NPopconfirm>
              </NSpace>
            </template>
          </n-card>
        </div>

<!--        <div v-if="!loading && listData.length === 0" class="text-center py-8 text-gray-400">-->
<!--          暂无数据-->
<!--        </div>-->

        <Pagination
          :count="pagination.total"
          :page="pagination.page"
          :page-size="pagination.pageSize"
          @change="changePage"
        />
        <PipelineModal
          v-model:visible="visible"
          :type="modalType"
          :modal-data="editData"
          @success="handleModalSuccess"
        />
      </NSpace>
    </n-card>
  </NSpace>
</template>
