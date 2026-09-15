<script setup>
import { ref, computed } from 'vue'
import { SET_ASSAY_TYPE, OBJ_ORGANISMS_TYPE, OBJ_REFERENCE_TYPE } from '@/constants/Automapper'
import { fetchFastqQuery } from '@/api/automapper'
import SubmitDialog from './components/SubmitDialog.vue'

const formData = ref({
  ASSAY_TYPE: null,
  DESTINATION: '',
  FQ_LIST: [],
  FQ_SOURCE: 'SJ_GSF',
  LIBRARY_LAYOUT: null,
  NUMBER_OF_SAMPLES: 0,
  ORGANISM: null,
  PIPELINE: null,
  PI_GROUP: '',
  PLATFORM: 'Illumina',
  REFERENCE: null,
  XENOGRAFT: false,
  SRM_ORDER_NUMBER: '',
  SEQUENCING_TYPE: null,
  USERINFO: {
    EMAIL: '',
    NAME: ''
  }
})

const formRef = ref(null)
const dialogRef = ref(null)
const submitToHpcLoading = ref(false)
const jsonModalVisible = ref(false)
const queryLoading = ref(false)
const fqString = ref('')

const organisms = ref([])
const references = ref([])
const seqtypes = ref([])

const isOrgDisabled = computed(() => !formData.value.ASSAY_TYPE)
const isRefDisabled = computed(() => !formData.value.ORGANISM)
const isSeqTypeDisabled = computed(() => !formData.value.ASSAY_TYPE)

const libraryLayouts = [
  { label: 'PE', value: 'PE' },
  { label: 'SE', value: 'SE' }
]
const fqSources = [
  { label: 'SJ_GSF', value: 'SJ_GSF' },
  { label: 'User_Defined', value: 'User_Defined' }
]
const pipelines = [
  { label: 'FQMapping', value: 'FQMapping' },
  { label: 'VariantCalling', value: 'VariantCalling' }
]

const rules = {
  PI_GROUP: { required: true, message: 'Please input PI GROUP', trigger: 'blur' },
  SRM_ORDER_NUMBER: { required: true, message: 'Please input SRM order number', trigger: 'blur' },
  ASSAY_TYPE: { required: true, message: 'Please select assay type', trigger: ['blur', 'change'] },
  SEQUENCING_TYPE: { required: true, message: 'Please select sequencing type', trigger: ['blur', 'change'] },
  ORGANISM: { required: true, message: 'Please select organism', trigger: ['blur', 'change'] },
  REFERENCE: { required: true, message: 'Please select reference genome', trigger: ['blur', 'change'] },
  LIBRARY_LAYOUT: { required: true, message: 'Please select library layout', trigger: ['blur', 'change'] },
  PIPELINE: { required: true, message: 'Please select pipeline', trigger: ['blur', 'change'] },
  FQ_SOURCE: { required: true, message: 'Please select FASTQ source', trigger: ['blur', 'change'] },
  DESTINATION: { required: true, message: 'Please input destination', trigger: 'blur' },
  'USERINFO.EMAIL': { required: true, message: 'Please enter user email', trigger: 'blur' },
  'USERINFO.NAME': { required: true, message: 'Please enter username', trigger: 'blur' }
}

