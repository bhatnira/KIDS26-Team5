import { fileURLToPath, URL } from 'node:url'

import { defineConfig, loadEnv } from 'vite'
import { createVitePlugins } from './build/plugins'

const __dirname = fileURLToPath(new URL('.', import.meta.url))

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, __dirname, '')

  return {
    base: env.VITE_BASE_URL,
    plugins: createVitePlugins(env),
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      host: '0.0.0.0',
      port: 9999,
    },
    build: {
      target: 'esnext',
      reportCompressedSize: false, // enable/disable gzip compression size report
    },
    css: {
      preprocessorOptions: {
        scss: {
          api: 'modern',
        }
      }
    },
  }
})
