import { request } from '@/service/http'

export function fetchApiKeys() {
  return request.Get('/user/api-keys')
}

export function createApiKey(data) {
  return request.Post('/user/api-keys', data)
}

export function revokeApiKey(id) {
  return request.Delete(`/user/api-keys/${id}`)
}
