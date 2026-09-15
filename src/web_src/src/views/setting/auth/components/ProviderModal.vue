<script setup>
import { useBoolean } from '@/hooks'
import { fetchAddAuthProvider, fetchUpdateAuthProvider } from '@/api/admin'
import { ref, shallowRef, computed, watch } from 'vue'

const emit = defineEmits(['open', 'close'])

const { bool: modalVisible, setTrue: showModal, setFalse: hiddenModal } = useBoolean(false)
const { bool: submitLoading, setTrue: startLoading, setFalse: endLoading } = useBoolean(false)

const formDefault = {
  id: null,
  name: '',
  type: 'basic',
  enabled: true,
  display_name: '',
  priority: 0,
  // LDAP fields
  ldap_url: '',
  ldap_bind_dn: '',
  ldap_bind_pass: '',
  ldap_base_dn: '',
  ldap_filter: '(uid=%s)',
  ldap_tls_cert: '',
  // OIDC fields
  oidc_issuer_url: '',
  oidc_client_id: '',
  oidc_client_secret: '',
  oidc_redirect_url: '',
}
const formModel = ref({ ...formDefault })

const modalType = shallowRef('add')
const modalTitle = computed(() => {
  const titleMap = {
    add: 'Add',
    edit: 'Edit',
  }
  return `${titleMap[modalType.value]} Auth Provider`
})

const providerTypeOptions = [
  { label: 'Basic (Email/Password)', value: 'basic' },
  { label: 'LDAP', value: 'ldap' },
  { label: 'OIDC (OpenID Connect)', value: 'oidc' },
]

const isLdap = computed(() => formModel.value.type === 'ldap')
const isOidc = computed(() => formModel.value.type === 'oidc')

async function openModal(type = 'add', data = null) {
  emit('open')
  modalType.value = type
  showModal()

  if (type === 'add') {
    formModel.value = { ...formDefault }
  } else if (data) {
    formModel.value = { ...formDefault, ...data }
  }
}

function closeModal() {
  hiddenModal()
  endLoading()
  emit('close')
}

defineExpose({ openModal })

const formRef = ref()

async function submitModal() {
  try {
    await formRef.value?.validate()
    startLoading()

    const payload = { ...formModel.value }

    // Clean up unused fields based on type
    if (!isLdap.value) {
      delete payload.ldap_url
      delete payload.ldap_bind_dn
      delete payload.ldap_bind_pass
      delete payload.ldap_base_dn
      delete payload.ldap_filter
      delete payload.ldap_tls_cert
    }
    if (!isOidc.value) {
      delete payload.oidc_issuer_url
      delete payload.oidc_client_id
      delete payload.oidc_client_secret
      delete payload.oidc_redirect_url
    }

    let success = false
    if (modalType.value === 'add') {
      delete payload.id
      const { isSuccess } = await fetchAddAuthProvider(payload)
      success = isSuccess
      if (success) window.$message.success('Provider added successfully')
    } else {
      const { isSuccess } = await fetchUpdateAuthProvider(payload)
      success = isSuccess
      if (success) window.$message.success('Provider updated successfully')
    }

    if (success) {
      closeModal()
    } else {
      endLoading()
    }
  } catch (error) {
    window.$message.error('Validation failed')
    endLoading()
  }
}

// Form validation rules
const rules = computed(() => ({
  name: {
    required: true,
    message: 'Please enter provider name',
    trigger: 'blur',
  },
  type: {
    required: true,
    message: 'Please select provider type',
    trigger: 'change',
  },
  ldap_url: {
    required: isLdap.value,
    message: 'Please enter LDAP URL',
    trigger: 'blur',
  },
  ldap_bind_dn: {
    required: isLdap.value,
    message: 'Please enter Bind DN',
    trigger: 'blur',
  },
  ldap_bind_pass: {
    required: isLdap.value && modalType.value === 'add',
    message: 'Please enter Bind Password',
    trigger: 'blur',
  },
  ldap_base_dn: {
    required: isLdap.value,
    message: 'Please enter Base DN',
    trigger: 'blur',
  },
  ldap_filter: {
    required: isLdap.value,
    message: 'Please enter search filter',
    trigger: 'blur',
  },
  oidc_issuer_url: {
    required: isOidc.value,
    message: 'Please enter Issuer URL',
    trigger: 'blur',
  },
  oidc_client_id: {
    required: isOidc.value,
    message: 'Please enter Client ID',
    trigger: 'blur',
  },
  oidc_client_secret: {
    required: isOidc.value && modalType.value === 'add',
    message: 'Please enter Client Secret',
    trigger: 'blur',
  },
  oidc_redirect_url: {
    required: isOidc.value,
    message: 'Please enter Redirect URL',
    trigger: 'blur',
  },
}))
</script>

