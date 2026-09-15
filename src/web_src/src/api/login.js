import { request } from '@/service/http'

export const fetchRegister = (data) => {
  return request.Post('/user/register', data)
}

export const fetchCode = (data) => {
  return request.Post('/user/register/code', data)
}

export const fetchResetPassword = (data) => {
  return request.Post('/user/reset', data)
}

export const fetchResetCode = (data) => {
  return request.Post('/user/reset/code', data)
}

export function fetchLogin(data) {
  const methodInstance = request.Post('/auth/login', data)
  methodInstance.meta = {
    authRole: null,
  }
  return methodInstance
}

export function fetchUpdateToken(data) {
  const method = request.Post('/auth/refresh', data)
  method.meta = {
    authRole: 'refreshToken',
  }
  return method
}

export function fetchUserJobList(page, pageSize) {
  return request.Get('/user/jobs', {
    params: {
      page,
      page_size: pageSize
    }
  })
}

/**
 * Get enabled auth providers for login page
 */
export function fetchAuthProviders() {
  const method = request.Get('/auth/providers')
  method.meta = {
    authRole: null,
  }
  return method
}

/**
 * LDAP login
 * @param {Object} data
 * @param {string} data.username
 * @param {string} data.password
 * @param {string} data.provider
 */
export function fetchLdapLogin(data) {
  const method = request.Post('/auth/ldap/login', data)
  method.meta = {
    authRole: null,
  }
  return method
}

/**
 * Start an OIDC login: returns the provider authorization URL carrying a
 * one-time state nonce. Call this when the user initiates OIDC login.
 * @param {string} provider Provider name
 */
export function fetchOidcStart(provider) {
  const method = request.Get(`/auth/oidc/${encodeURIComponent(provider)}/start`)
  method.meta = {
    authRole: null,
  }
  return method
}

/**
 * OIDC callback. The provider is resolved server-side from the state nonce, so
 * only the code and state returned by the IdP are sent.
 * @param {Object} data
 * @param {string} data.code
 * @param {string} data.state
 */
export function fetchOidcCallback(data) {
  const method = request.Post('/auth/oidc/callback', data)
  method.meta = {
    authRole: null,
  }
  return method
}
