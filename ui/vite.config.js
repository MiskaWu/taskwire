import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
// dev 模式把 /api 代理到本機服務；正式環境由 bin/task-ui 直接伺服 dist/。
export default defineConfig({
  plugins: [react()],
  build: { outDir: 'dist' },
  server: { proxy: { '/api': 'http://127.0.0.1:9588' } },
})
