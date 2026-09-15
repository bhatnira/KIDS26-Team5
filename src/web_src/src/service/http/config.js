import { $t } from '@/utils/i18n'

export const DEFAULT_ALOVA_OPTIONS = {
  // request timout: 99 seconds
  timeout: 99 * 1000,
}

export const DEFAULT_BACKEND_OPTIONS = {
  codeKey: 'code',
  dataKey: 'data',
  msgKey: 'msg',
  successCode: 2000, // http.StatusOk
}

export const ERROR_STATUS = {
  default: $t('http.defaultTip'),
  400: $t('http.400'),
  401: $t('http.401'),
  403: $t('http.403'),
  404: $t('http.404'),
  405: $t('http.405'),
  408: $t('http.408'),
  500: $t('http.500'),
  501: $t('http.501'),
  502: $t('http.502'),
  503: $t('http.503'),
  504: $t('http.504'),
  505: $t('http.505'),
}

export const ERROR_NO_TIP_STATUS = [10000]
