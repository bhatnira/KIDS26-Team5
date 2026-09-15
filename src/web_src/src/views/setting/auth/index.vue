<script setup lang="tsx">
import { useBoolean } from '@/hooks'
import {
  fetchAuthProviderList,
  fetchDeleteAuthProvider,
  fetchTestAuthProvider
} from '@/api/admin'
import { NButton, NPopconfirm, NSpace, NSwitch, NTag } from 'naive-ui'
import { ref, onMounted } from 'vue'
import ProviderModal from './components/ProviderModal.vue'

const { bool: loading, setTrue: startLoading, setFalse: endLoading } = useBoolean(false)

const providers = ref([])
const modalRef = ref()

// Table columns
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
    key: 'name',
  },
  {
    title: 'Type',
    align: 'center',
    key: 'type',
    render: (row) => {
      const typeColors = {
        basic: 'default',
        ldap: 'info',
        oidc: 'success',
      }
      const typeLabels = {
        basic: 'Basic',
        ldap: 'LDAP',
        oidc: 'OIDC',
      }
      return (
        <NTag type={typeColors[row.type] || 'default'} size="small">
          {typeLabels[row.type] || row.type}
        </NTag>
      )
    },
  },
  {
    title: 'Display Name',
    align: 'center',
    key: 'display_name',
  },
  {
    title: 'Priority',
    align: 'center',
    key: 'priority',
    width: 80,
  },
  {
    title: 'Enabled',
    align: 'center',
    key: 'enabled',
    render: (row) => {
      return (
        <NTag type={row.enabled ? 'success' : 'default'} size="small">
          {row.enabled ? 'Yes' : 'No'}
        </NTag>
      )
    },
  },
  {
    title: 'Actions',
    align: 'center',
    key: 'actions',
    width: 280,
    render: (row) => {
      return (
        <NSpace justify="center">
          <NButton size="small" type="info" onClick={() => testProvider(row.id)}>
            Test
          </NButton>
          <NButton size="small" onClick={() => modalRef.value.openModal('edit', row)}>
            Edit
          </NButton>
          <NPopconfirm onPositiveClick={() => deleteProvider(row.id)}>
            {{
              default: () => 'Confirm delete this provider?',
              trigger: () => <NButton size="small" type="error">Delete</NButton>,
            }}
          </NPopconfirm>
        </NSpace>
      )
    },
  },
]

async function getProviderList() {
  startLoading()
  try {
    const { data } = await fetchAuthProviderList()
    if (data?.providers) {
      providers.value = data.providers
    }
  } catch (error) {
    window.$message.error('Failed to fetch auth providers')
  } finally {
    endLoading()
  }
}

async function testProvider(id) {
  try {
    const { isSuccess, msg } = await fetchTestAuthProvider(id)
    if (isSuccess) {
      window.$message.success(msg || 'Connection successful')
    }
  } catch (error) {
    window.$message.error('Connection test failed')
  }
}

async function deleteProvider(id) {
  try {
    await fetchDeleteAuthProvider(id)
    window.$message.success('Provider deleted')
    getProviderList()
  } catch (error) {
    window.$message.error('Delete failed')
  }
}

function handleModalClose() {
  getProviderList()
}

onMounted(() => {
  getProviderList()
})
</script>

<template>
  <n-space vertical>
    <n-card>
      <template #header>
        <n-flex justify="space-between" align="center">
          <n-text strong>Authentication Providers</n-text>
          <NButton type="primary" @click="modalRef.openModal('add')">
            <template #icon>
              <icon-park-outline-add-one />
            </template>
            Add Provider
          </NButton>
        </n-flex>
      </template>

      <n-alert type="info" class="mb-4">
        Configure authentication providers to allow users to login via LDAP, OIDC, or basic email/password authentication.
      </n-alert>

      <n-data-table
        :columns="columns"
        :data="providers"
        :loading="loading"
        :pagination="false"
      />
    </n-card>

    <ProviderModal
      ref="modalRef"
      @close="handleModalClose"
    />
  </n-space>
</template>
