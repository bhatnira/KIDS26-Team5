import { request } from '@/service/http'

/** Get the current user's own profile */
export function fetchProfile() {
  return request.Get('/user/profile')
}

/** Update the current user's name and department */
export function fetchUpdateProfile(data) {
  return request.Put('/user/profile', data)
}

/** Change the current user's local password */
export function fetchChangePassword(data) {
  return request.Post('/user/password', data)
}
