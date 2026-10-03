import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    proxy: {
      '/ws': { target: process.env.GAME_SERVER_URL ?? 'ws://127.0.0.1:8080', ws: true },
    },
  },
})
