# taskwire 的兩支常駐服務都是 Go 執行檔（2026-08-31 改寫，對齊 canopy）。
# build 順序固定：前端先（Vite 把產物放進 internal/webui/dist），
# Go 再用 go:embed 把它包進 task-ui——dist 不存在時 go build 會直接失敗，
# 這是刻意的：寧可 build 不過，也不要一顆端出空白頁的執行檔。
#
# 改前端後：make（三步：npm build → go build → 自己 restart task-ui）。
# 只改 Go：make backend。
.PHONY: all frontend backend

all: frontend backend

frontend:
	cd ui && npm run build

backend:
	go build -o bin/task-ui ./cmd/task-ui
	go build -o bin/task-webhookd ./cmd/task-webhookd
