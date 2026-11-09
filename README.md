# taskwire

讓 Claude Code 在背景處理 GitLab 上的待辦。你把單標成 `todo`，taskwire 會叫醒一個無頭的 Claude，在獨立的 git worktree 裡動手，把過程留言在單上，做完標成 `done` 等你驗收。

給已經用 GitLab 開單、想把講得清楚的工作交給 Claude 的人。要不要做（拉進 `todo`）和算不算完成（關單）留在你手上：`task` 指令刻意沒有這兩個動作。

## 用起來長這樣

**1. 開單、標工作區，然後由你排進佇列。** 前兩行 Claude 在對話裡也會做，最後一行只有你做。

```sh
task new "把 CI 快取搬到物件儲存" "驗收：重跑 CI 兩次都命中快取"
task ws 15 demo-app     # 路徑相對 ~/projects
glab issue update 15 --label todo -R gitlab.example.com/acme/backlogs
```

**2. 背景接單。** GitLab 的 webhook 會響門鈴；沒設 webhook 的機器每小時輪詢一次。`task-dispatch` 取一張單、開好 worktree、用 `claude -p` 做完，過程寫進 `~/.local/state/taskwire/dispatch.log`：

```text
=== dispatch 2026-11-09 16:31:05 ===
取件：#15  [todo]  把 CI 快取搬到物件儲存
https://gitlab.example.com/acme/backlogs/-/issues/15

排隊中的單：task start 接手就開工。
處理 #15，claude log → ~/.local/state/taskwire/run-20261109-163105-15.log
worktree 開好：~/.local/state/taskwire/worktrees/15（基準 origin/main）
claude 退出 rc=0
#15 最終狀態：done workspace::demo-app，不需兜底。
（GC：沒有要清的；1 個在途 worktree）
=== dispatch 結束 2026-11-09 16:31:06（exit=0）===
```

Claude 如果中途掛掉，單子不會停在 `doing`。`task-dispatch` 會在它退出後補上 `blocked` 和一則留言，說明是異常結束、log 在哪裡。

**3. 你回來驗收。** `task ball` 列出等你的單，`task show` 看 Claude 留下的紀錄。滿意就關單，要重做就把標籤從 `done` 改回 `todo`。

```console
$ task ball

做完了，等你驗收（done）
  #15   done,workspace::demo-app 把 CI 快取搬到物件儲存

卡住等你回話（blocked）
  #18   blocked                  匯出報表要不要分頁

$ task show 15 | tail -6
you commented 2026-11-09 08:31:05 UTC
🤖 [claude] 上工：先改快取金鑰，再搬儲存位置

you commented 2026-11-09 08:31:05 UTC
🤖 [claude] 快取已搬完，重跑 CI 兩次都命中
```

還沒排進佇列的單，`task start` 會拒絕接：

```console
$ task start 21 "先動手"
錯誤：#21 還在收集箱。授權是使用者的動作：先由使用者拉 todo（glab issue update 21 --label todo -R gitlab.example.com/acme/backlogs）。
```

<sub>以上是實際執行 `bin/task` 與 `bin/task-dispatch` 的輸出。GitLab 和 Claude 換成了本機的替身（假的 `glab` 與 `claude`），單子是虛構的，家目錄縮寫成 `~`。</sub>

## 需要什麼

