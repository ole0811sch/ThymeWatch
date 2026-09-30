import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueDevTools from 'vite-plugin-vue-devtools'
import electron from 'vite-plugin-electron/simple'

// https://vite.dev/config/
export default defineConfig({
  ...(process.env.VITE_ELECTRON && {
    base: './',
    build: {
      outDir: 'dist-electron',
    },
  }),
  plugins: [vue(), vueDevTools()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
})
