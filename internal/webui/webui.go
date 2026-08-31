// Package webui 把控制台前端的 build 產物嵌進執行檔（2026-08-31 拍板，對齊
// canopy 的單一執行檔語意）。dist/ 由 ui/ 的 Vite 產出（vite.config.ts 的
// outDir 指到這裡），所以 build 順序固定是：先 cd ui && npm run build，
// 再 go build——dist 不存在時 go:embed 會直接編譯失敗，這是刻意的：
// 寧可 build 不過，也不要一顆端出空白頁的執行檔。
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // embed 的形狀在編譯期就固定了，這裡失敗只可能是程式寫錯。
	}
	return sub
}
