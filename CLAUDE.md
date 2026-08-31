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
   也不要繞過腳本直接下 glab 動這兩件事。worktree 不推 origin（**2026-08-28 使用者暫時放寬**：無頭線 settings 的 push deny 已移除，推送交給 git-push-guard——主 checkout 可推、worktree 要 /allow-push 放行；收回＝把 `Bash(git push:*)` 加回 headless/settings.json 的 deny）；token 輪替不由代理觸發。
3. **查詢失敗必須長得不像空清單**。所有拿 JSON 的 glab 呼叫走 `_glab_json`，
   別繞過它——token 過期那天 `task mine` 要說「連不上」，不能說「沒有待辦」。
4. **webhook 是門鈴，不是資料來源**。payload 不進決策；dispatch 是 level-triggered，
   每次醒來用 `task next` 重讀完整真實狀態。K8s reconcile 的形：標籤=期望狀態、
   dispatch=controller、webhook=提早叫醒、`task-scan.timer`=periodic resync。
   （2026-08-28 門鈴改造）門鈴現在**物理上**只剩一個位元：webhookd 驗簽後往
   `~/.local/state/taskwire/doorbell/` 投紙條，起 dispatch 歸 `task-doorbell.path`；
   dispatch 開場收信（在搶 flock **之前**收，否則 path unit 會空轉）。
   因此 webhookd 可以乾淨容器化（現行部署：quadlet，只掛密鑰 ro＋信箱 rw）。
   沒設密鑰的機器門鈴不啟動（ConditionPathExists 安靜跳過），只剩每小時輪巡——
   這是允許的降級模式，慢但不漏。
5. **兩層寫回**：協定層歸模型（提示教它 done/block），兜底層歸機械
   （dispatch 退出後單子還在 doing 就補 block）。兜底不能刪——它保證單子永遠不會
   停在「看起來有人在做、其實沒人在做」。
6. 代理寫的留言一律經 `_note`（`🤖 [claude]` 前綴）——同一個 GitLab 帳號下的作者標記。
7. **工作區是標籤，回收是對帳**（2026-08-28）：單上貼 `workspace::<相對 ~/projects 路徑>`
   （`task ws` 代勞），dispatch 據此在
   `~/.local/state/taskwire/worktrees/<單號>` 開 worktree（分支 `task/<單號>`）站進去起會話；
   路徑指到多 repo 工作區目錄（如 `hydrogen`）就只站目錄、內部交模型判斷，
   但 worktree 要照 `<單號>-<repo名>` 慣例放同一目錄。基準分支兩形，跟模式對齊：
   單 repo 貼 `base::<分支>`；多 repo 逐 repo 貼 `base::<repo名>::<分支>`（scope 是
   「最後一個 :: 之前」，所以逐 repo 互斥、彼此並存），沒貼的用各自遠端預設主線。
   執行方式另有 `mode::` 旋鈕（`task mode` 代勞）：不貼＝worktree 流程（安全預設）；
   `mode::direct`＝直接修主 checkout，dispatch 進場前機械檢查主 checkout 乾淨、髒就
   block（不可逆的守門，不放）；`mode::read`＝調查單，不開 worktree、只讀，交件是留言。
   標籤詞彙的權責：流程四標由使用者建；`workspace::`／`base::`／`mode::` 由
   `task ws`／`task mode` 代建（`_ensure_label`，失敗大聲死不默默吞），
   沒有 open 單掛著的 `base::` 屍體由 `task-gc` 回收——分支短命、專案標籤永久，
   不掃會堆滿下拉選單。因此 **token 要開標籤建刪權限**，`task doctor` 的
   「標籤寫入」探針會建刪一顆 `zz-taskwire-probe` 直接驗。`task-gc` 只在
   「單子已關＋worktree 乾淨＋尖端已併入基準」三條件齊時才刪（含放行 remote 的
   `task/<N>` 遠端分支），不齊就通知不刪——掉工作的決定留給人。
8. 通知一律走 `task-notify`：例行的用 `-d -t <類型>`（球權／日報／異常，同類回同串、
   新類自動開串）；GitLab 健康卡（blocked）只留給真異常，別讓例行訊息洗掉它。
8. **控制台是設定與觀測，單況唯讀**（2026-08-28）。`task-ui` 不加「拉 todo」與
   「關單」按鈕。理由不是技術上擋得住——無頭 claude 手上有 Bash，它要關單根本
   不必經過任何 HTTP 端點。那道防線是**協定性**的：`task help`、SKILL.md、控制台，
   每一處都說「這兩件不歸代理」，模型才不會把某一處的缺口誤讀成授權。
   頁面上擺一顆關單按鈕，等於從內部把這份一致性拆掉。
9. **設定的單一真相是 `~/.config/taskwire/config.env`**（2026-08-28）。
   優先序**環境變數 > config.env > 腳本內建預設**，bash 端（`taskwire-config.sh`）
   與 python 端（`taskwire_config.py`）兩份實作必須同義——分岔的話控制台顯示的
   就不是腳本真正在用的值，那比沒有控制台更糟。加新旋鈕只改 `taskwire_config.py`
   的 `SETTINGS`，網頁表單會自己長出來，不要在頁面上手寫欄位。

## 早報（雲端 routine）與 taskwire 的邊界（2026-08-28 收尾）

早報是獨立的上層服務（claude.ai routine「Morning brief」，trig_01AqYFffSzcMDYt99U7cEFA4，
平日 08:00 台北）：**讀 taskwire、不借 taskwire 的任何管道**——不用 task-notify、
不進 discord-threads.json，依賴方向單向，上層不寫下層。唯讀鐵則寫死在 routine prompt
（不執行 task pull/done/close/block/new/note）。

