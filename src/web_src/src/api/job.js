import { request } from '@/service/http'
import { local } from '@/utils/storage'

// delete job by id
export function fetchDeleteJob(jobId) {
  return request.Delete('/job/delete', {
    job_id: jobId,
  })
}

export function fetchDispatchJob(data) {
  return request.Post('/job/add', data)
}

// stop a dispatched job on nomad
export function fetchStopJob(jobId) {
  return request.Post('/job/stop', {
    job_id: jobId,
  })
}

// get full job details including input parameters
export function fetchJobDetails(jobId) {
  return request.Get('/job/details', {
    params: {
      job_id: jobId,
    },
  })
}

export function fetchStreamJobLog(allocId, logType) {
  return request.Get('/user/job/log', {
    headers: {
      Authorization: `Bearer ${local.get('accessToken')}`,
    },
    params: {
      allocId: allocId,
      logType: logType,
    }
  })
}
