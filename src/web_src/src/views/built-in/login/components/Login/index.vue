<script setup>
import { useAuthStore } from '@/stores'
import { local } from '@/utils/storage'
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMessage } from 'naive-ui'
import { fetchAuthProviders, fetchLdapLogin, fetchOidcCallback, fetchOidcStart } from '@/api/login'
import { useRoute } from 'vue-router'

const emit = defineEmits(['update:modelValue'])
const authStore = useAuthStore()
const route = useRoute()

function toOtherForm(type) {
  emit('update:modelValue', type)
}

const { t } = useI18n()
const message = useMessage()

// Auth providers state
const authProviders = ref([])
const selectedProvider = ref('basic')
const providersLoading = ref(true)

// Form rules
const rules = computed(() => {
  return {
    email: {
      required: true,
      trigger: 'blur',
      message: t('login.emailRuleTip'),
    },
    pwd: {
      required: true,
      trigger: 'blur',
      message: t('login.passwordRuleTip'),
    },
    username: {
      required: true,
      trigger: 'blur',
      message: t('login.accountRuleTip'),
    },
  }
})

const formValue = ref({
  email: '',
  pwd: '',
  username: '',
})
const isRemember = ref(false)
const isLoading = ref(false)
const formRef = ref(null)

// Computed properties for provider types
const hasBasicProvider = computed(() =>
  authProviders.value.some(p => p.type === 'basic') || authProviders.value.length === 0
)
const ldapProviders = computed(() =>
  authProviders.value.filter(p => p.type === 'ldap')
)
const oidcProviders = computed(() =>
  authProviders.value.filter(p => p.type === 'oidc')
)
const isLdapLogin = computed(() => selectedProvider.value.startsWith('ldap:'))
const isOidcLogin = computed(() => selectedProvider.value.startsWith('oidc:'))

// Provider options for tabs/select
const providerOptions = computed(() => {
  const options = []

  if (hasBasicProvider.value) {
    options.push({ label: 'Email Login', value: 'basic' })
  }

  ldapProviders.value.forEach(p => {
    options.push({
      label: p.display_name || p.name,
      value: `ldap:${p.name}`
    })
  })

  oidcProviders.value.forEach(p => {
    options.push({
      label: p.display_name || p.name,
      value: `oidc:${p.name}`
    })
  })

  return options
})

async function loadAuthProviders() {
  try {
    providersLoading.value = true
    const { isSuccess, data } = await fetchAuthProviders()
    if (isSuccess && data?.providers) {
      authProviders.value = data.providers

      // Check for OIDC callback
      if (route.query.code && route.query.state) {
        await handleOidcCallback(route.query.code, route.query.state)
      }
    }
  } catch (error) {
    console.error('Failed to load auth providers:', error)
  } finally {
    providersLoading.value = false
  }
}

async function handleLogin() {
  formRef.value?.validate(async (errors) => {
    if (errors)
      return

    try {
      isLoading.value = true

      if (isLdapLogin.value) {
        await handleLdapLogin()
      } else if (isOidcLogin.value) {
        await handleOidcRedirect()
      } else {
        await handleBasicLogin()
      }
    } catch (error) {
      message.error(error.message || t('login.registerFailed'))
    } finally {
      isLoading.value = false
    }
  })
}

async function handleBasicLogin() {
  const { email, pwd } = formValue.value

  if (isRemember.value)
    local.set('loginAccount', { email, pwd })
  else
    local.remove('loginAccount')

  await authStore.login(email, pwd)
}

async function handleLdapLogin() {
  const providerName = selectedProvider.value.replace('ldap:', '')
  const { username, pwd } = formValue.value

  const { isSuccess, data } = await fetchLdapLogin({
    username,
    password: pwd,
    provider: providerName,
  })

  if (isSuccess) {
    await authStore.handleLoginInfo(data)
    message.success('Login successful')
  }
}

async function handleOidcRedirect() {
  const providerName = selectedProvider.value.replace('oidc:', '')
  try {
    // Fetch a fresh authorization URL carrying a one-time state nonce, then
    // redirect the browser to the IdP.
    const { isSuccess, data } = await fetchOidcStart(providerName)
    if (isSuccess && data?.auth_url) {
      window.location.href = data.auth_url
    } else {
      message.error('OIDC configuration error')
    }
  } catch (error) {
    message.error('OIDC configuration error')
  }
}

