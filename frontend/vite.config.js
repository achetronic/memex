import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    // Proxy API calls to the Go backend during development.
    proxy: {
      '/api': 'http://localhost:8080',
      '/swagger': 'http://localhost:8080',
    },
  },
})
