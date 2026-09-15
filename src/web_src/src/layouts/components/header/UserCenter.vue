<script setup lang="ts">
import { useAuthStore } from '@/stores'
import { useI18n } from 'vue-i18n'
import IconBookOpen from '~icons/icon-park-outline/book-open'
import IconGithub from '~icons/icon-park-outline/github'
import IconLogout from '~icons/icon-park-outline/logout'
import IconUser from '~icons/icon-park-outline/user'

const { t } = useI18n()

const authStore = useAuthStore()
const router = useRouter()

// Keep avatar reactive to profile updates made on the Personal Center page.
const userInfo = computed(() => authStore.userInfo)

// First letter of the username (fallback to email), centered in the avatar.
const initial = computed(() => {
  const source = userInfo.value?.name || userInfo.value?.email || ''
  return source.trim().charAt(0).toUpperCase()
})

const options = computed(() => {
  return [
    {
      label: t('app.userCenter'),
      key: 'userCenter',
      icon: () => h(IconUser),
    },
    {
      type: 'divider',
      key: 'd1',
    },
    {
      label: 'Github',
      key: 'github',
      icon: () => h(IconGithub),
    },
    {
      label: 'Docs',
      key: 'docs',
      icon: () => h(IconBookOpen),
    },
    {
      type: 'divider',
      key: 'd2',
    },
    {
      label: t('app.loginOut'),
      key: 'loginOut',
      icon: () => h(IconLogout),
    },
  ]
})
function handleSelect(key: string | number) {
  if (key === 'loginOut') {
    window.$dialog?.info({
      title: t('app.loginOutTitle'),
      content: t('app.loginOutContent'),
      positiveText: t('common.confirm'),
      negativeText: t('common.cancel'),
      onPositiveClick: () => {
        authStore.logout()
      },
    })
  }
  if (key === 'userCenter')
    router.push('/user-center')

  if (key === 'github')
    window.open('https://github.com/HaidYi/antelope')

  if (key === 'docs') {
    // Swagger UI is served by the backend at <backend>/api/docs. Derive it from
    // the API base URL (e.g. http://host/api/v1) so it works in dev and prod,
    // since the frontend talks to the backend directly rather than via proxy.
    const docsUrl = __URL_MAP__.url.path.replace(/\/api\/v1\/?$/, '/api/docs/index.html')
    window.open(docsUrl)
  }
}
</script>

<template>
  <n-dropdown
    trigger="click"
    :options="options"
    @select="handleSelect"
  >
    <n-avatar
      round
      class="cursor-pointer"
      color="#2080f0"
      :src="userInfo?.avatar"
    >
      <template #default>
        <span class="text-16px font-600 text-white">{{ initial }}</span>
      </template>
    </n-avatar>
  </n-dropdown>
</template>

<style scoped></style>
