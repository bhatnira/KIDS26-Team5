import { fileURLToPath } from 'node:url'
import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'
import Icons from 'unplugin-icons/vite'
import UnoCSS from 'unocss/vite'
import { serviceConfig } from '../../service.config.js'

function urlMap(mode) {
  const cfg = serviceConfig[mode] || serviceConfig.production
  return { url: { path: cfg.url } }
}

export function createVitePlugins(env) {
  const plugins = [
    vue(),
    vueJsx(),
    UnoCSS(),
    AutoImport({
      imports: ['vue', 'vue-router', 'vue-i18n', 'pinia', '@vueuse/core'],
      dts: 'src/auto-imports.d.ts',
    }),
    Components({
      dts: 'src/components.d.ts',
      resolvers: [NaiveUiResolver()],
    }),
    Icons({ autoInstall: true }),
  ]

  if (env.VITE_GZIP === 'true') {
    throw new Error('vite-plugin-compression is not installed; set VITE_GZIP=false')
  }

  if (env.VITE_DEVTOOLS === 'true') {
    throw new Error('vite-plugin-vue-devtools is not installed; set VITE_DEVTOOLS=false')
  }

  const antelope = {
    name: 'antelope:url-map',
    // The URL map depends on the build mode (dev vs. production), which is only
    // known inside Vite's config hook — not at the outer env-args stage.
    config(_config, env) {
      const mode = env.command === 'build' ? 'production' : 'development'
      return {
        define: {
          __URL_MAP__: JSON.stringify(urlMap(mode)),
        },
      }
    },
  }

  plugins.push(antelope)
  return plugins
}