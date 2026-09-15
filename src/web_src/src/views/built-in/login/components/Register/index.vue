<script setup>
import { fetchRegister, fetchCode } from '@/api/login'
import { useMessage } from 'naive-ui'
import { useI18n } from 'vue-i18n'

const emit = defineEmits(['update:modelValue'])
function toLogin() {
  emit('update:modelValue', 'login')
}
const { t } = useI18n()
const message = useMessage()
const formRef = ref(null)
const countdown = ref(0)
const isSendingCode = ref(false)

const canSendCode = computed(() => countdown.value === 0 && !isSendingCode.value)

const rules = {
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
  rePwd: {
    required: true,
    trigger: ['blur', 'input'],
    validator: (rule, value) => {
      if (!value) {
        return new Error(t('login.checkPasswordRuleTip'))
      }
      if (value !== formValue.value.pwd) {
        return new Error(t('login.passwordNotMatch'))
      }
      return true
    },
  },
  code: {
    required: true,
    trigger: 'blur',
    message: t('login.codeRuleTip'), // Add this i18n key
  },
}
const formValue = ref({
  email: '',
  pwd: '',
  rePwd: '',
  code: '',
})

function startCountdown() {
  countdown.value = 60
  const timer = setInterval(() => {
    countdown.value--
    if (countdown.value <= 0) {
      clearInterval(timer)
    }
  }, 1000)
}

async function handleSendCode() {
  if (!formValue.value.email) {
    message.error(t('login.emailRequired')) // Add this i18n key
    return
  }

  try {
    isSendingCode.value = true
    const res = await fetchCode({ email: formValue.value.email }).send()
    if (res.isSuccess) {
      message.success(t('login.codeSent'))
      startCountdown()
    }
  } catch (error) {
    message.error(error.message || t('login.codeSendFailed'))
  } finally {
    isSendingCode.value = false
  }
}

async function handleRegister() {
  formRef.value?.validate(async (errors) => {
    if (errors) return

    try {
      const { email, pwd, code } = formValue.value
      await fetchRegister({ email, password: pwd, code }).send()
      message.success(t('login.registerSuccess'))
      toLogin()
    } catch (error) {
      message.error(error.message || t('login.registerFailed'))
    }
  })
}
</script>

<template>
  <div>
    <n-h2 depth="3" class="text-center">
      {{ $t('login.registerTitle') }}
    </n-h2>
    <n-form ref="formRef" :rules="rules" :model="formValue" :show-label="false" size="large">
      <n-form-item path="email">
        <n-input
          v-model:value="formValue.email"
          clearable
          :placeholder="t('login.emailPlaceholder')"
        />
      </n-form-item>
      <n-form-item path="pwd">
        <n-input
          v-model:value="formValue.pwd"
          type="password"
          :placeholder="t('login.passwordPlaceholder')"
          clearable
          show-password-on="click"
        >
          <template #password-invisible-icon>
            <icon-park-outline-preview-close-one />
          </template>
          <template #password-visible-icon>
            <icon-park-outline-preview-open />
          </template>
        </n-input>
      </n-form-item>
      <n-form-item path="rePwd">
        <n-input
          v-model:value="formValue.rePwd"
          type="password"
          :placeholder="t('login.checkPasswordPlaceholder')"
          clearable
          show-password-on="click"
        >
          <template #password-invisible-icon>
            <icon-park-outline-preview-close-one />
          </template>
          <template #password-visible-icon>
            <icon-park-outline-preview-open />
          </template>
        </n-input>
      </n-form-item>

      <n-form-item path="code">
        <n-input-group>
          <n-input
            v-model:value="formValue.code"
            clearable
            placeholder="XXXXXX"
            style="flex: 1"
          />
          <n-button :disabled="!canSendCode" :loading="isSendingCode" @click="handleSendCode">
            {{ countdown > 0 ? `${countdown}s` : t('login.sendCode') }}
          </n-button>
        </n-input-group>
      </n-form-item>

      <n-form-item>
        <n-space vertical :size="20" class="w-full">
          <!--          <n-checkbox v-model:checked="isRead">
            {{ $t('login.readAndAgree') }}
            <n-button type="primary" text>
              {{ $t('login.userAgreement') }}
            </n-button>
          </n-checkbox>-->
          <n-button block type="primary" @click="handleRegister">
            {{ t('login.signUp') }}
          </n-button>
          <n-flex justify="center">
            <n-text>{{ t('login.haveAccountText') }}</n-text>
            <n-button text type="primary" @click="toLogin">
              {{ t('login.signIn') }}
            </n-button>
          </n-flex>
        </n-space>
      </n-form-item>
    </n-form>
  </div>
</template>

<style scoped></style>
