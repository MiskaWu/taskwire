# taskwire —— 給在這個 repo 工作的 Claude

工作清單本體（work items）在內部 GitLab `hydrogen/roles/engineer/miskawu/docs/backlogs`，
**這個 repo 只放工具**。使用者視角的設計脈絡見 artifact「Backlog 接線設計」與
「Taskwire 下一步」（claude.ai，2026-08 的兩份決策紀錄）。

## 不可違背的設計拍板（改動前先讀，推翻要使用者點頭）

1. **判斷放模型，規則只守不可逆**（2026-08-28）。驗收閘門已拆除：任何攔「判斷品質」
   的機械檢查（驗收寫沒寫、算不算談定、該不該開工）都不要加回來。模型判斷不足時，
   解法是在 `skill/SKILL.md` 的提示清單加一條帶日期與理由的提示（上限五條、自我修剪），
   不是加規則。記憶 `no-mechanical-gates-on-judgment` 同義。
2. **傷害邊界維持機械**：拉 todo（授權）與關單（驗收）是使用者的動作，`bin/task`
   刻意沒有這些指令——**指令的缺席本身就是權限文件**，不要「補全」它們，
   也不要繞過腳本直接下 glab 動這兩件事。worktree 不推 origin；token 輪替不由代理觸發。
3. **查詢失敗必須長得不像空清單**。所有拿 JSON 的 glab 呼叫走 `_glab_json`，
   別繞過它——token 過期那天 `task mine` 要說「連不上」，不能說「沒有待辦」。
4. **webhook 是門鈴，不是資料來源**。payload 不進決策；dispatch 是 level-triggered，
   每次醒來用 `task next` 重讀完整真實狀態。K8s reconcile 的形：標籤=期望狀態、
   dispatch=controller、webhook=提早叫醒、`task-scan.timer`=periodic resync。
5. **兩層寫回**：協定層歸模型（提示教它 done/block），兜底層歸機械
   （dispatch 退出後單子還在 doing 就補 block）。兜底不能刪——它保證單子永遠不會
   停在「看起來有人在做、其實沒人在做」。
6. 代理寫的留言一律經 `_note`（`🤖 [claude]` 前綴）——同一個 GitLab 帳號下的作者標記。
7. 通知一律走 `task-notify`：例行的用 `-d -t <類型>`（球權／日報／異常，同類回同串、
   新類自動開串）；GitLab 健康卡（blocked）只留給真異常，別讓例行訊息洗掉它。

## 元件與部署位置

| 元件 | 部署 | 改動後 |
|---|---|---|
| `bin/task` `task-notify` | symlink 進 `~/.local/bin/`，改即生效 | `bash -n` 檢查 |
| `bin/task-dispatch` `task-digest` | 由 systemd／webhookd 以絕對路徑呼叫，改即生效 | `bash -n` |
| `bin/task-webhookd` | `task-webhook.service` 常駐（:9587） | `python3 -m py_compile` 後 `systemctl --user restart task-webhook` |
| `systemd/*` | **複本**在 `~/.config/systemd/user/`，repo 是源 | `cp` 過去 → `daemon-reload` → restart／re-enable |
| `skill/SKILL.md` | symlink 於 `~/.claude/skills/taskwire`，改即生效 | — |

機器狀態：密鑰 `~/.config/taskwire/`（webhook-secret、discord-webhook，皆 0600，不入版控）、
執行狀態 `~/.local/state/taskwire/`（dispatch/run log、discord-threads.json、auth-notify.stamp）。
timers：`task-scan.timer` 每小時對帳、`task-digest.timer` 每天 09:00 日報，都 `Persistent=true`（WSL2 必需）。

## 驗證手段

`task doctor`（API／身分／token 到期／無頭憑證／標籤）、
`curl -s http://127.0.0.1:9587/healthz`、`journalctl --user -u task-webhook -n 20`、
`systemctl --user list-timers 'task-*'`。無頭行為看 `~/.local/state/taskwire/dispatch.log`
與同目錄 `run-*.log`。webhookd 的簽章驗證有本機自測法：對 body 以
Standard Webhooks 格式（`<webhook-id>.<webhook-timestamp>.<body>` 的 HMAC-SHA256 → base64 →
`v1,` 前綴）簽了打 `/hook`。

## 已踩過的坑（別再踩）

- `_fmt` 的欄位分隔是 ``，**不能換 tab**（IFS 會合併連續 tab，無標籤的單標題會靜默錯位）。
- glab 的 repo 路徑**必須帶主機名**，`glab api` 要用 `--hostname`——少了會靜默打 gitlab.com，症狀是 404/401。
- `glab auth status` 對細粒度 token 必定誤報 Invalid token，健康檢查一律打真實 API。
  細粒度 token 授權是**逐專案**的：對 backlogs 通不代表對別的專案通。
- webhook 事件的 `object_kind` 有 `issue`／`work_item`／`note` 三種都要當門鈴
  （這版 GitLab 把 issue 改叫 work item；note 是使用者補留言的路徑）。
- CLI 憑證檔的 `expiresAt` 是 access token 的短期到期，**過期是常態**（refresh 會救）；
  只有「檔案不存在」或「落後 48h 未刷新」才算異常，別拿它誤報。
- Discord webhook 只能在**論壇頻道**開串（220003）；串被刪是 10003，task-notify 會自動重開。
