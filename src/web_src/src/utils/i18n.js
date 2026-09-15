import { i18n } from '@/modules/i18n'
import { dateZhCN, zhCN } from 'naive-ui'

export function setLocale(locale) {
  i18n.global.locale.value = locale
}

export const $t = i18n.global.t

export const naiveI18nOptions = {
  zhCN: {
    locale: zhCN,
    dateLocale: dateZhCN,
  },
  enUS: {
    locale: null,
    dateLocale: null,
  },
}