function getOrganismOptions() {
  const assay = formData.value.ASSAY_TYPE
  if (assay) {
    if (['ChIPseq', 'CutRun'].includes(assay)) {
      organisms.value = [
        ...OBJ_ORGANISMS_TYPE.PRIMARY,
        ...OBJ_ORGANISMS_TYPE.SECONDARY,
        ...OBJ_ORGANISMS_TYPE.HYBRID,
        ...OBJ_ORGANISMS_TYPE.RDNA
      ].map(opt => ({ label: opt, value: opt }))
    } else if (['ATACseq', 'CutTag'].includes(assay)) {
      organisms.value = [
        ...OBJ_ORGANISMS_TYPE.PRIMARY,
        ...OBJ_ORGANISMS_TYPE.SECONDARY,
        ...OBJ_ORGANISMS_TYPE.RDNA
      ].map(opt => ({ label: opt, value: opt }))
    } else {
      organisms.value = OBJ_ORGANISMS_TYPE.PRIMARY.map(opt => ({ label: opt, value: opt }))
    }

    seqtypes.value = (assay === 'RNAseq' ? ['Stranded', 'Unstranded'] : [assay]).map(opt => ({ label: opt, value: opt }))

    // Reset organism and reference when assay changes
    formData.value.ORGANISM = null
    formData.value.REFERENCE = null
    formData.value.SEQUENCING_TYPE = seqtypes.value.length === 1 ? seqtypes.value[0].value : null
  } else {
    organisms.value = []
    seqtypes.value = []
  }
}

function getReferenceOptions() {
  if (formData.value.ORGANISM) {
    references.value = (OBJ_REFERENCE_TYPE[formData.value.ORGANISM] || []).map(opt => ({ label: opt, value: opt }))
    formData.value.REFERENCE = null
  } else {
    references.value = []
  }
}

function handleFqSourceChange() {
  fqString.value = ''
  parseFqList()
}

async function queryGSF() {
  const gsfFields = ['PI_GROUP', 'SRM_ORDER_NUMBER', 'ASSAY_TYPE', 'SEQUENCING_TYPE', 'ORGANISM', 'REFERENCE']
  const missingFields = gsfFields.filter(field => !formData.value[field])

  if (missingFields.length > 0) {
    window.$message.error('Please fill in all required GSF fields: ' + missingFields.join(', '))
    return
  }

  try {
    queryLoading.value = true
    const params = {
      pigrp: formData.value.PI_GROUP,
      soid: formData.value.SRM_ORDER_NUMBER,
      organism: formData.value.ORGANISM,
      reference: formData.value.REFERENCE,
      assay: formData.value.ASSAY_TYPE,
      seqtype: formData.value.SEQUENCING_TYPE
    }

    const res = await fetchFastqQuery(params)
    console.log(res)

    if (res.isSuccess) {
      const result = res.data?.RESULT || res.RESULT
      if (result && result.FQ_LIST && Array.isArray(result.FQ_LIST)) {
        formData.value = { ...formData.value, ...result }
        fqString.value = result.FQ_LIST.join('\n')
        window.$message.success(`Successfully retrieved ${result.FQ_LIST.length} sequence files`)
      } else {
        window.$message.warning('No FASTQ files found or invalid response format')
      }
    }
  } catch (error) {
    console.error('GSF query error:', error)
  } finally {
    queryLoading.value = false
  }
}

function parseFqList() {
  if (!fqString.value) {
    formData.value.FQ_LIST = []
    formData.value.NUMBER_OF_SAMPLES = 0
  } else {
    formData.value.FQ_LIST = fqString.value.split('\n').filter(line => line.trim() !== '')
    formData.value.NUMBER_OF_SAMPLES = formData.value.FQ_LIST.length
  }
}

function handleFileUpload({ file }) {
  if (file.file.size > 10 * 1024 * 1024) {
    window.$message.error('File size should not exceed 10MB')
    return false
  }

  const reader = new FileReader()
  reader.onload = (e) => {
    fqString.value = e.target.result
    parseFqList()
    window.$message.success('File uploaded successfully.')
  }
  reader.onerror = () => {
    window.$message.error('File upload failed.')
  }
  reader.readAsText(file.file)
  return false
}

async function onSubmitToHPC() {
  try {
    await formRef.value?.validate()
    dialogRef.value?.submit(formData.value)
  } catch (error) {
    window.$message.error('Please fill the form before submitting.')
  }
}

function onSubmitLoading(value) {
  submitToHpcLoading.value = value
}

