import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { viteSingleFile } from 'vite-plugin-singlefile';

// dev 模式把 /api 代理到本機服務；正式環境的 dist 由 go:embed 嵌進 task-ui
// 執行檔（outDir 直接指到 internal/webui/dist，Go 那邊 //go:embed all:dist）。
// singlefile 把 JS/CSS 全部內嵌進 index.html——改前端後要 npm run build ＋
// go build ＋ restart 服務，三步都在 Makefile 的 `make` 裡。
export default defineConfig({
  plugins: [react(), viteSingleFile()],
  build: { outDir: '../internal/webui/dist', emptyOutDir: true },
  server: { proxy: { '/api': 'http://127.0.0.1:9588' } },
});
