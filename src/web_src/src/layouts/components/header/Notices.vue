<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import NoticeList from '../common/NoticeList.vue'
import { useNotificationStore } from '@/stores'

const store = useNotificationStore()

onMounted(() => store.init())
onUnmounted(() => store.cleanup())

const currentTab = ref(0)

function handleRead(id: number) {
  store.markRead(id)
}
</script>

<template>
  <n-popover placement="bottom" trigger="click" arrow-point-to-center class="!p-0">
    <template #trigger>
      <n-tooltip placement="bottom" trigger="hover">
        <template #trigger>
          <CommonWrapper>
            <n-badge :value="store.unreadCount" :max="99" style="color: unset">
              <icon-park-outline-remind />
            </n-badge>
          </CommonWrapper>
        </template>
        <span>{{ $t('app.notificationsTips') }}</span>
      </n-tooltip>
    </template>
    <n-tabs v-model:value="currentTab" type="line" animated justify-content="space-evenly" class="w-390px">
      <n-tab-pane :name="0">
        <template #tab>
          <n-space class="w-130px" justify="center">
            {{ $t('app.notifications') }}
            <n-badge type="info" :value="store.grouped[0]?.filter((i: Entity.Message) => !i.isRead).length" :max="99" />
          </n-space>
        </template>
        <NoticeList :list="store.grouped[0]" @read="handleRead" />
      </n-tab-pane>
      <n-tab-pane :name="1">
        <template #tab>
          <n-space class="w-130px" justify="center">
            {{ $t('app.messages') }}
            <n-badge type="warning" :value="store.grouped[1]?.filter((i: Entity.Message) => !i.isRead).length" :max="99" />
          </n-space>
        </template>
        <NoticeList :list="store.grouped[1]" @read="handleRead" />
      </n-tab-pane>
      <n-tab-pane :name="2">
        <template #tab>
          <n-space class="w-130px" justify="center">
            {{ $t('app.todos') }}
            <n-badge type="error" :value="store.grouped[2]?.filter((i: Entity.Message) => !i.isRead).length" :max="99" />
          </n-space>
        </template>
        <NoticeList :list="store.grouped[2]" @read="handleRead" />
      </n-tab-pane>
    </n-tabs>
  </n-popover>
</template>

<style scoped></style>