const assayTypeOptions = SET_ASSAY_TYPE.map(assay => ({ label: assay, value: assay }))
</script>

<template>
  <n-space vertical size="large">
    <n-card :loading="queryLoading">
      <n-form
        ref="formRef"
        :model="formData"
        :rules="rules"
        label-placement="left"
        label-width="140"
        require-mark-placement="right-hanging"
      >
        <n-space vertical size="medium">
          <!-- Basic Information -->
          <n-divider title-placement="left" style="margin: 12px 0">
            <n-space align="center" :size="8">
              <icon-park-outline-info class="text-base" />
              <span class="text-sm font-medium">Basic Information</span>
            </n-space>
          </n-divider>
          <n-grid :cols="24" :x-gap="24" :y-gap="12">
            <n-form-item-grid-item :span="12" label="PI Group ID" path="PI_GROUP">
              <n-input v-model:value="formData.PI_GROUP" placeholder="e.g., cabgrp" />
            </n-form-item-grid-item>
            <n-form-item-grid-item :span="12" label="SRM Order ID" path="SRM_ORDER_NUMBER">
              <n-input v-model:value="formData.SRM_ORDER_NUMBER" placeholder="e.g., 843877" />
            </n-form-item-grid-item>
          </n-grid>

          <!-- Sample Configuration -->
          <n-divider title-placement="left" style="margin: 12px 0">
            <n-space align="center" :size="8">
              <icon-park-outline-table-file class="text-base" />
              <span class="text-sm font-medium">Sample Configuration</span>
            </n-space>
          </n-divider>
          <n-grid :cols="24" :x-gap="24" :y-gap="12">
            <n-form-item-grid-item :span="12" label="Assay Type" path="ASSAY_TYPE">
              <n-select
                v-model:value="formData.ASSAY_TYPE"
                :options="assayTypeOptions"
                placeholder="Select Assay Type"
                @update:value="getOrganismOptions"
              />
            </n-form-item-grid-item>
            <n-form-item-grid-item :span="12" label="Sequencing Type" path="SEQUENCING_TYPE">
              <n-select
                v-model:value="formData.SEQUENCING_TYPE"
                :options="seqtypes"
                :disabled="isSeqTypeDisabled"
                placeholder="Select Sequencing Type"
              />
            </n-form-item-grid-item>

            <n-form-item-grid-item :span="12" label="Organism" path="ORGANISM">
              <n-select
                v-model:value="formData.ORGANISM"
                :options="organisms"
                :disabled="isOrgDisabled"
                placeholder="Select Organism"
                @update:value="getReferenceOptions"
              />
            </n-form-item-grid-item>
            <n-form-item-grid-item :span="12" label="Reference" path="REFERENCE">
              <n-select
                v-model:value="formData.REFERENCE"
                :options="references"
                :disabled="isRefDisabled"
                placeholder="Select Reference"
              />
            </n-form-item-grid-item>
          </n-grid>

          <!-- Pipeline Configuration -->
          <n-divider title-placement="left" style="margin: 12px 0">
            <n-space align="center" :size="8">
              <icon-park-outline-setting-two class="text-base" />
              <span class="text-sm font-medium">Pipeline Configuration</span>
            </n-space>
          </n-divider>
          <n-grid :cols="24" :x-gap="24" :y-gap="12">
            <n-form-item-grid-item :span="8" label="Library Layout" path="LIBRARY_LAYOUT">
              <n-select v-model:value="formData.LIBRARY_LAYOUT" :options="libraryLayouts" />
            </n-form-item-grid-item>
            <n-form-item-grid-item :span="8" label="Pipeline" path="PIPELINE">
              <n-select v-model:value="formData.PIPELINE" :options="pipelines" />
            </n-form-item-grid-item>
            <n-form-item-grid-item :span="8" label="XenoGraft" path="XENOGRAFT">
              <n-switch v-model:value="formData.XENOGRAFT" />
            </n-form-item-grid-item>
          </n-grid>

          <!-- FASTQ Configuration -->
          <n-divider title-placement="left" style="margin: 12px 0">
            <n-space align="center" :size="8">
              <icon-park-outline-file-code class="text-base" />
              <span class="text-sm font-medium">FASTQ Configuration</span>
            </n-space>
          </n-divider>
          <n-grid :cols="24" :x-gap="24" :y-gap="12">
            <n-form-item-grid-item :span="16" label="FASTQ Source" path="FQ_SOURCE">
              <n-select
                v-model:value="formData.FQ_SOURCE"
                :options="fqSources"
                @update:value="handleFqSourceChange"
              />
            </n-form-item-grid-item>
            <n-form-item-grid-item :span="8">
              <n-button
                v-if="formData.FQ_SOURCE === 'SJ_GSF'"
                type="primary"
                block
                :loading="queryLoading"
                :disabled="queryLoading"
                @click="queryGSF"
              >
                Query FASTQ
              </n-button>
              <n-upload
                v-else
                :show-file-list="false"
                @change="handleFileUpload"
              >
                <n-button type="primary" block>Select File</n-button>
              </n-upload>
            </n-form-item-grid-item>

            <n-form-item-grid-item :span="24" label="FASTQ Files">
              <n-input
                v-model:value="fqString"
                type="textarea"
                placeholder="FASTQ files displayed here"
                :autosize="{ minRows: 4, maxRows: 8 }"
                @update:value="parseFqList"
              />
            </n-form-item-grid-item>
          </n-grid>

          <!-- Output & User Information -->
          <n-divider title-placement="left" style="margin: 12px 0">
            <n-space align="center" :size="8">
              <icon-park-outline-user class="text-base" />
              <span class="text-sm font-medium">Output & User Information</span>
            </n-space>
          </n-divider>
          <n-grid :cols="24" :x-gap="24" :y-gap="12">
            <n-form-item-grid-item :span="24" label="Destination" path="DESTINATION">
              <n-input v-model:value="formData.DESTINATION" placeholder="Output destination path" />
            </n-form-item-grid-item>

            <n-form-item-grid-item :span="12" label="User Email" path="USERINFO.EMAIL">
              <n-input v-model:value="formData.USERINFO.EMAIL" placeholder="Email address" />
            </n-form-item-grid-item>
            <n-form-item-grid-item :span="12" label="User Name" path="USERINFO.NAME">
              <n-input v-model:value="formData.USERINFO.NAME" placeholder="Full name" />
            </n-form-item-grid-item>
          </n-grid>

          <!-- Action Buttons -->
          <n-divider style="margin: 16px 0 8px 0" />
          <n-space justify="end" :size="12">
            <n-button strong secondary type="info" @click="jsonModalVisible = true">
              <template #icon>
                <icon-park-outline-code />
              </template>
              View JSON
            </n-button>
            <n-button
              type="primary"
              :loading="submitToHpcLoading"
              :disabled="submitToHpcLoading"
              @click="onSubmitToHPC"
            >
              <template #icon>
                <icon-park-outline-send />
              </template>
              Submit to HPC
            </n-button>
          </n-space>
        </n-space>
      </n-form>
    </n-card>

    <n-modal
      v-model:show="jsonModalVisible"
      preset="card"
      title="Current Form Data (JSON)"
      class="w-600px"
    >
      <div class="bg-gray-100 p-4 rounded font-mono text-sm overflow-auto max-h-500px">
        <pre>{{ JSON.stringify(formData, null, 2) }}</pre>
      </div>
      <template #footer>
        <n-space justify="end">
          <n-button @click="jsonModalVisible = false">Close</n-button>
        </n-space>
      </template>
    </n-modal>

    <SubmitDialog ref="dialogRef" @submit-loading="onSubmitLoading" />
  </n-space>
</template>

