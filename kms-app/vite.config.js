import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/kms': 'http://localhost:5001',
      '/cluster': 'http://localhost:5001',
      '/events': 'http://localhost:5001',
      '/status': 'http://localhost:5001'
    }
  }
})
