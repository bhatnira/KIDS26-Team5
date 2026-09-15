<script setup lang="tsx">
import { useBoolean } from '@/hooks'
import { fetchUserList, fetchUpdateUser, fetchDeleteUser } from '@/api/admin'
import { NButton, NPopconfirm, NSpace, NSwitch, NTag } from 'naive-ui'
import { ref, onMounted } from 'vue'
import { $t } from '@/utils/i18n'
import TableModal from './components/TableModal.vue'

const { bool: loading, setTrue: startLoading, setFalse: endLoading } = useBoolean(false)

// Search form
const initialModel = {
  name: '',
  email: '',
  role: null,
}
const model = ref({ ...initialModel })
const formRef = ref(null)
const modalRef = ref()

function handleResetSearch() {
  model.value = { ...initialModel }
  applyFilters()
}

// Role options for search filter
const roleOptions = [
  { label: 'All', value: null },
  { label: 'Admin', value: 'admin' },
  { label: 'Super', value: 'super' },
  { label: 'User', value: 'user' },
]

// Data storage
const allUsers = ref([])
const filteredUsers = ref([])
const displayUsers = ref([])

// Pagination
const pagination = ref({
  page: 1,
  pageSize: 10,
  total: 0,
})

// Tree filter
const selectedDepartment = ref(null)
const selectedGroup = ref(null)
const treeData = ref([])

// Generate tree data from user list
function generateTreeData(users) {
  const departmentMap = new Map()

  users.forEach(user => {
    if (user.department) {
      if (!departmentMap.has(user.department)) {
        departmentMap.set(user.department, new Set())
      }
      if (user.group) {
        departmentMap.get(user.department).add(user.group)
      }
    }
  })

  const tree = [
    {
      key: 'all',
      label: $t('common.all'),
      children: Array.from(departmentMap.entries()).map(([dept, groups]) => ({
        key: `dept-${dept}`,
        label: dept,
        children: Array.from(groups).map(group => ({
          key: `group-${dept}-${group}`,
          label: group,
        })),
      })),
    },
  ]

  return tree
}

// Handle tree node selection
function handleTreeSelect(keys) {
  if (!keys.length) {
    selectedDepartment.value = null
    selectedGroup.value = null
  } else {
    const key = keys[0]

    if (key === 'all') {
      selectedDepartment.value = null
      selectedGroup.value = null
    } else if (key.startsWith('dept-')) {
      selectedDepartment.value = key.replace('dept-', '')
      selectedGroup.value = null
    } else if (key.startsWith('group-')) {
      const parts = key.replace('group-', '').split('-')
      selectedDepartment.value = parts[0]
      selectedGroup.value = parts.slice(1).join('-')
    }
  }

  applyFilters()
}

// Apply all filters (tree, search, pagination)
function applyFilters() {
  let filtered = [...allUsers.value]

  // Apply department/group filter
  if (selectedDepartment.value) {
    filtered = filtered.filter(user => user.department === selectedDepartment.value)

    if (selectedGroup.value) {
      filtered = filtered.filter(user => user.group === selectedGroup.value)
    }
  }

  // Apply search filters
  if (model.value.name) {
    filtered = filtered.filter(user =>
      user.userName?.toLowerCase().includes(model.value.name.toLowerCase())
    )
  }

  if (model.value.email) {
    filtered = filtered.filter(user =>
      user.email?.toLowerCase().includes(model.value.email.toLowerCase())
    )
  }

  if (model.value.role) {
    filtered = filtered.filter(user =>
      user.role === model.value.role
    )
  }

  filteredUsers.value = filtered
  pagination.value.total = filtered.length
  pagination.value.page = 1 // Reset to first page

  updateDisplayUsers()
}

// Update displayed users based on pagination
function updateDisplayUsers() {
  const start = (pagination.value.page - 1) * pagination.value.pageSize
  const end = start + pagination.value.pageSize
  displayUsers.value = filteredUsers.value.slice(start, end)
}

// Handle pagination change
function changePage(page, pageSize) {
  pagination.value.page = page
  pagination.value.pageSize = pageSize
  updateDisplayUsers()
}

// Handle user status update
async function handleUpdateStatus(value, row) {
  try {
    await fetchUpdateUser({
      name: row.userName,
      email: row.email,
      department: row.department,
      group: row.group,
      role: row.role,
      status: value
    })

    // Update in all data arrays
    const updateUserStatus = (users) => {
      const index = users.findIndex(item => item.id === row.id)
      if (index > -1) {
        users[index].status = value
      }
    }

    updateUserStatus(allUsers.value)
    updateUserStatus(filteredUsers.value)
    updateUserStatus(displayUsers.value)

    window.$message.success('User status updated')
  } catch (error) {
    window.$message.error('Update failed, please retry')
  }
}

// Handle user deletion
async function deleteUser(id) {
  try {
    await fetchDeleteUser(id)

    // Remove from all data arrays
    allUsers.value = allUsers.value.filter(item => item.id !== id)

    // Reapply filters and update display
    applyFilters()

    window.$message.success('User deleted')
  } catch (error) {
    window.$message.error('Delete failed, please retry')
  }
}

// Auth source labels and colors
const authSourceConfig = {
  local: { label: 'Local', type: 'default' },
  ldap: { label: 'LDAP', type: 'info' },
  oidc: { label: 'OIDC', type: 'success' },
}

