import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import os from 'os'

// Dynamically find the first non-internal IPv4 address on the machine.
// Falls back to localhost if none found (e.g. no network interface up).
function getLocalIP() {
  const interfaces = os.networkInterfaces()
  for (const iface of Object.values(interfaces)) {
    for (const addr of iface) {
      if (addr.family === 'IPv4' && !addr.internal) {
        return addr.address
      }
    }
  }
  return 'localhost'
}

const localIP = getLocalIP()
const NODE1 = 'http://localhost:5001'

console.log(`[vite] Proxying API to: ${NODE1} (local IP: ${localIP})`)

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    host: '0.0.0.0',
    proxy: {
      '/kms':      { target: NODE1, changeOrigin: true },
      '/cluster':  { target: NODE1, changeOrigin: true },
      '/events':   { target: NODE1, changeOrigin: true },
      '/status':   { target: NODE1, changeOrigin: true },
      '/bitsecure': {
        target: 'http://localhost:7777',
        rewrite: (path) => path.replace(/^\/bitsecure/, '')
      }
    }
  }
})
