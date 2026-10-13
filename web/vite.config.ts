import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Single-binary contract: build output goes into the Go embed package, and the dev
// server proxies /api to the Go backend on :8080. emptyOutDir is off so the tracked
// .gitkeep survives; package.json's build script clears stale hashed assets first.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../internal/assets/dist',
    emptyOutDir: false,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