- Linux，或有開 systemd 的 WSL2。常駐服務和排程都是 systemd user unit。
- `git`、`jq`、[`glab`](https://gitlab.com/gitlab-org/cli)，以及已登入的 `claude`。
- `go` 與 `npm`，用來編譯控制台和門鈴。
- 一個當工作清單的 GitLab 專案，裡面要先建好 `todo`、`doing`、`blocked`、`done` 四個標籤。token 要能讀寫單子、建立與刪除標籤。
- [claude-kit](https://github.com/MiskaWu/claude-kit) 的 `git-push-guard.py` 放在 `~/.claude/hooks/`。無頭 Claude 的設定（`headless/settings.json`）會呼叫它。
- `podman` 可有可無，沒有的話門鈴直接跑在主機上。

## 安裝（新機器）

一定要放在 `~/projects/taskwire`：systemd unit 的啟動路徑寫死在這裡，`task ws` 的路徑也都相對於 `~/projects`。

```sh
git clone https://github.com/MiskaWu/taskwire.git ~/projects/taskwire
cd ~/projects/taskwire
./bin/taskwire-install install    # symlink、unit 複本、quadlet、skill link 一次就位（冪等）
# 執行檔缺的話 install 會自己 build（需要 go 與 npm；之後手動重 build 就跑 make）
```

接著指定工作清單並登入 GitLab。路徑一定要帶主機名，只寫 `<group>/<repo>` 的話 glab 會去打 gitlab.com。

```sh
echo 'TASK_REPO=gitlab.example.com/<群組>/<專案>' >> ~/.config/taskwire/config.env
glab auth login --hostname gitlab.example.com
task doctor
```

`task doctor` 全部通過時長這樣（同樣是替身環境的輸出）：

```text
API        可用
身分       you
token      2027-06-30 到期，還有 232 天
無頭憑證   正常（refresh 活著）
標籤寫入   可用（探針建刪成功）
標籤       四個都在（todo doing blocked done）
```

腳印的完整清單就在 `bin/taskwire-install` 開頭的表裡；`taskwire-install status`
逐項對帳（含 unit 複本與 repo 分岔的偵測），`taskwire-install uninstall` 照表拆光
（設定與狀態預設保留，`--purge` 才刪）。任一動作加 `-n` 只印不做。門鈴預設走 podman quadlet，
機器上沒有 podman 會自動退回直跑 unit。

裝完開 <http://127.0.0.1:9588/>，其餘設定在頁面上轉。設定寫進
`~/.config/taskwire/config.env`；排程改動寫成 systemd drop-in，
所以之後重跑 `taskwire-install install` 也不會把調整洗掉。

### 接上 GitLab webhook（選用）

沒接 webhook 也能用，只是最多要等一小時才會接單。先產生密鑰（或裝完在控制台的密鑰區按「重新產生」）：

```sh
openssl rand -hex 32 > ~/.config/taskwire/webhook-secret
chmod 600 ~/.config/taskwire/webhook-secret
```

GitLab 端：backlogs 專案 Settings → Webhooks，URL `http://<主機IP>:9587/hook`，
Signing token 填密鑰檔內容，勾 Work item events 與 Comments。
內網目標要 GitLab 管理員在 Outbound requests 允許清單放行網段（CIDR）。
跑在 WSL 的話，Windows 防火牆也要放行 9587 入站。

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

## 邊界與限制

- **無頭 Claude 直接跑在主機上，沒有沙箱。** `headless/settings.json` 擋 `ssh`、`scp`、`sftp`、`rsync`、`aws`、`gh pr create`，以及讀寫 `~/.ssh`、`~/.aws`，但這些規則防的是失誤，防不了刻意繞路。目前的假設是單子都是你自己開的，不要把外部來源的內容直接排進佇列。
- 無頭 Claude 用獨立的設定目錄（`~/.config/taskwire/headless-claude`），不開 `--dangerously-skip-permissions`。需要人確認的動作會被自動拒絕；照協定，它要把單標成 `blocked` 回來問你。
- 單次會話上限預設 3600 秒（`TASK_DISPATCH_TIMEOUT`），一次處理一張單，用 `flock` 防重入。
- 控制台預設只綁 `127.0.0.1`，走 http 明文，頁面上有 webhook 密鑰與服務控制權，不建議改綁 `0.0.0.0`。

## 元件

| 檔案 | 角色 |
|---|---|
| `bin/task` | 指令列入口，`task help` 有完整用法。標籤狀態機：收集（無標籤）→ todo → doing → done → 關單，旁支 blocked。**只包含代理被允許的動作**——拉 todo（授權）與關單（驗收）刻意沒有指令。另管工作區標籤：`task ws`（`workspace::` 路徑＋兩形 `base::` 基準）、`task mode`（direct／read 執行方式）。 |
| `cmd/task-webhookd` | 門鈴（:9587，Go）。驗 GitLab signing token（Standard Webhooks 簽章）後**往門鈴信箱投紙條**——它不再起任何子行程，全部能力就是按鈴（一個位元），因此跑在 podman 容器裡（quadlet，`Containerfile.webhook` 多階段編譯靜態執行檔進 **scratch**——映像裡連 shell 都沒有；只掛密鑰唯讀＋信箱可寫）。起 dispatch 歸 `task-doorbell.path`（systemd 盯信箱）。沒設密鑰時 ConditionPathExists 讓它安靜不啟動，機器退化成純輪巡模式。 |
| `bin/task-dispatch` | 無頭取件器。`flock` 防重入；`task next` 取件；照 `workspace::` 標籤對位——單 repo 開（或重用）worktree 站進去、多 repo 目錄站目錄、`mode::direct` 進場前機械檢查主 checkout 乾淨；起 `claude -p` 照協定做；退出後機械兜底——單子停在 doing 就補 block，永不留下「看起來有人在做、其實沒人在做」。開場先收門鈴信箱（在搶鎖之前）。 |
| `bin/task-gc` | 對帳回收（dispatch 收尾與日報都會跑，手動 `task gc`）。worktree：「單子已關＋乾淨＋尖端已併入基準」三條件齊才刪（含放行 remote 的 `task/<N>` 遠端分支），不齊通知不刪。標籤：沒有 open 單掛著的 `base::` 屍體一併回收。查詢失敗大聲中止——查不到 ≠ 沒人用。 |
| `bin/task-notify` | 系統級通知管道：Discord 推播（`~/.config/taskwire/discord-webhook`，選配）＋ GitLab「🤖 taskwire 系統健康」卡留痕掛 blocked。憑證失效、token 到期等異常都走這裡。 |
| `cmd/task-ui` | 本機控制台（127.0.0.1:9588，Go）。設定、密鑰、服務與排程、log、通知測試，加上**唯讀**的單況。拉 todo 與關單刻意沒有按鈕——理由跟 `bin/task` 裡沒有那兩個指令是同一個。前端是 React 19＋TypeScript＋Vite（`ui/`，經 vite-plugin-singlefile 產單檔、`go:embed` 嵌進執行檔；log 用 SSE 即時跟隨）。build：repo 根目錄 `make`。 |
| `bin/taskwire-config.sh`<br>`internal/config/` | 設定的單一真相（`~/.config/taskwire/config.env`）。bash 與 Go 兩份實作同義，優先序都是**環境變數 > config.env > 內建預設**。旋鈕的目錄在 `internal/config/config.go` 的 `Settings`，控制台的表單由它長出來。 |
| `bin/taskwire-install` | 檔案腳印的單一清單。`install`（冪等就位）、`status`（逐項對帳）、`uninstall`（照表拆光，設定與狀態預設保留）。 |
| `systemd/` | user units：`task-webhook.service`、`task-ui.service`（兩個常駐）、`task-scan.timer`（每小時對帳，K8s 式 periodic resync，`Persistent=true`）、`task-digest.timer`（每日日報）。另有 `task-ui.container`（podman quadlet，與 `task-ui.service` 二選一）。 |

## 設計要點（改動前先讀）

- **判斷放模型，規則只守不可逆**（2026-08-28 拍板）：驗收閘門已拆除，
  不要在沒有新拍板的情況下加回任何攔「判斷品質」的機械檢查。
  傷害邊界維持機械：關單不歸代理、worktree 的推送交給 `git-push-guard` hook 判斷（沒放行的 repo 不推，放行的也絕不推 main）、token 輪替不由代理觸發。
- 查詢失敗必須長得不像空清單——`_glab_json` 統一把關，別繞過它。
- 代理留言一律帶 `🤖 [claude]` 前綴（`_note`），同帳號下的作者標記。
- 控制台是設定與觀測，**單況唯讀**。不加拉 todo／關單按鈕——那道防線是協定性的
  （每一處都說「這兩件不歸代理」），不是靠技術擋住的，所以一致性就是它的全部力量。

## 開發

```sh
make test    # bin/task 的狀態判斷測試，離線，用假的 glab
```

開發慣例見 [CLAUDE.md](CLAUDE.md)。

## 授權

[MIT](LICENSE)
