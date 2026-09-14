import { fileURLToPath, URL } from 'node:url'

import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  server: {
    port: 5173,
    proxy: {
      // 契约基路径 /api/v1，本地开发代理到后端 :8080
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  },
  build: {
    rollupOptions: {
      output: {
        // 管理端组件库独立分包，避免首屏 vendor 过大
        manualChunks: {
          elementPlus: ['element-plus']
        }
      }
    }
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.ts']
  }
})
