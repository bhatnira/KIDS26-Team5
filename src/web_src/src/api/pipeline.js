import { request } from '@/service/http'

// add pipeline
export function fetchAddPipeline(data) {
  return request.Post('/pipeline/add', data)
}

// delete pipeline
export function  fetchDeletePipeline(name, version) {
  return request.Delete('/pipeline/delete', {
    name,
    version
  })
}

// get pipeline
export function fetchPipelineList(page, pageSize) {
  return request.Get('/pipeline/list', {
    params: {
      page,
      page_size: pageSize
    }
  })
}

// get pipeline list
export function fetchPipelineListAll() {
  return request.Get('/pipeline/list_all')
}

// update pipeline
export function fetchUpdatePipeline(data) {
  return request.Post('/pipeline/update', data)
}

// fetch nextflow_schema.json for a pipeline (proxied + cached by the backend)
export function fetchPipelineSchema(repository, version) {
  return request.Get('/pipeline/schema', {
    params: {
      repository,
      version
    }
  })
}
