<script setup>
import { ref, onMounted, computed } from 'vue'
import { fetchDashboardStats } from '@/api/dashboard'
import { useBoolean } from '@/hooks'
import { useAuthStore } from '@/stores/auth'

const { bool: loading, setTrue: startLoading, setFalse: endLoading } = useBoolean(true)
const authStore = useAuthStore()

// Check if current user is admin or super
const isAdminOrSuper = computed(() => authStore.isAdminOrSuper)

// Track if response contains admin data
const isAdmin = ref(false)

const stats = ref({
  total_users: 0,
  active_users: 0,
  total_pipelines: 0,
  ready_pipelines: 0,
  total_jobs: 0,
  recent_jobs: 0,
  successful_jobs: 0,
  failed_jobs: 0,
  pending_jobs: 0,
  local_users: 0,
  ldap_users: 0,
  oidc_users: 0,
  jobs_today: 0,
  jobs_this_week: 0,
  jobs_this_month: 0,
  pipelines_pending: 0,
  pipelines_ready: 0,
  pipelines_failed: 0,
  pipelines_deleting: 0,
})

const recentJobs = ref([])

// Computed job success rate
const successRate = computed(() => {
  const total = stats.value.successful_jobs + stats.value.failed_jobs
  if (total === 0) return 0
  return Math.round((stats.value.successful_jobs / total) * 100)
})

// Table columns for recent jobs
const jobColumns = [
  {
    title: 'Pipeline',
    key: 'pipeline_name',
    render: (row) => `${row.pipeline_name} (v${row.pipeline_version || 'N/A'})`
  },
  {
    title: 'User',
    key: 'user_email',
  },
  {
    title: 'Status',
    key: 'status',
    render: (row) => {
      const statusMap = {
        'submitted': { type: 'default', text: 'Submitted' },
        'dispatch_success': { type: 'info', text: 'Dispatched' },
        'pending': { type: 'warning', text: 'Pending' },
        'running': { type: 'info', text: 'Running' },
        'completed': { type: 'success', text: 'Completed' },
        'failed': { type: 'error', text: 'Failed' },
      }
      const config = statusMap[row.status] || { type: 'default', text: row.status }
      return h('n-tag', { type: config.type, size: 'small' }, { default: () => config.text })
    }
  },
  {
    title: 'Created',
    key: 'created_at',
    render: (row) => {
      if (!row.created_at) return '-'
      return new Date(row.created_at).toLocaleString()
    }
  },
]

import { h } from 'vue'

async function loadDashboardStats() {
  startLoading()
  try {
    const { isSuccess, data } = await fetchDashboardStats()
    if (isSuccess && data) {
      stats.value = data.stats || stats.value
      recentJobs.value = data.recent_jobs || []
      isAdmin.value = data.is_admin || false
    }
  } catch (error) {
    console.error('Failed to load dashboard stats:', error)
  } finally {
    endLoading()
  }
}

onMounted(() => {
  loadDashboardStats()
})
</script>