<template>
  <n-modal
    v-model:show="modalVisible"
    :mask-closable="false"
    preset="card"
    :title="modalTitle"
    class="w-700px"
    :segmented="{
      content: true,
      action: true,
    }"
  >
    <n-form
      ref="formRef"
      :rules="rules"
      label-placement="left"
      :model="formModel"
      :label-width="120"
    >
      <n-grid :cols="2" :x-gap="18">
        <!-- Basic Info -->
        <n-form-item-grid-item :span="1" label="Name" path="name">
          <n-input
            v-model:value="formModel.name"
            placeholder="e.g., company-ldap"
            :disabled="modalType === 'edit'"
          />
        </n-form-item-grid-item>

        <n-form-item-grid-item :span="1" label="Type" path="type">
          <n-select
            v-model:value="formModel.type"
            :options="providerTypeOptions"
            placeholder="Select type"
            :disabled="modalType === 'edit'"
          />
        </n-form-item-grid-item>

        <n-form-item-grid-item :span="1" label="Display Name" path="display_name">
          <n-input v-model:value="formModel.display_name" placeholder="e.g., Company LDAP" />
        </n-form-item-grid-item>

        <n-form-item-grid-item :span="1" label="Priority" path="priority">
          <n-input-number v-model:value="formModel.priority" :min="0" :max="100" />
        </n-form-item-grid-item>

        <n-form-item-grid-item :span="2" label="Enabled" path="enabled">
          <n-switch v-model:value="formModel.enabled">
            <template #checked>Yes</template>
            <template #unchecked>No</template>
          </n-switch>
        </n-form-item-grid-item>
      </n-grid>

      <!-- LDAP Configuration -->
      <template v-if="isLdap">
        <n-divider title-placement="left">LDAP Configuration</n-divider>
        <n-grid :cols="2" :x-gap="18">
          <n-form-item-grid-item :span="2" label="LDAP URL" path="ldap_url">
            <n-input
              v-model:value="formModel.ldap_url"
              placeholder="ldaps://ldap.example.com:636"
            />
          </n-form-item-grid-item>

          <n-form-item-grid-item :span="2" label="Bind DN" path="ldap_bind_dn">
            <n-input
              v-model:value="formModel.ldap_bind_dn"
              placeholder="cn=admin,dc=example,dc=com"
            />
          </n-form-item-grid-item>

          <n-form-item-grid-item :span="2" label="Bind Password" path="ldap_bind_pass">
            <n-input
              v-model:value="formModel.ldap_bind_pass"
              type="password"
              show-password-on="click"
              :placeholder="modalType === 'edit' ? 'Leave empty to keep current' : 'Enter password'"
            />
          </n-form-item-grid-item>

          <n-form-item-grid-item :span="2" label="Base DN" path="ldap_base_dn">
            <n-input
              v-model:value="formModel.ldap_base_dn"
              placeholder="ou=users,dc=example,dc=com"
            />
          </n-form-item-grid-item>

          <n-form-item-grid-item :span="2" label="Search Filter" path="ldap_filter">
            <n-input
              v-model:value="formModel.ldap_filter"
              placeholder="(uid=%s)"
            />
          </n-form-item-grid-item>

          <n-form-item-grid-item :span="2" label="TLS CA Cert">
            <n-input
              v-model:value="formModel.ldap_tls_cert"
              type="textarea"
              :autosize="{ minRows: 2, maxRows: 5 }"
              placeholder="Optional: PEM-encoded CA certificate"
            />
          </n-form-item-grid-item>
        </n-grid>
      </template>

      <!-- OIDC Configuration -->
      <template v-if="isOidc">
        <n-divider title-placement="left">OIDC Configuration</n-divider>
        <n-grid :cols="2" :x-gap="18">
          <n-form-item-grid-item :span="2" label="Issuer URL" path="oidc_issuer_url">
            <n-input
              v-model:value="formModel.oidc_issuer_url"
              placeholder="https://auth.example.com"
            />
          </n-form-item-grid-item>

          <n-form-item-grid-item :span="2" label="Client ID" path="oidc_client_id">
            <n-input
              v-model:value="formModel.oidc_client_id"
              placeholder="your-client-id"
            />
          </n-form-item-grid-item>

          <n-form-item-grid-item :span="2" label="Client Secret" path="oidc_client_secret">
            <n-input
              v-model:value="formModel.oidc_client_secret"
              type="password"
              show-password-on="click"
              :placeholder="modalType === 'edit' ? 'Leave empty to keep current' : 'Enter secret'"
            />
          </n-form-item-grid-item>

          <n-form-item-grid-item :span="2" label="Redirect URL" path="oidc_redirect_url">
            <n-input
              v-model:value="formModel.oidc_redirect_url"
              placeholder="https://your-app.com/auth/callback"
            />
          </n-form-item-grid-item>
        </n-grid>
      </template>
    </n-form>

    <template #action>
      <n-space justify="center">
        <n-button @click="closeModal">Cancel</n-button>
        <n-button type="primary" :loading="submitLoading" @click="submitModal">
          Submit
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>
