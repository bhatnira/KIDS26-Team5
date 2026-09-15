import { request } from '@/service/http'

/**
 * List user's LLM configurations
 */
export function fetchLLMConfigs() {
  return request.Get('/llm/configs')
}

/**
 * Add a new LLM configuration
 */
export function addLLMConfig(config) {
  return request.Post('/llm/configs', config)
}

/**
 * Update an existing LLM configuration
 */
export function updateLLMConfig(id, config) {
  return request.Put(`/llm/configs/${id}`, config)
}

/**
 * Delete an LLM configuration
 */
export function deleteLLMConfig(id) {
  return request.Delete(`/llm/configs/${id}`)
}

/**
 * Test an LLM connection without saving
 */
export function testLLMConnection(config) {
  return request.Post('/llm/test-connection', config)
}