<template>
  <n-spin :show="loading">
    <n-grid :x-gap="16" :y-gap="16" :cols="24" item-responsive responsive="screen">
      <!-- Summary Stats Row -->
      <!-- Total Users Card - Admin/Super only -->
      <n-gi v-if="isAdmin" span="24 m:6">
        <n-card class="stat-card stat-card--users">
          <n-space vertical align="center" class="py-2">
            <n-icon-wrapper :size="52" color="rgba(24, 160, 88, 0.15)" :border-radius="12">
              <nova-icon :size="28" icon="icon-park-outline:every-user" color="#18a058" />
            </n-icon-wrapper>
            <n-statistic label="Total Users" tabular-nums>
              <n-number-animation :from="0" :to="stats.total_users" show-separator />
            </n-statistic>
            <n-space :size="4">
              <n-tag size="small" :bordered="false" type="success">
                {{ stats.active_users }} active
              </n-tag>
            </n-space>
          </n-space>
        </n-card>
      </n-gi>

      <n-gi :span="isAdmin ? '24 m:6' : '24 m:8'">
        <n-card class="stat-card stat-card--pipelines">
          <n-space vertical align="center" class="py-2">
            <n-icon-wrapper :size="52" color="rgba(32, 128, 240, 0.15)" :border-radius="12">
              <nova-icon :size="28" icon="carbon:pipelines" color="#2080f0" />
            </n-icon-wrapper>
            <n-statistic label="Pipelines" tabular-nums>
              <n-number-animation :from="0" :to="stats.total_pipelines" show-separator />
            </n-statistic>
            <n-space :size="4">
              <n-tag size="small" :bordered="false" type="success">
                {{ stats.ready_pipelines }} ready
              </n-tag>
            </n-space>
          </n-space>
        </n-card>
      </n-gi>

      <n-gi :span="isAdmin ? '24 m:6' : '24 m:8'">
        <n-card class="stat-card stat-card--jobs">
          <n-space vertical align="center" class="py-2">
            <n-icon-wrapper :size="52" color="rgba(245, 158, 11, 0.15)" :border-radius="12">
              <nova-icon :size="28" icon="carbon:batch-job" color="#f59e0b" />
            </n-icon-wrapper>
            <n-statistic :label="isAdmin ? 'Total Jobs' : 'My Jobs'" tabular-nums>
              <n-number-animation :from="0" :to="stats.total_jobs" show-separator />
            </n-statistic>
            <n-space :size="4">
              <n-tag size="small" :bordered="false" type="info">
                {{ stats.jobs_today }} today
              </n-tag>
            </n-space>
          </n-space>
        </n-card>
      </n-gi>

      <n-gi :span="isAdmin ? '24 m:6' : '24 m:8'">
        <n-card class="stat-card stat-card--success">
          <n-space vertical align="center" class="py-2">
            <n-icon-wrapper :size="52" color="rgba(99, 102, 241, 0.15)" :border-radius="12">
              <nova-icon :size="28" icon="icon-park-outline:chart-pie" color="#6366f1" />
            </n-icon-wrapper>
            <n-statistic label="Success Rate" tabular-nums>
              <template #suffix>%</template>
              <n-number-animation :from="0" :to="successRate" />
            </n-statistic>
            <n-space :size="4">
              <n-tag size="small" :bordered="false" type="success">
                {{ stats.successful_jobs }} passed
              </n-tag>
            </n-space>
          </n-space>
        </n-card>
      </n-gi>

      <!-- Jobs Activity Section -->
      <n-gi span="24 m:16">
        <n-card title="Job Activity Overview">
          <template #header-extra>
            <n-button quaternary size="small" @click="loadDashboardStats">
              <template #icon>
                <icon-park-outline-refresh />
              </template>
              Refresh
            </n-button>
          </template>

          <n-grid :cols="3" :x-gap="16" :y-gap="16" class="mb-4">
            <n-gi>
              <n-card embedded :bordered="false">
                <n-space vertical align="center">
                  <n-text depth="3" class="text-sm">Today</n-text>
                  <n-text strong class="text-2xl">
                    <n-number-animation :from="0" :to="stats.jobs_today" />
                  </n-text>
                  <n-text depth="3" class="text-xs">jobs submitted</n-text>
                </n-space>
              </n-card>
            </n-gi>
            <n-gi>
              <n-card embedded :bordered="false">
                <n-space vertical align="center">
                  <n-text depth="3" class="text-sm">This Week</n-text>
                  <n-text strong class="text-2xl">
                    <n-number-animation :from="0" :to="stats.jobs_this_week" />
                  </n-text>
                  <n-text depth="3" class="text-xs">jobs submitted</n-text>
                </n-space>
              </n-card>
            </n-gi>
            <n-gi>
              <n-card embedded :bordered="false">
                <n-space vertical align="center">
                  <n-text depth="3" class="text-sm">This Month</n-text>
                  <n-text strong class="text-2xl">
                    <n-number-animation :from="0" :to="stats.jobs_this_month" />
                  </n-text>
                  <n-text depth="3" class="text-xs">jobs submitted</n-text>
                </n-space>
              </n-card>
            </n-gi>
          </n-grid>

          <n-divider class="!my-3" />

          <!-- Job Status Breakdown -->
          <n-space vertical :size="12">
            <n-text strong>Job Status Breakdown</n-text>
            <n-grid :cols="4" :x-gap="12">
              <n-gi>
                <n-space align="center" :size="8">
                  <n-badge dot type="info" />
                  <n-text>Pending</n-text>
                  <n-text strong>{{ stats.pending_jobs }}</n-text>
                </n-space>
              </n-gi>
              <n-gi>
                <n-space align="center" :size="8">
                  <n-badge dot type="success" />
                  <n-text>Successful</n-text>
                  <n-text strong>{{ stats.successful_jobs }}</n-text>
                </n-space>
              </n-gi>
              <n-gi>
                <n-space align="center" :size="8">
                  <n-badge dot type="error" />
                  <n-text>Failed</n-text>
                  <n-text strong>{{ stats.failed_jobs }}</n-text>
                </n-space>
              </n-gi>
              <n-gi>
                <n-space align="center" :size="8">
                  <n-badge dot type="default" />
                  <n-text>Total</n-text>
                  <n-text strong>{{ stats.total_jobs }}</n-text>
                </n-space>
              </n-gi>
            </n-grid>
          </n-space>
        </n-card>
      </n-gi>

      <!-- Right Sidebar -->
      <n-gi span="24 m:8">
        <n-space vertical :size="16" class="h-full">
          <!-- User Sources - Admin/Super only -->
          <n-card v-if="isAdmin" title="User Sources">
            <n-space vertical :size="12">
              <n-space justify="space-between">
                <n-space align="center" :size="8">
                  <n-icon-wrapper :size="24" color="rgba(24, 160, 88, 0.15)" :border-radius="6">
                    <nova-icon :size="14" icon="icon-park-outline:mail" color="#18a058" />
                  </n-icon-wrapper>
                  <n-text>Local (Email)</n-text>
                </n-space>
                <n-text strong>{{ stats.local_users }}</n-text>
              </n-space>

              <n-space justify="space-between">
                <n-space align="center" :size="8">
                  <n-icon-wrapper :size="24" color="rgba(32, 128, 240, 0.15)" :border-radius="6">
                    <nova-icon :size="14" icon="icon-park-outline:folder-lock" color="#2080f0" />
                  </n-icon-wrapper>
                  <n-text>LDAP</n-text>
                </n-space>
                <n-text strong>{{ stats.ldap_users }}</n-text>
              </n-space>

              <n-space justify="space-between">
                <n-space align="center" :size="8">
                  <n-icon-wrapper :size="24" color="rgba(245, 158, 11, 0.15)" :border-radius="6">
                    <nova-icon :size="14" icon="icon-park-outline:link-cloud" color="#f59e0b" />
                  </n-icon-wrapper>
                  <n-text>OIDC (SSO)</n-text>
                </n-space>
                <n-text strong>{{ stats.oidc_users }}</n-text>
              </n-space>
            </n-space>
          </n-card>

          <!-- Pipeline Status -->
          <n-card title="Pipeline Status">
            <n-space vertical :size="12">
              <n-space justify="space-between">
                <n-space align="center" :size="8">
                  <n-badge dot type="success" />
                  <n-text>Ready</n-text>
                </n-space>
                <n-text strong>{{ stats.pipelines_ready }}</n-text>
              </n-space>

              <n-space justify="space-between">
                <n-space align="center" :size="8">
                  <n-badge dot type="warning" />
                  <n-text>Pending</n-text>
                </n-space>
                <n-text strong>{{ stats.pipelines_pending }}</n-text>
              </n-space>

              <n-space justify="space-between">
                <n-space align="center" :size="8">
                  <n-badge dot type="error" />
                  <n-text>Failed</n-text>
                </n-space>
                <n-text strong>{{ stats.pipelines_failed }}</n-text>
              </n-space>

              <n-space justify="space-between">
                <n-space align="center" :size="8">
                  <n-badge dot type="info" />
                  <n-text>Deleting</n-text>
                </n-space>
                <n-text strong>{{ stats.pipelines_deleting }}</n-text>
              </n-space>
            </n-space>
          </n-card>
        </n-space>
      </n-gi>

      <!-- Recent Jobs Table -->
      <n-gi span="24">
        <n-card :title="isAdmin ? 'Recent Jobs' : 'My Recent Jobs'">
          <template #header-extra>
            <n-text depth="3">Last 10 jobs</n-text>
          </template>
          <n-data-table
            :columns="jobColumns"
            :data="recentJobs"
            :loading="loading"
            :bordered="false"
            size="small"
            :pagination="false"
          />
          <template v-if="recentJobs.length === 0 && !loading" #default>
            <n-empty description="No jobs found" />
          </template>
        </n-card>
      </n-gi>
    </n-grid>
  </n-spin>
</template>

<style scoped>
.stat-card {
  transition: transform 0.2s, box-shadow 0.2s;
}

.stat-card:hover {
  transform: translateY(-2px);
}

/* Center the statistic label and value under the icon (Naive UI left-aligns them by default) */
.stat-card :deep(.n-statistic) {
  text-align: center;
}

.stat-card :deep(.n-statistic .n-statistic__label) {
  display: block;
  text-align: center;
}

.stat-card :deep(.n-statistic .n-statistic-value) {
  display: flex;
  justify-content: center;
}
</style>
