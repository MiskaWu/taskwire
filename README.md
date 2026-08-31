# taskwire

把 GitLab work items 接成可以被 Claude 驅動的工作流：標籤狀態機、webhook 門鈴、
無頭取件。工作清單本體（work items）在內部 GitLab 的 `docs/backlogs` 專案；
這裡只放工具。設計討論見 artifact「Backlog 接線設計」（claude.ai）。

## 元件

| 檔案 | 角色 |
|---|---|
| `bin/task` | 指令列入口。標籤狀態機 v2：收集（無標籤）→ todo → doing → done → 關單，旁支 blocked。**只包含代理被允許的動作**——拉 todo（授權）與關單（驗收）刻意沒有指令。另管工作區標籤：`task ws`（`workspace::` 路徑＋兩形 `base::` 基準）、`task mode`（direct／read 執行方式）。 |
| `cmd/task-webhookd` | 門鈴（:9587，Go）。驗 GitLab signing token（Standard Webhooks 簽章）後**往門鈴信箱投紙條**——它不再起任何子行程，全部能力就是按鈴（一個位元），因此跑在 podman 容器裡（quadlet，`Containerfile.webhook` 多階段編譯靜態執行檔進 **scratch**——映像裡連 shell 都沒有；只掛密鑰唯讀＋信箱可寫）。起 dispatch 歸 `task-doorbell.path`（systemd 盯信箱）。沒設密鑰時 ConditionPathExists 讓它安靜不啟動，機器退化成純輪巡模式。 |
| `bin/task-dispatch` | 無頭取件器。`flock` 防重入；`task next` 取件；照 `workspace::` 標籤對位——單 repo 開（或重用）worktree 站進去、多 repo 目錄站目錄、`mode::direct` 進場前機械檢查主 checkout 乾淨；起 `claude -p` 照協定做；退出後機械兜底——單子停在 doing 就補 block，永不留下「看起來有人在做、其實沒人在做」。開場先收門鈴信箱（在搶鎖之前）。 |
| `bin/task-gc` | 對帳回收（dispatch 收尾與日報都會跑，手動 `task gc`）。worktree：「單子已關＋乾淨＋尖端已併入基準」三條件齊才刪（含放行 remote 的 `task/<N>` 遠端分支），不齊通知不刪。標籤：沒有 open 單掛著的 `base::` 屍體一併回收。查詢失敗大聲中止——查不到 ≠ 沒人用。 |
| `bin/task-notify` | 系統級通知管道：Discord 推播（`~/.config/taskwire/discord-webhook`，選配）＋ GitLab「🤖 taskwire 系統健康」卡留痕掛 blocked。憑證失效、token 到期等異常都走這裡。 |
| `cmd/task-ui` | 本機控制台（127.0.0.1:9588，Go）。設定、密鑰、服務與排程、log、通知測試，加上**唯讀**的單況。拉 todo 與關單刻意沒有按鈕——理由跟 `bin/task` 裡沒有那兩個指令是同一個。前端是 React 19＋TypeScript＋Vite（`ui/`，經 vite-plugin-singlefile 產單檔、`go:embed` 嵌進執行檔；log 用 SSE 即時跟隨），版型照 artifact「taskwire 控制台」設計稿。build：repo 根目錄 `make`。 |
| `bin/taskwire-config.sh`<br>`internal/config/` | 設定的單一真相（`~/.config/taskwire/config.env`）。bash 與 Go 兩份實作同義，優先序都是**環境變數 > config.env > 內建預設**。旋鈕的目錄在 `internal/config/config.go` 的 `Settings`，控制台的表單由它長出來。 |
| `bin/taskwire-install` | 檔案腳印的單一清單。`install`（冪等就位）、`status`（逐項對帳）、`uninstall`（照表拆光，設定與狀態預設保留）。 |
| `systemd/` | user units：`task-webhook.service`、`task-ui.service`（兩個常駐）、`task-scan.timer`（每小時對帳，K8s 式 periodic resync，`Persistent=true`）、`task-digest.timer`（每日日報）。另有 `task-ui.container`（podman quadlet，與 `task-ui.service` 二選一）。 |

## 安裝（新機器）

```sh
./bin/taskwire-install install    # symlink、unit 複本、quadlet、skill link 一次就位（冪等）
# 執行檔缺的話 install 會自己 build（需要 go 與 npm；之後手動重 build 就跑 make）
# 密鑰：openssl rand -hex 32 > ~/.config/taskwire/webhook-secret && chmod 600 同檔
#（或裝完在控制台 http://127.0.0.1:9588/ 的密鑰區按「重新產生」）
task doctor
```

腳印的完整清單就在 `bin/taskwire-install` 開頭的表裡；`taskwire-install status`
逐項對帳（含 unit 複本與 repo 分岔的偵測），`taskwire-install uninstall` 照表拆光
（設定與狀態預設保留，`--purge` 才刪）。門鈴預設走 podman quadlet，
機器上沒有 podman 會自動退回直跑 unit。

裝完開 <http://127.0.0.1:9588/>，其餘設定在頁面上轉。設定寫進
`~/.config/taskwire/config.env`；排程改動寫成 systemd drop-in，
所以之後再跑一次上面的 `cp` 也不會把調整洗掉。

### 想用 podman 跑控制台

```sh
podman build -t taskwire-ui:latest -f Containerfile .
mkdir -p ~/.config/containers/systemd
cp systemd/task-ui.container ~/.config/containers/systemd/
systemctl --user disable --now task-ui.service    # 與直跑版二選一，兩者搶同一個埠
systemctl --user daemon-reload && systemctl --user start task-ui.service
```

先讀 `task-ui.container` 裡的掛載清單。它分成兩段：上半段是控制台的本業
（自己的設定、狀態、repo），掛了不影響隔離；下半段每一行都是為了讓某顆按鈕
真的按得動而交出去的宿主機能力——其中 `/run/user/<uid>` 給的是**整個 user
systemd 的控制權**，不只 taskwire 的那幾個 unit。少給哪一項，對應功能會自動
降級成在頁面上印指令給你複製，不會壞掉。

GitLab 端：backlogs 專案 Settings → Webhooks，URL `http://<主機IP>:9587/hook`，
Signing token 填密鑰檔內容，勾 Work item events 與 Comments。
內網目標要 GitLab 管理員在 Outbound requests 允許清單放行網段（CIDR）。
Windows 防火牆放行 9587 入站。

## 設計要點（改動前先讀）

- **判斷放模型，規則只守不可逆**（2026-08-28 拍板）：驗收閘門已拆除，
  不要在沒有新拍板的情況下加回任何攔「判斷品質」的機械檢查。
  傷害邊界維持機械：關單不歸代理、worktree 不推 origin、token 輪替不由代理觸發。
- 查詢失敗必須長得不像空清單——`_glab_json` 統一把關，別繞過它。
- 代理留言一律帶 `🤖 [claude]` 前綴（`_note`），同帳號下的作者標記。
- 控制台是設定與觀測，**單況唯讀**。不加拉 todo／關單按鈕——那道防線是協定性的
  （每一處都說「這兩件不歸代理」），不是靠技術擋住的，所以一致性就是它的全部力量。
