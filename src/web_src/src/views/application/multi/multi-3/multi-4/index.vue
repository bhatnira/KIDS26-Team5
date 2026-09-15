<script setup>
import { ref, reactive } from 'vue'
import {
  NSpace,
  NCard,
  NButton,
  NForm,
  NFormItem,
  NInput,
  NInputGroup,
  NTag,
  NText,
  useMessage
} from 'naive-ui'
import S3FileBrowserDialog from '@/components/custom/S3FileBrowserDialog.vue'

const message = useMessage()

// Form data
const formData = reactive({
  inputFile: '',
  outputPath: '',
  description: ''
})

// Dialog state
const fileBrowserVisible = ref(false)
const browserMode = ref('input') // 'input' or 'output'
const browserTitle = ref('Select File')

// Methods
const showFileBrowser = () => {
  browserMode.value = 'input'
  browserTitle.value = 'Select Input File'
  fileBrowserVisible.value = true
}

const showOutputBrowser = () => {
  browserMode.value = 'output'
  browserTitle.value = 'Select Output Location'
  fileBrowserVisible.value = true
}

const handleFileSelect = (path) => {
  if (browserMode.value === 'input') {
    formData.inputFile = path
    message.success('Input file selected')
  } else {
    formData.outputPath = path
    message.success('Output path selected')
  }
}

const handleSubmit = () => {
  if (!formData.inputFile) {
    message.warning('Please select an input file')
    return
  }

  if (!formData.outputPath) {
    message.warning('Please select an output path')
    return
  }

  // Submit logic here
  console.log('Form submitted:', formData)
  message.success('Form submitted successfully')
}

const handleReset = () => {
  formData.inputFile = ''
  formData.outputPath = ''
  formData.description = ''
  message.info('Form reset')
}
</script>

<template>
  <NSpace vertical size="large">
    <n-card title="File Selection Example">
      <n-space vertical size="large">
        <n-form ref="formRef" :model="formData" label-placement="left" label-width="120">
          <n-form-item label="Input File" path="inputFile">
            <n-input-group>
              <n-input
                v-model:value="formData.inputFile"
                placeholder="Select a file from S3"
                readonly
              />
              <n-button type="primary" @click="showFileBrowser">
                <template #icon>
                  <icon-park-outline-folder-open />
                </template>
                Browse
              </n-button>
            </n-input-group>
          </n-form-item>

          <n-form-item label="Output Path" path="outputPath">
            <n-input-group>
              <n-input
                v-model:value="formData.outputPath"
                placeholder="Select output location"
                readonly
              />
              <n-button type="primary" @click="showOutputBrowser">
                <template #icon>
                  <icon-park-outline-folder-open />
                </template>
                Browse
              </n-button>
            </n-input-group>
          </n-form-item>

          <n-form-item label="Description" path="description">
            <n-input
              v-model:value="formData.description"
              type="textarea"
              placeholder="Enter description"
              :rows="3"
            />
          </n-form-item>

          <n-form-item>
            <n-space>
              <n-button type="primary" @click="handleSubmit">
                <template #icon>
                  <icon-park-outline-check-one />
                </template>
                Submit
              </n-button>
              <n-button @click="handleReset">
                <template #icon>
                  <icon-park-outline-refresh />
                </template>
                Reset
              </n-button>
            </n-space>
          </n-form-item>
        </n-form>

        <!-- Selected Files Display -->
        <n-card v-if="formData.inputFile || formData.outputPath" title="Selected Paths">
          <n-space vertical>
            <div v-if="formData.inputFile">
              <n-text strong>Input File:</n-text>
              <n-tag type="info" size="small" style="margin-left: 8px">
                {{ formData.inputFile }}
              </n-tag>
            </div>
            <div v-if="formData.outputPath">
              <n-text strong>Output Path:</n-text>
              <n-tag type="success" size="small" style="margin-left: 8px">
                {{ formData.outputPath }}
              </n-tag>
            </div>
          </n-space>
        </n-card>
      </n-space>
    </n-card>

    <!-- File Browser Dialog -->
    <S3FileBrowserDialog
      v-model:visible="fileBrowserVisible"
      :title="browserTitle"
      :show-upload="false"
      @select="handleFileSelect"
    />
  </NSpace>
</template>
