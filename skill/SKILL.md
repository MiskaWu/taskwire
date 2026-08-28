---
name: taskwire
description: GitLab work items 工作清單（backlogs）的入口與慣例。當討論收斂出待辦或拍板、使用者提到某張單（#N）、backlog、開單、落單、驗收、或要查工作狀態時使用；指令是 task。
---

# taskwire —— backlog 操作慣例

工作清單在內部 GitLab（docs/backlogs 專案）的 work items。指令 `task` 已在 PATH，
`task help` 有完整用法。狀態機：收集（無標籤）→ todo → doing → done → 關單，旁支 blocked。
無頭線（webhook 取件）會自動處理 todo/doing，互動對話的主要角色是：談定、落單、驗收協作、解卡。

## 時機（方向，不是規則——判斷在你）

- **談定就落單。** 對話收斂出需求、拍板、驗收條件時，主動問使用者「要開單／落到 #N 嗎」。
  對話會消失，單不會——結晶必須進單。開單 `task new "<標題>" "<why／現況／落地位置／驗收方向>"`，
  補脈絡 `task note <iid> "…"`。
- **落單時順手標工作區。** `task ws <iid> <相對 ~/projects 的路徑> [基準分支]`——
  無頭線照它開 worktree（指到 git repo）或站進多 repo 工作區目錄（如 `hydrogen`）；
  多 repo 的基準逐 repo 指定：`task ws <iid> hydrogen server=feat-abyss gm=dev`，
  沒指定的用各自遠端預設主線。不標籤＝無頭側自己判斷，判斷不了會 block 回來要你標。
  執行方式 `task mode <iid> <direct|read|off>`：direct＝直接修主 checkout（dispatch 會
  先機械檢查主 checkout 乾淨，髒就 block）、read＝調查單只讀不開 worktree、
  off＝回預設 worktree 流程。direct 放寬安全，由使用者拍板才貼。
- **動手前對一眼 `task mine`。** 無頭線可能正在做同一件事（doing 標籤），撞到先跟使用者確認。
- 接手：`task start <iid> "開工摘要"`（todo／blocked 可接）。卡住：`task block <iid> "卡在哪＋建議"`。
  做完：`task done <iid> "交件摘要＋怎麼驗的"`。
- 驗收條件：單上有就照它；沒有就自己擬、先留單再動工；擬不出就問。沒把握就問，不硬做。

## 邊界（機械，不可越）

- 拉 todo（授權）與關單（驗收）是使用者的動作，`task` 刻意沒有這些指令；
  不要繞過腳本直接用 glab 補標籤。
- 程式改動在目標 repo 的 worktree 進行，commit 不推 origin。

## 判斷提示（上限五條，加第六條先刪一條）

讀到某條提示時，若你判斷現在的你沒有它也會做對，向使用者提議移除。

（目前無——磨合中。出狀況才加，每條帶日期與當初為什麼。）
