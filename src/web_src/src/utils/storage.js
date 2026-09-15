const { STORAGE_PREFIX = '' } = import.meta.env

/**
 * LocalStorage Ops
 */
function createLocalStorage() {
  // 默认缓存期限为7天
  function set(key, value, expire = 60 * 60 * 24 * 1000 * 7) {
    const storageData = {
      value,
      expire: new Date().getTime() + expire,
    }
    const json = JSON.stringify(storageData)
    window.localStorage.setItem(`${STORAGE_PREFIX}${String(key)}`, json)
  }

  function get(key) {
    const json = window.localStorage.getItem(`${STORAGE_PREFIX}${String(key)}`)
    if (!json) return null

    const storageData = JSON.parse(json)

    if (storageData) {
      const { value, expire } = storageData
      if (expire === null || expire >= Date.now()) return value
    }
    remove(key)
    return null
  }

  function remove(key) {
    window.localStorage.removeItem(`${STORAGE_PREFIX}${String(key)}`)
  }

  const clear = window.localStorage.clear

  return {
    set,
    get,
    remove,
    clear,
  }
}

/**
 * sessionStorage部分操作
 */
function createSessionStorage() {
  function set(key, value) {
    const json = JSON.stringify(value)
    window.sessionStorage.setItem(`${STORAGE_PREFIX}${String(key)}`, json)
  }
  function get(key) {
    const json = sessionStorage.getItem(`${STORAGE_PREFIX}${String(key)}`)
    if (!json) return null

    const storageData = JSON.parse(json)

    if (storageData) return storageData

    return null
  }
  function remove(key) {
    window.sessionStorage.removeItem(`${STORAGE_PREFIX}${String(key)}`)
  }
  const clear = window.sessionStorage.clear

  return {
    set,
    get,
    remove,
    clear,
  }
}

export const local = createLocalStorage()
export const session = createSessionStorage()
