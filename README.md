# taskwire

把 GitLab work items 接成可以被 Claude 驅動的工作流：標籤狀態機、webhook 門鈴、
無頭取件。工作清單本體（work items）在內部 GitLab 的 `docs/backlogs` 專案；
這裡只放工具。設計討論見 artifact「Backlog 接線設計」（claude.ai）。

## 元件

| 檔案 | 角色 |
|---|---|
| `bin/task` | 指令列入口。標籤狀態機 v2：收集（無標籤）→ todo → doing → done → 關單，旁支 blocked。**只包含代理被允許的動作**——拉 todo（授權）與關單（驗收）刻意沒有指令。 |
| `bin/task-webhookd` | 常駐門鈴（:9587）。驗 GitLab signing token（Standard Webhooks 簽章），issue／work item／note 事件叫醒 dispatch。**門鈴不是資料來源**：payload 不進決策。 |
| `bin/task-dispatch` | 無頭取件器。`flock` 防重入；`task next` 取件；起 `claude -p` 照協定做；退出後機械兜底——單子停在 doing 就補 block，永不留下「看起來有人在做、其實沒人在做」。 |
| `systemd/` | user units：`task-webhook.service`（常駐）、`task-scan.timer`（每小時對帳，K8s 式 periodic resync，`Persistent=true`）。 |

## 安裝（新機器）

```sh
ln -sf ~/projects/taskwire/bin/task ~/.local/bin/task
mkdir -p ~/.config/taskwire ~/.local/state/taskwire
# 密鑰：openssl rand -hex 32 > ~/.config/taskwire/webhook-secret && chmod 600 同檔
cp systemd/* ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now task-webhook.service task-scan.timer
task doctor
```

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
