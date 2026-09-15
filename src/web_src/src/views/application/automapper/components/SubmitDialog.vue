<script setup>
import { ref, watch } from 'vue'
import { fetchSubmitJob } from '@/api/automapper'

const emit = defineEmits(['submit-loading'])

const credentialForm = ref({
  Credentials: {
    uid: '',
    pwd: ''
  }
})

const rules = {
  Credentials: {
    uid: { required: true, message: 'Please input User ID', trigger: 'blur' },
    pwd: { required: true, message: 'Please input password', trigger: 'blur' }
  }
}

const credentialRef = ref(null)
const dialogVisible = ref(false)
const submitting = ref(false)
let submitForm = {}

const submit = (AtgcpForm) => {
  dialogVisible.value = true
  submitForm = { ...AtgcpForm }
}

watch(submitting, v => emit('submit-loading', v))

const submitJob = async () => {
  try {
    await credentialRef.value?.validate()

    submitting.value = true

    // Update submit form with credentials
    const finalData = {
      ...submitForm,
      Credentials: { ...credentialForm.value.Credentials }
    }

    const res = await fetchSubmitJob(finalData)
    console.log(res)

    if (res.isSuccess) {
      window.$message.success('Job created successfully')
      dialogVisible.value = false
    } else {
      // Error is already handled by alova interceptor (showError)
      console.error('Submit job failed:', res.message)
    }
  } catch (error) {
    if (error?.errors) {
      window.$message.error('Please input User ID and password')
      return
    }
    console.error('Unexpected error:', error)
    window.$message.error('An unexpected error occurred')
  } finally {
    submitting.value = false
  }
}

defineExpose({
  submit
})
</script>

<template>
  <n-modal
    v-model:show="dialogVisible"
    preset="card"
    title="Submit to HPC"
    class="w-450px"
    :segmented="{
      content: true,
      action: true,
    }"
  >
    <n-form
      ref="credentialRef"
      :model="credentialForm"
      :rules="rules"
      label-placement="left"
      label-width="100"
      require-mark-placement="right-hanging"
    >
      <n-form-item label="User ID" path="Credentials.uid">
        <n-input
          v-model:value="credentialForm.Credentials.uid"
          placeholder="Please input St. Jude UID"
        >
          <template #prefix>
            <icon-park-outline-user />
          </template>
        </n-input>
      </n-form-item>
      <n-form-item label="Password" path="Credentials.pwd">
        <n-input
          v-model:value="credentialForm.Credentials.pwd"
          type="password"
          show-password-on="mousedown"
          placeholder="Please input Password"
        >
          <template #prefix>
            <icon-park-outline-lock />
          </template>
        </n-input>
      </n-form-item>
    </n-form>
    <template #action>
      <n-space justify="end">
        <n-button @click="dialogVisible = false">Cancel</n-button>
        <n-button
          type="primary"
          :loading="submitting"
          :disabled="submitting"
          @click="submitJob"
        >
          Confirm
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

