import piniaPluginPersistedstate from 'pinia-plugin-persistedstate'

export * from './app/index'
export * from './auth'
export * from './notification'
export * from './router'
export * from './tab'

export function installPinia(app) {
  const pinia = createPinia();
  pinia.use(piniaPluginPersistedstate)
  app.use(pinia)
}
