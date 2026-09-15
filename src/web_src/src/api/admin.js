import { request } from '@/service/http'

// User management (super only)
export function fetchUserList() {
  return request.Get('/system/user/list')
}

export function fetchUpdateUser(data) {
  return request.Post('/system/user/update', data)
}

export function fetchDeleteUser(id) {
  return request.Delete(`/system/user/delete/${id}`)
}

export function fetchAddUser(data) {
  return request.Post('/system/user/add', data)
}

// Auth provider management (super only)
export function fetchAuthProviderList() {
  return request.Get('/system/auth/providers')
}

export function fetchAddAuthProvider(data) {
  return request.Post('/system/auth/provider/add', data)
}

export function fetchUpdateAuthProvider(data) {
  return request.Post('/system/auth/provider/update', data)
}

export function fetchDeleteAuthProvider(id) {
  return request.Delete(`/system/auth/provider/delete/${id}`)
}

export function fetchTestAuthProvider(id) {
  return request.Post(`/system/auth/provider/test/${id}`)
}
