import { request } from '@/service/http'

/**
 * Get all available job templates
 */
export function fetchJobTemplates() {
  return request.Get('/templates')
}

/**
 * Get a single job template by ID
 */
export function fetchJobTemplate(id) {
  return request.Get(`/templates/${id}`)
}

/**
 * Create a new job template (admin/super only)
 */
export function fetchCreateJobTemplate(data) {
  return request.Post('/templates', data)
}

/**
 * Update a job template (admin/super only)
 */
export function fetchUpdateJobTemplate(id, data) {
  return request.Put(`/templates/${id}`, data)
}

/**
 * Delete a job template (admin/super only, not built-in)
 */
export function fetchDeleteJobTemplate(id) {
  return request.Delete(`/templates/${id}`)
}
