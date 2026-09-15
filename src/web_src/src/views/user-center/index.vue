<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { fetchProfile, fetchUpdateProfile, fetchChangePassword } from '@/api/user'
import { useAuthStore } from '@/stores'
import { useBoolean } from '@/hooks'

const { t } = useI18n()
const authStore = useAuthStore()

const profile = ref({
  id: null,
  name: '',
  email: '',
  department: '',
  group: '',
  role: '',
  auth_source: 'local',
  auth_provider: '',
})

const isLocalAuth = computed(() => (profile.value.auth_source || 'local') === 'local')

const authSourceLabel = computed(() => {
  const map = { local: 'Local', ldap: 'LDAP', oidc: 'OIDC' }
  const source = profile.value.auth_source || 'local'
  const base = map[source] || source
  return profile.value.auth_provider ? `${base} (${profile.value.auth_provider})` : base
})

// ── Basic information ─────────────────────────────────────────────
const profileForm = reactive({ name: '', department: '' })
const profileFormRef = ref(null)
const { bool: savingProfile, setTrue: startSaveProfile, setFalse: endSaveProfile } = useBoolean(false)

const profileRules = {
  name: {
    required: true,
    message: t('userCenter.nameRequired'),
    trigger: ['input', 'blur'],
  },
}

async function loadProfile() {
  const { isSuccess, data } = await fetchProfile()
  if (isSuccess && data?.profile) {
    profile.value = data.profile
    profileForm.name = data.profile.name || ''
    profileForm.department = data.profile.department || ''
  }
}

async function handleSaveProfile() {
  await profileFormRef.value?.validate()
  startSaveProfile()
  try {
    const { isSuccess } = await fetchUpdateProfile({
      name: profileForm.name,
      department: profileForm.department,
    })
    if (isSuccess) {
      window.$message.success(t('userCenter.saveSuccess'))
      profile.value.name = profileForm.name
      profile.value.department = profileForm.department
      // keep the header avatar / cached profile in sync
      authStore.updateUserInfo({ name: profileForm.name, department: profileForm.department })
    }
  } finally {
    endSaveProfile()
  }
}

// ── Change password (local accounts only) ─────────────────────────
const pwdForm = reactive({ old_password: '', new_password: '', confirm_password: '' })
const pwdFormRef = ref(null)
const { bool: savingPwd, setTrue: startSavePwd, setFalse: endSavePwd } = useBoolean(false)

const pwdRules = {
  old_password: {
    required: true,
    message: t('userCenter.oldPasswordRequired'),
    trigger: ['input', 'blur'],
  },
  new_password: {
    required: true,
    trigger: ['input', 'blur'],
    validator(rule, value) {
      if (!value) return new Error(t('userCenter.newPasswordRequired'))
      if (value.length < 6) return new Error(t('userCenter.passwordTooShort'))
      return true
    },
  },
  confirm_password: {
    required: true,
    trigger: ['input', 'blur'],
    validator(rule, value) {
      if (!value) return new Error(t('userCenter.confirmPasswordRequired'))
      if (value !== pwdForm.new_password) return new Error(t('userCenter.passwordNotMatch'))
      return true
    },
  },
}

async function handleChangePassword() {
  await pwdFormRef.value?.validate()
  startSavePwd()
  try {
    const { isSuccess } = await fetchChangePassword({
      old_password: pwdForm.old_password,
      new_password: pwdForm.new_password,
    })
    if (isSuccess) {
      window.$message.success(t('userCenter.passwordSuccess'))
      pwdForm.old_password = ''
      pwdForm.new_password = ''
      pwdForm.confirm_password = ''
      pwdFormRef.value?.restoreValidation()
    }
  } finally {
    endSavePwd()
  }
}

onMounted(loadProfile)
</script>

<template>
  <n-grid :x-gap="16" :y-gap="16" :cols="24" item-responsive responsive="screen">
    <!-- Basic information -->
    <n-gi span="24 m:12">
      <n-card :title="t('userCenter.basicInfo')">
        <n-form
          ref="profileFormRef"
          :model="profileForm"
          :rules="profileRules"
          label-placement="left"
          label-width="auto"
          require-mark-placement="right-hanging"
        >
          <n-form-item :label="t('userCenter.email')">
            <n-input :value="profile.email" disabled />
          </n-form-item>
          <n-form-item :label="t('userCenter.role')">
            <n-input :value="profile.role" disabled />
          </n-form-item>
          <n-form-item :label="t('userCenter.authSource')">
            <n-input :value="authSourceLabel" disabled />
          </n-form-item>
          <n-form-item :label="t('userCenter.name')" path="name">
            <n-input v-model:value="profileForm.name" :placeholder="t('userCenter.namePlaceholder')" />
          </n-form-item>
          <n-form-item :label="t('userCenter.department')" path="department">
            <n-input v-model:value="profileForm.department" :placeholder="t('userCenter.departmentPlaceholder')" />
          </n-form-item>
          <n-form-item :show-label="false">
            <n-button type="primary" :loading="savingProfile" @click="handleSaveProfile">
              {{ t('userCenter.save') }}
            </n-button>
          </n-form-item>
        </n-form>
      </n-card>
    </n-gi>

    <!-- Change password -->
    <n-gi span="24 m:12">
      <n-card :title="t('userCenter.changePassword')">
        <n-form
          v-if="isLocalAuth"
          ref="pwdFormRef"
          :model="pwdForm"
          :rules="pwdRules"
          label-placement="left"
          label-width="auto"
          require-mark-placement="right-hanging"
        >
          <n-form-item :label="t('userCenter.oldPassword')" path="old_password">
            <n-input
              v-model:value="pwdForm.old_password"
              type="password"
              show-password-on="click"
              :placeholder="t('userCenter.oldPasswordPlaceholder')"
            />
          </n-form-item>
          <n-form-item :label="t('userCenter.newPassword')" path="new_password">
            <n-input
              v-model:value="pwdForm.new_password"
              type="password"
              show-password-on="click"
              :placeholder="t('userCenter.newPasswordPlaceholder')"
            />
          </n-form-item>
          <n-form-item :label="t('userCenter.confirmPassword')" path="confirm_password">
            <n-input
              v-model:value="pwdForm.confirm_password"
              type="password"
              show-password-on="click"
              :placeholder="t('userCenter.confirmPasswordPlaceholder')"
            />
          </n-form-item>
          <n-form-item :show-label="false">
            <n-button type="primary" :loading="savingPwd" @click="handleChangePassword">
              {{ t('userCenter.updatePassword') }}
            </n-button>
          </n-form-item>
        </n-form>
        <n-alert v-else type="info" :bordered="false">
          {{ t('userCenter.externalAuthNote') }}
        </n-alert>
      </n-card>
    </n-gi>
  </n-grid>
</template>

<style scoped></style>