- **心跳走 Slack**「早報」頻道（C0BTATMB91U）——連接器流量走 Anthropic 通道，不經沙盒
  網路。**discord.com 被雲端 egress 在 Custom 白名單之上封鎖**（實測：frame 網域通、
  discord CONNECT 403），別再試著讓雲端發 Discord；Discord 論壇串「早報」已停用留檔。
- remote-devices（本機橋）在排程 session 目前不存在 → 早報暫看不到 taskwire；
  prompt 每次試找（上限兩次呼叫），橋回來會自動接上。
- **task-digest 續開**（與早報無重疊）。哪天早報吃得到 taskwire，用控制台的
  「取消自啟」關 digest；心跳職責（天天發、沒發即故障）由早報承接。
- 無人值守的教訓：routine 用到的工具（Artifact、PushNotification、Slack 發送）都要進
  allowed_tools，否則執行會停在權限提示等人。細節見記憶 morning-brief-routine。

## 元件與部署位置

| 元件 | 部署 | 改動後 |
|---|---|---|
| `bin/task` `task-notify` | symlink 進 `~/.local/bin/`，改即生效 | `bash -n` 檢查 |
| `bin/task-dispatch` `task-digest` `task-gc` | 由 systemd／webhookd 以絕對路徑呼叫，改即生效（gc 另有手動入口 `task gc`） | `bash -n` |
| `bin/task-webhookd` | **quadlet 容器**（taskwire-webhook 映像，:9587；程式 COPY 進映像） | `py_compile` → `podman build -t taskwire-webhook:latest -f Containerfile.webhook .` → `systemctl --user restart task-webhook` |
| `systemd/task-doorbell.path` `task-dispatch.service` | 門鈴信箱監看＋dispatch 的 unit 形（`task-scan.timer` 也指向後者） | `cp` → `daemon-reload` |
| `bin/task-ui` `ui/`（React＋Vite） | `task-ui.service` 常駐（127.0.0.1:9588），伺服 `ui/dist` | 改後端：`py_compile` → restart；改前端：`cd ui && npm run build`（不用 restart）。設計稿見 artifact「taskwire 控制台」 |
| `bin/taskwire-config.sh` `taskwire_config.py` | 被上面各支 source／import，改即生效 | `bash -n`／`py_compile` |
| `bin/taskwire-install` | 檔案腳印的單一清單：install／status／uninstall（symlink、unit 複本、quadlet、skill link 一把管；uninstall 預設保留設定與狀態） | `bash -n`；要加腳印改腳本內的表 |
| `systemd/*` | **複本**在 `~/.config/systemd/user/`，repo 是源 | `cp` 過去 → `daemon-reload` → restart／re-enable |
| `skill/SKILL.md` | symlink 於 `~/.claude/skills/taskwire`，改即生效 | — |

機器狀態：設定與密鑰 `~/.config/taskwire/`（config.env；webhook-secret、discord-webhook
皆 0600；三者都不入版控）、執行狀態 `~/.local/state/taskwire/`（dispatch/run log、
discord-threads.json、auth-notify.stamp）。
timers：`task-scan.timer` 每小時對帳、`task-digest.timer` 每天 09:00 日報，都 `Persistent=true`（WSL2 必需）。
**排程改動一律寫 drop-in**（`~/.config/systemd/user/<unit>.d/override.conf`），
不改 unit 檔本體——repo 的 `systemd/` 是出廠值，直接改複本的話下次 `cp` 就把調整洗掉了。
控制台的排程欄走的就是這條路，還原＝刪掉 drop-in。

常駐服務有兩個，都是拍板過的例外：`task-webhook`（2026-08-27）與 `task-ui`（2026-08-28）。
控制台選常駐而非按需的理由：它不會給無頭會話任何它沒有的能力（無頭 claude 有 Bash，
本來就能改設定檔、跑 systemctl），所以「按需」保護不到任何東西，剩下的只有隨時可用。

## 驗證手段

控制台 <http://127.0.0.1:9588/>（doctor、門鈴健康、服務與排程、log、單況都在同一頁）。
指令列：`task doctor`（API／身分／token 到期／無頭憑證／標籤）、
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
- 容器裡 `systemctl --user` 會失敗，錯誤是「Failed to connect to user scope bus via
  local transport: No data available」——**看起來像 systemd 不在，其實只是走錯門**。
  systemctl 預設走 `/run/user/<uid>/systemd/private`，那個 socket 有跨 namespace
  過不了的對端檢查；`SYSTEMCTL_FORCE_BUS=1` 改走 D-Bus 就完全正常（含 restart）。
  `task-ui` 的 `systemd_available()` 會自動探這兩條路，別把它簡化掉。
- 容器內 `journalctl` 說 No journal files found，八成是缺 `/etc/machine-id`——
  journalctl 靠它定位 `/var/log/journal/<machine-id>/`，只掛 journal 目錄不夠。
- 單檔 bind mount 遇上「tmp 檔寫好再 os.replace」的原子換檔會**留在舊 inode**：
  UI 重產 webhook 密鑰後，門鈴容器讀到的還是舊密鑰，必須 restart（UI 存檔後會提示）。
- quadlet 生成的 unit（UnitFileState=generated）不吃 `systemctl enable/disable`，
  開機自啟由 `.container` 的 `[Install]` 決定；控制台對這類 unit 不顯示自啟按鈕。