// Table columns configuration
const columns = [
  {
    title: 'ID',
    align: 'center',
    key: 'id',
    width: 60,
  },
  {
    title: 'Name',
    align: 'center',
    key: 'userName',
  },
  {
    title: 'Email',
    align: 'center',
    key: 'email',
  },
  {
    title: 'Department',
    align: 'center',
    key: 'department',
    width: 120,
  },
  {
    title: 'Group',
    align: 'center',
    key: 'group',
    width: 120,
  },
  {
    title: 'Auth Source',
    align: 'center',
    key: 'auth_source',
    width: 120,
    render: (row) => {
      const source = row.auth_source || 'local'
      const config = authSourceConfig[source] || authSourceConfig.local
      return (
        <NTag type={config.type} size="small">
          {config.label}
          {row.auth_provider ? ` (${row.auth_provider})` : ''}
        </NTag>
      )
    },
  },
  {
    title: 'Role',
    align: 'center',
    key: 'role',
    width: 120,
    render: (row) => {
      const roleColors = {
        admin: 'error',
        super: 'warning',
        user: 'info',
      }

      if (row.role) {
        return (
          <NTag type={roleColors[row.role] || 'default'} size="small">
            {row.role}
          </NTag>
        )
      }
      return null
    },
  },
  {
    title: 'Status',
    align: 'center',
    key: 'status',
    width: 100,
    render: (row) => {
      return (
        <NSwitch
          value={row.status}
          checked-value={1}
          unchecked-value={0}
          onUpdateValue={(value) => handleUpdateStatus(value, row)}
        >
          {{ checked: () => 'Active', unchecked: () => 'Disabled' }}
        </NSwitch>
      )
    },
  },
  {
    title: 'Updated',
    align: 'center',
    key: 'updated_at',
    width: 200,
    render: (row) => {
      if (row.updated_at) {
        return new Date(row.updated_at).toLocaleString()
      }
      return '-'
    },
  },
  {
    title: 'Actions',
    align: 'center',
    key: 'actions',
    width: 180,
    render: (row) => {
      return (
        <NSpace justify="center">
          <NButton
            size="small"
            onClick={() => modalRef.value.openModal('edit', row)}
          >
            Edit
          </NButton>
          <NPopconfirm onPositiveClick={() => deleteUser(row.id)}>
            {{
              default: () => 'Confirm delete this user?',
              trigger: () => <NButton size="small" type="error">Delete</NButton>,
            }}
          </NPopconfirm>
        </NSpace>
      )
    },
  },
]

// Fetch user list
async function getUserList() {
  startLoading()
  try {
    const { data } = await fetchUserList()

    if (data && data.users) {
      // Transform API data to match User format
      allUsers.value = data.users.map((user, index) => ({
        id: user.id || index + 1, // Use API id if provided, otherwise generate
        userName: user.name,
        email: user.email,
        department: user.department,
        group: user.group,
        role: user.role || 'user', // Single role value
        status: user.status ?? 1,
        updated_at: user.updated_at,
        auth_source: user.auth_source || 'local',
        auth_provider: user.auth_provider || '',
      }))

      // Generate tree data
      treeData.value = generateTreeData(allUsers.value)

      // Apply initial filters
      applyFilters()
    }
  } catch (error) {
    window.$message.error('Failed to fetch user list')
    allUsers.value = []
    filteredUsers.value = []
    displayUsers.value = []
  } finally {
    endLoading()
  }
}

// Handle modal close - refresh data
function handleModalClose() {
  getUserList()
}

onMounted(() => {
  getUserList()
})
</script>

<template>
  <n-flex>
    <!-- Left side: Department/Group tree -->
    <n-card class="w-70" title="Departments">
      <n-tree
        block-line
        :data="treeData"
        key-field="key"
        label-field="label"
        children-field="children"
        selectable
        @update:selected-keys="handleTreeSelect"
      />
    </n-card>

    <!-- Right side: Search and table -->
    <NSpace vertical class="flex-1">
      <!-- Search form -->
      <n-card>
        <n-form
          ref="formRef"
          :model="model"
          label-placement="left"
          inline
          :show-feedback="false"
        >
          <n-flex>
            <n-form-item label="Name" path="name">
              <n-input
                v-model:value="model.name"
                placeholder="Enter name"
                clearable
              />
            </n-form-item>
            <n-form-item label="Email" path="email">
              <n-input
                v-model:value="model.email"
                placeholder="Enter email"
                clearable
              />
            </n-form-item>
            <n-form-item label="Role" path="role">
              <n-select
                v-model:value="model.role"
                :options="roleOptions"
                placeholder="Select role"
                class="w-120px"
                clearable
              />
            </n-form-item>
            <n-flex class="ml-auto">
              <NButton type="primary" @click="applyFilters">
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

      <!-- Data table -->
      <n-card class="flex-1">
        <template #header>
          <n-flex justify="space-between" align="center">
            <NButton type="primary" @click="modalRef.openModal('add')">
              <template #icon>
                <icon-park-outline-add-one />
              </template>
              Add User
            </NButton>
            <n-text depth="3">
              Total {{ pagination.total }} records
            </n-text>
          </n-flex>
        </template>

        <NSpace vertical>
          <n-data-table
            :columns="columns"
            :data="displayUsers"
            :loading="loading"
            :pagination="false"
          />

          <n-flex justify="end">
            <n-pagination
              v-model:page="pagination.page"
              v-model:page-size="pagination.pageSize"
              :page-count="Math.ceil(pagination.total / pagination.pageSize)"
              :page-sizes="[10, 20, 30, 50]"
              show-size-picker
              @update:page="changePage"
              @update:page-size="changePage"
            />
          </n-flex>
        </NSpace>

        <TableModal
          ref="modalRef"
          :modal-name="$t('setting.userEntity')"
          @close="handleModalClose"
        />
      </n-card>
    </NSpace>
  </n-flex>
</template>