async function handleOidcCallback(code, state) {
  try {
    isLoading.value = true
    const { isSuccess, data } = await fetchOidcCallback({
      code,
      state,
    })

    if (isSuccess) {
      await authStore.handleLoginInfo(data)
      message.success('Login successful')
    }
  } catch (error) {
    message.error('OIDC authentication failed')
  } finally {
    isLoading.value = false
  }
}

onMounted(() => {
  loadAuthProviders()
  checkUserAccount()
})

function checkUserAccount() {
  const loginAccount = local.get('loginAccount')
  if (!loginAccount)
    return

  formValue.value.email = loginAccount.email
  formValue.value.pwd = loginAccount.pwd
  isRemember.value = true
}
</script>

<template>
  <div>
    <n-h2 depth="3" class="text-center">
      {{ $t('login.signInTitle') }}
    </n-h2>

    <!-- Provider selector (shown only if multiple providers) -->
    <n-form-item v-if="providerOptions.length > 1" :show-label="false" class="mb-4">
      <n-select
        v-model:value="selectedProvider"
        :options="providerOptions"
        :loading="providersLoading"
        placeholder="Select login method"
      />
    </n-form-item>

    <n-form ref="formRef" :rules="rules" :model="formValue" :show-label="false" size="large">
      <!-- Basic/Email login form -->
      <template v-if="!isLdapLogin && !isOidcLogin">
        <n-form-item path="email">
          <n-input v-model:value="formValue.email" clearable :placeholder="$t('login.emailPlaceholder')" />
        </n-form-item>
        <n-form-item path="pwd">
          <n-input v-model:value="formValue.pwd" type="password" :placeholder="$t('login.passwordPlaceholder')" clearable show-password-on="click">
            <template #password-invisible-icon>
              <icon-park-outline-preview-close-one />
            </template>
            <template #password-visible-icon>
              <icon-park-outline-preview-open />
            </template>
          </n-input>
        </n-form-item>
      </template>

      <!-- LDAP login form -->
      <template v-else-if="isLdapLogin">
        <n-form-item path="username">
          <n-input v-model:value="formValue.username" clearable :placeholder="$t('login.accountPlaceholder')">
            <template #prefix>
              <icon-park-outline-user />
            </template>
          </n-input>
        </n-form-item>
        <n-form-item path="pwd">
          <n-input v-model:value="formValue.pwd" type="password" placeholder="Password" clearable show-password-on="click">
            <template #prefix>
              <icon-park-outline-lock />
            </template>
            <template #password-invisible-icon>
              <icon-park-outline-preview-close-one />
            </template>
            <template #password-visible-icon>
              <icon-park-outline-preview-open />
            </template>
          </n-input>
        </n-form-item>
      </template>

      <!-- OIDC login (just a redirect button) -->
      <template v-else-if="isOidcLogin">
        <n-alert type="info" class="mb-4">
          Click the button below to authenticate with your organization's identity provider.
        </n-alert>
      </template>

      <n-space vertical :size="20">
        <div v-if="!isLdapLogin && !isOidcLogin" class="flex-y-center justify-between">
          <n-checkbox v-model:checked="isRemember">
            {{ $t('login.rememberMe') }}
          </n-checkbox>
          <n-button type="primary" text @click="toOtherForm('resetPwd')">
            {{ $t('login.forgotPassword') }}
          </n-button>
        </div>

        <n-button
          block
          type="primary"
          size="large"
          :loading="isLoading"
          :disabled="isLoading"
          @click="handleLogin"
        >
          <template v-if="isOidcLogin">
            <icon-park-outline-link-one class="mr-2" />
            Continue with SSO
          </template>
          <template v-else>
            {{ $t('login.signIn') }}
          </template>
        </n-button>

        <n-flex v-if="!isLdapLogin && !isOidcLogin">
          <n-text>{{ $t('login.noAccountText') }}</n-text>
          <n-button type="primary" text @click="toOtherForm('register')">
            {{ $t('login.signUp') }}
          </n-button>
        </n-flex>
      </n-space>
    </n-form>
  </div>
</template>

<style scoped></style>
