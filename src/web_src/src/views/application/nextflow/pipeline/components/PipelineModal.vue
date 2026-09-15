<script setup>
import { fetchAddPipeline, fetchUpdatePipeline } from '@/api/pipeline'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true,
  },
  type: {
    type: String,
    default: 'add',
  },
  modalData: {
    type: Object,
    default: null,
  },
})

const emit = defineEmits(['update:visible', 'success'])

const defaultFormModal = {
  name: '',
  version: '',
  repository: '',
  author: '',
  description: '',
}

const formModel = ref({ ...defaultFormModal })
const formRef = ref(null)
const submitting = ref(false)

// Form validation rules
const rules = {
  name: {
    required: true,
    message: 'Please input pipeline name',
    trigger: 'blur',
  },
  version: {
    required: true,
    message: 'Please input pipeline version',
    trigger: 'blur',
  },
  repository: {
    required: true,
    message: 'Please input repository URL',
    trigger: 'blur',
  },
  author: {
    required: true,
    message: 'Please input author',
    trigger: 'blur',
  },
  description: {
    required: true,
    message: 'Please input description',
    trigger: 'blur',
  },
}

const modalVisible = computed({
  get() {
    return props.visible
  },
  set(visible) {
    closeModal(visible)
  },
})

function closeModal(visible = false) {
  emit('update:visible', visible)
  if (!visible) {
    // Reset form after closing
    setTimeout(() => {
      formModel.value = { ...defaultFormModal }
      formRef.value?.restoreValidation()
    }, 200)
  }
}

const title = computed(() => {
  const titles = {
    add: 'New Pipeline',
    edit: 'Edit Pipeline',
  }
  return titles[props.type]
})

function updateFormModelByModalType() {
  const handlers = {
    add: () => {
      formModel.value = { ...defaultFormModal }
    },
    edit: () => {
      if (props.modalData) {
        formModel.value = {
          name: props.modalData.name || '',
          version: props.modalData.version || '',
          repository: props.modalData.repository || '',
          author: props.modalData.author || '',
          description: props.modalData.description || '',
        }
      }
    },
  }
  handlers[props.type]()
}

watch(
  () => props.visible,
  (newValue) => {
    if (newValue) {
      updateFormModelByModalType()
    }
  },
)

async function handleSubmit() {
  try {
    await formRef.value?.validate()
  } catch (error) {
    // Invalid form: fields surface their own inline errors, nothing to toast.
    return
  }

  submitting.value = true
  try {
    const data = {
      name: formModel.value.name,
      version: formModel.value.version,
      repository: formModel.value.repository,
      author: formModel.value.author,
      description: formModel.value.description,
    }

    const isAdd = props.type === 'add'
    // The alova interceptor resolves (never rejects) on API/business errors and
    // has already shown the matching error toast, so gate every success
    // side-effect on isSuccess instead of assuming resolution means success —
    // otherwise a failed add still pops a spurious "success" toast and closes
    // the modal.
    const res = isAdd ? await fetchAddPipeline(data) : await fetchUpdatePipeline(data)
    if (!res?.isSuccess) return

    window.$message.success(isAdd ? 'Add pipeline success' : 'Update pipeline success')
    emit('success')
    closeModal()
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <n-modal
    v-model:show="modalVisible"
    :mask-closable="false"
    preset="card"
    :title="title"
    class="w-700px"
    :segmented="{
      content: true,
      action: true,
    }"
  >
    <n-form
      ref="formRef"
      label-placement="left"
      :model="formModel"
      :rules="rules"
      label-align="left"
      :label-width="100"
      require-mark-placement="right-hanging"
    >
      <n-grid :cols="24" :x-gap="18">
        <n-form-item-grid-item :span="12" label="Name" path="name">
          <n-input
            v-model:value="formModel.name"
            placeholder="Please input pipeline name"
            :disabled="type === 'edit'"
          />
        </n-form-item-grid-item>
        <n-form-item-grid-item :span="12" label="Version" path="version">
          <n-input
            v-model:value="formModel.version"
            placeholder="Please input version"
            :disabled="type === 'edit'"
          />
        </n-form-item-grid-item>
        <n-form-item-grid-item :span="24" label="Repo URL" path="repository">
          <n-input
            v-model:value="formModel.repository"
            placeholder="Please input repo URL，e.g.,: https://github.com/nf-core/rnaseq"
            :disabled="type === 'edit'"
          />
        </n-form-item-grid-item>
        <n-form-item-grid-item :span="12" label="Author" path="author">
          <n-input
            v-model:value="formModel.author"
            placeholder="Please input author"
          />
        </n-form-item-grid-item>
        <n-form-item-grid-item :span="24" label="Description" path="description">
          <n-input
            v-model:value="formModel.description"
            type="textarea"
            placeholder="Please input pipeline description"
            :autosize="{
              minRows: 3,
              maxRows: 5,
            }"
          />
        </n-form-item-grid-item>
      </n-grid>
    </n-form>
    <template #action>
      <n-space justify="center">
        <n-button @click="closeModal()" :disabled="submitting">
          Cancel
        </n-button>
        <n-button type="primary" @click="handleSubmit" :loading="submitting">
          {{ type === 'add' ? 'Create' : 'Save' }}
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<style scoped></style>
