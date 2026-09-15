import { local } from '@/utils/storage'
import { createAlova } from 'alova'
import adapterFetch from 'alova/fetch'
import { createServerTokenAuthentication } from 'alova/client'
import VueHook from 'alova/vue'
import { useAuthStore } from '@/stores'
import { fetchUpdateToken } from '@/api/login'
import { $t } from '@/utils/i18n'
import {
  DEFAULT_ALOVA_OPTIONS,
  DEFAULT_BACKEND_OPTIONS,
  ERROR_NO_TIP_STATUS,
  ERROR_STATUS,
} from './config'

/**
 * @description: 处理请求成功，但返回后端服务器报错
 * @param {Response} response
 * @return {*}
 */
async function handleResponseError(response) {
  const error = {
    errorType: 'Response Error',
    code: 0,
    message: ERROR_STATUS.default,
    data: null,
  }

  const errorCode = response.status
  const apiData = await response.json()
  const message = apiData.msg || ERROR_STATUS[errorCode] || ERROR_STATUS.default
  Object.assign(error, { code: errorCode, message })

  showError(error)

  return error
}

/**
 * @description:
 * @param  data 接口返回的后台数据
 * @param  config 后台字段配置
 * @return {*}
 */
export function handleBusinessError(data, config) {
  const { codeKey, msgKey } = config
  const error = {
    errorType: 'Business Error',
    code: data[codeKey],
    message: data[msgKey],
    data: data.data,
  }

  showError(error)

  return error
}

/**
 * @description: 统一成功和失败返回类型
 * @param {any} data
 * @param {boolean} isSuccess
 * @return {*} result
 */
function handleServiceResult(data, isSuccess = true) {
  return {
    isSuccess,
    errorType: null,
    ...data,
  }
}

// Guard so concurrent failed requests produce exactly one session-expired
// message + redirect instead of one per in-flight request.
let isHandlingExpiry = false

// While true, raw auth-error toasts are suppressed because we are in the middle
// of refreshing the token / handling a session expiry and will (or already did)
// show the single centralized message. This is scoped to the refresh flow only,
// so intentional auth errors (e.g. a wrong-password login) still show normally.
let suppressAuthToast = false

/**
 * @description: Centralized session-expiry handling. Idempotent: shows a single
 * "please log in again" message and redirects to login regardless of how many
 * requests failed simultaneously.
 */
async function handleSessionExpired() {
  if (isHandlingExpiry) return
  isHandlingExpiry = true
  suppressAuthToast = true

  window.$message?.warning($t('login.sessionExpired'))

  const authStore = useAuthStore()
  try {
    await authStore.logout()
  } finally {
    // allow a future session to expire and notify again
    setTimeout(() => {
      isHandlingExpiry = false
      suppressAuthToast = false
    }, 3000)
  }
}

/**
 * @description: 处理接口token刷新
 * @return {*}
 */
async function handleRefreshToken() {
  const { VITE_AUTO_REFRESH_TOKEN = 'N' } = import.meta.env
  const isAutoRefresh = VITE_AUTO_REFRESH_TOKEN === 'Y'
  if (!isAutoRefresh) {
    await handleSessionExpired()
    return
  }

  // 刷新token — suppress the refresh call's own auth-error toast; we surface a
  // single centralized message instead if it ultimately fails.
  suppressAuthToast = true
  try {
    const { data } = await fetchUpdateToken({ refreshToken: local.get('refreshToken') })
    if (data?.accessToken) {
      const { accessExpiresAt, refreshExpiresAt } = data
      local.set('accessToken', data.accessToken, accessExpiresAt)
      local.set('refreshToken', data.refreshToken, refreshExpiresAt)
      suppressAuthToast = false // refreshed OK — resume normal error toasts
      return
    }
  } catch {
    // fall through to session-expired handling below
  }
  // 刷新失败，退出
  await handleSessionExpired()
}

function showError(error) {
  // skip if message is not needed
  const code = Number(error.code)
  if (ERROR_NO_TIP_STATUS.includes(code)) return
  // skip while a token refresh / session-expiry is being handled centrally
  if (suppressAuthToast) return
  window.$message.error(error.message)
}

const { onAuthRequired, onResponseRefreshToken } = createServerTokenAuthentication({
  refreshTokenOnSuccess: {
    isExpired: async (response, method) => {
      // Only authenticated requests should attempt a token refresh.
      // Login / public requests set `authRole: null`; the refresh request itself
      // sets `authRole: 'refreshToken'`. Neither must trigger a refresh — their
      // real errors (e.g. wrong password) need to surface normally.
      const authRole = method.meta?.authRole
      if (authRole === null || authRole === 'refreshToken') return false
      const res = await response.clone().json()
      const alreadyRetried = method.meta && method.meta.isExpired
      return (response.status === 401 || res.code === 4010 || res.code === 4011) && !alreadyRetried
    },
    handler: async (response, method) => {
      // Mark this request as retried so a second failure does not loop.
      if (!method.meta) {
        method.meta = { isExpired: true }
      } else {
        method.meta.isExpired = true
      }
      await handleRefreshToken()
    },
  },
  assignToken: (method) => {
    const accessToken = local.get('accessToken')
    if (accessToken)
      method.config.headers.Authorization = `Bearer ${accessToken}`
  },
})

export function createAlovaInstance(alovaConfig, backendConfig) {
  const _backendConfig = { ...DEFAULT_BACKEND_OPTIONS, ...backendConfig }
  const _alovaConfig = { ...DEFAULT_ALOVA_OPTIONS, ...alovaConfig }

  return createAlova({
    statesHook: VueHook,
    requestAdapter: adapterFetch(),
    cacheFor: null,
    baseURL: _alovaConfig.baseURL,
    timeout: _alovaConfig.timeout,

    beforeRequest: onAuthRequired((method) => {
      if (method.meta?.isFormPost) {
        method.config.headers['Content-Type'] = 'application/x-www-form-urlencoded'
        method.data = new URLSearchParams(method.data).toString()
      }
      alovaConfig.beforeRequest?.(method)
    }),
    responded: onResponseRefreshToken({
      // success interceptor
      onSuccess: async (response, method) => {
        const { status } = response

        if (status === 200 || status === 201) {
          // return blob data
          if (method.meta?.isBlob) return response.blob()

          // return raw json data (bypass standard response validation)
          if (method.meta?.isRawResponse) {
            const rawData = await response.json()
            return { isSuccess: true, data: rawData }
          }

          // return json data
          const apiData = await response.json()
          // request success
          if (apiData[_backendConfig.codeKey] === _backendConfig.successCode)
            return handleServiceResult(apiData)

          // business error
          const errorResult = handleBusinessError(apiData, _backendConfig)
          return handleServiceResult(errorResult, false)
        }
        // api service error
        const errorResult = await handleResponseError(response)
        return handleServiceResult(errorResult, false)
      },
      onError: async (error, method) => {
        const tip = `[${method.type}] - [${method.url}] - ${error.message}`
        window.$message?.warning(tip)
      },

      onComplete: async (_method) => {
        // success
      },
    }),
  })
}
