import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  server: {
    proxy: {
      '/heimdallr.v1.': {
        target: 'http://localhost:8484',
        changeOrigin: true,
      },
    },
  },
})
