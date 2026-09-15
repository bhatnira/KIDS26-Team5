<script setup>
import { useBoolean } from '@/hooks'
import { fetchAddUser, fetchUpdateUser } from '@/api/admin'
import { ref, shallowRef, computed } from 'vue'

const props = defineProps({
  modalName: {
    type: String,
    default: '',
  },
})

const emit = defineEmits(['open', 'close'])

const { bool: modalVisible, setTrue: showModal, setFalse: hiddenModal } = useBoolean(false)
const { bool: submitLoading, setTrue: startLoading, setFalse: endLoading } = useBoolean(false)

const formDefault = {
  userName: '',
  email: '',
  department: '',
  group: '',
  role: 'user',
  status: 1,
  password: '',
  rePassword: '',
}
const formModel = ref({ ...formDefault })

const modalType = shallowRef('add')
const modalTitle = computed(() => {
  const titleMap = {
    add: 'Add',
    view: 'View',
    edit: 'Edit',
  }
  return `${titleMap[modalType.value]} ${props.modalName}`
})

async function openModal(type = 'add', data = null) {
  emit('open')
  modalType.value = type
  showModal()

  const handlers = {
    async add() {
      formModel.value = { ...formDefault }
    },
    async view() {
      if (!data) return
      formModel.value = { ...data }
    },
    async edit() {
      if (!data) return
      formModel.value = { ...data }
    },
  }
  await handlers[type]()
}

function closeModal() {
  hiddenModal()
  endLoading()
  emit('close')
}

defineExpose({
  openModal,
})

const formRef = ref()

async function submitModal() {
  try {
    await formRef.value?.validate()
    startLoading()

    const handlers = {
      async add() {
        try {
          await fetchAddUser({
            name: formModel.value.userName,
            email: formModel.value.email,
            department: formModel.value.department,
            group: formModel.value.group,
            role: formModel.value.role,
            status: formModel.value.status,
            password: formModel.value.password,
          })
          window.$message.success('User added successfully')
          return true
        } catch (error) {
          window.$message.error('Add failed, please retry')
          return false
        }
      },
      async edit() {
        try {
          await fetchUpdateUser({
            name: formModel.value.userName,
            email: formModel.value.email,
            department: formModel.value.department,
            group: formModel.value.group,
            role: formModel.value.role,
            status: formModel.value.status,
          })
          window.$message.success('User updated successfully')
          return true
        } catch (error) {
          window.$message.error('Update failed, please retry')
          return false
        }
      },
      async view() {
        return true
      },
    }

    const success = await handlers[modalType.value]()
    if (success) {
      closeModal()
    } else {
      endLoading()
    }
  } catch (error) {
    endLoading()
  }
}

// Form validation rules
const rules = {
  userName: {
    required: true,
    message: 'Please enter username',
    trigger: 'blur',
  },
  email: [
    {
      required: true,
      message: 'Please enter email',
      trigger: 'blur',
    },
    {
      type: 'email',
      message: 'Please enter a valid email address',
      trigger: ['blur', 'change'],
    },
  ],
  department: {
    required: true,
    message: 'Please enter department',
    trigger: 'blur',
  },
  role: {
    required: true,
    message: 'Please select role',
    trigger: 'change',
  },
  password: {
    required: true,
    message: 'Please enter password',
    trigger: 'blur',
  },
  rePassword: [
    {
      required: true,
      message: 'Please enter password again',
      trigger: 'blur',
    },
    {
      validator: (rule, value) => {
        if (value !== formModel.value.password) {
          return new Error('Passwords do not match')
        }
        return true
      },
      trigger: ['blur', 'change'],
    },
  ],
}

// Role options
const roleOptions = [
  { label: 'Admin', value: 'admin' },
  { label: 'Super', value: 'super' },
  { label: 'User', value: 'user' },
]
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
      :label-width="100"
      :disabled="modalType === 'view'"
    >
      <n-grid :cols="2" :x-gap="18">
        <n-form-item-grid-item :span="1" label="Username" path="userName">
          <n-input v-model:value="formModel.userName" placeholder="Enter username" />
        </n-form-item-grid-item>

        <n-form-item-grid-item :span="1" label="Email" path="email">
          <n-input
            v-model:value="formModel.email"
            placeholder="Enter email"
            :disabled="modalType === 'edit'"
          />
        </n-form-item-grid-item>

        <n-form-item-grid-item :span="1" label="Department" path="department">
          <n-input v-model:value="formModel.department" placeholder="Enter department" />
        </n-form-item-grid-item>

        <n-form-item-grid-item :span="1" label="Group" path="group">
          <n-input v-model:value="formModel.group" placeholder="Enter group" />
        </n-form-item-grid-item>

        <n-form-item-grid-item :span="2" label="Role" path="role">
          <n-select
            v-model:value="formModel.role"
            :options="roleOptions"
            placeholder="Select role"
          />
        </n-form-item-grid-item>

        <n-form-item-grid-item v-if="modalType === 'add'" :span="1" label="Password" path="password">
          <n-input
            v-model:value="formModel.password"
            type="password"
            show-password-on="click"
            placeholder="Enter password"
          />
        </n-form-item-grid-item>

        <n-form-item-grid-item
          v-if="modalType === 'add'"
          :span="1"
          label="Confirm Password"
          path="rePassword"
        >
          <n-input
            v-model:value="formModel.rePassword"
            type="password"
            show-password-on="click"
            placeholder="Enter password again"
          />
        </n-form-item-grid-item>

        <n-form-item-grid-item :span="1" label="Status" path="status">
          <n-switch v-model:value="formModel.status" :checked-value="1" :unchecked-value="0">
            <template #checked> Active </template>
            <template #unchecked> Disabled </template>
          </n-switch>
        </n-form-item-grid-item>
      </n-grid>
    </n-form>

    <template #action>
      <n-space justify="center">
        <n-button @click="closeModal">Cancel</n-button>
        <n-button
          v-if="modalType !== 'view'"
          type="primary"
          :loading="submitLoading"
          @click="submitModal"
        >
          Submit
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>
