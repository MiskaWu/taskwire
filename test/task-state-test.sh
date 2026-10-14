#!/usr/bin/env bash
# bin/task 的狀態判斷測試：_position，start／block／done 對已關閉單的拒絕，以及清單與留言的分頁取完。
# 不連網路：source bin/task 取得函式後，用假的 glab 取代真的——issue view 回預先寫好的 JSON，
# 其餘呼叫（留言、改標籤）只記下來，拿來確認「被擋下時完全沒有寫入」。
#
#   bash test/task-state-test.sh     （或 make test）
set -uo pipefail
cd "$(dirname "$0")/.."
# shellcheck source=../bin/task
source bin/task
set +e   # 測試本身要能收集失敗；被測的程式碼在 run() 的子 shell 裡照原樣以 set -euo pipefail 執行

CALLS="$(mktemp)"
trap 'rm -f "$CALLS"' EXIT
declare -A FAKE_ISSUES=(
  [1]='{"iid":1,"state":"closed","labels":[]}'
  [2]='{"iid":2,"state":"closed","labels":["done"]}'
  [3]='{"iid":3,"state":"closed","labels":["todo"]}'
  [4]='{"iid":4,"state":"opened","labels":[]}'
  [5]='{"iid":5,"state":"opened","labels":["todo"]}'
  [6]='{"iid":6,"state":"opened","labels":["doing"]}'
)
glab() {
  printf '%s\n' "$*" >> "$CALLS"
  if [ "$1 $2" = "issue view" ]; then printf '%s' "${FAKE_ISSUES[$3]}"; fi
  return 0
}
task-notify() { printf 'task-notify %s\n' "$*" >> "$CALLS"; }

fail=0
check() { # $1=名稱 $2=實際 $3=預期
  if [ "$2" = "$3" ]; then printf 'ok    %s\n' "$1"
  else printf 'FAIL  %s\n      實際：%s\n      預期：%s\n' "$1" "$2" "$3"; fail=1; fi
}
run() { ( set -euo pipefail; "$@" ) 2>&1; }
writes() { grep -v '^issue view' "$CALLS" | tr '\n' ';'; }

# --- _position：關閉優先於標籤，標籤間 doing > done > todo／blocked
for c in \
  '{"state":"closed","labels":[]}=closed' \
  '{"state":"closed","labels":["done"]}=closed' \
  '{"state":"closed","labels":["todo"]}=closed' \
  '{"state":"closed","labels":["doing"]}=closed' \
  '{"state":"opened","labels":[]}=inbox' \
  '{"state":"opened","labels":["workspace::acme","mode::read"]}=inbox' \
  '{"state":"opened","labels":["todo"]}=todo' \
  '{"state":"opened","labels":["blocked"]}=blocked' \
  '{"state":"opened","labels":["done"]}=done' \
  '{"state":"opened","labels":["doing","todo"]}=doing' \
  '{"state":"opened","labels":["done","todo"]}=done'; do
  check "_position ${c%=*}" "$(_position <<< "${c%=*}")" "${c##*=}"
done

# --- 已關閉的單：start／block／done 都要擋下，而且一筆寫入都沒有
for iid in 1 2 3; do
  for action in "start:接" "block:標卡住" "done:交件"; do
    cmd="${action%%:*}" verb="${action##*:}"
    : > "$CALLS"
    out=$(run main "$cmd" "$iid" "測試摘要"); status=$?
    check "已關閉 #$iid $cmd 被擋" "$status $out" "1 錯誤：#$iid 已經關閉（使用者驗收完成或放棄），不$verb。要重做請使用者重開，或另開新單。"
    check "已關閉 #$iid $cmd 沒有寫入" "$(writes)" ""
  done
done

# --- 還開著的單照原本規則走（反向對照：擋的只有已關閉）
: > "$CALLS"; out=$(run main start 4 "測試摘要"); status=$?
check "收集箱 #4 start 被擋" "$status" "1"
check "收集箱 #4 start 訊息" "${out%%授權*}" "錯誤：#4 還在收集箱。"
check "收集箱 #4 start 沒有寫入" "$(writes)" ""

: > "$CALLS"; out=$(run main start 5 "測試摘要"); status=$?
check "todo #5 start 成功" "$status" "0"
check "todo #5 start 留言並改成 doing" "$(writes)" \
  "issue note 5 -R $REPO -m 🤖 [claude] 上工：測試摘要;issue update 5 -R $REPO --label doing --unlabel todo,blocked,done;"

: > "$CALLS"; out=$(run main done 6 "交件摘要"); status=$?
check "doing #6 done 成功" "$status" "0"
check "doing #6 done 留言、改成 done、發通知" "$(writes)" \
  "issue note 6 -R $REPO -m 🤖 [claude] 交件摘要;issue update 6 -R $REPO --label done --unlabel todo,doing,blocked;task-notify -d -t 球權 🟣 #6 做完待驗收——交件摘要;"

: > "$CALLS"; out=$(run main block 6 "卡住原因"); status=$?
check "doing #6 block 成功" "$status" "0"

# --- 分頁：清單與留言都要逐頁取完，部分結果不能長得像全部（2026-10-14 #71 少兩則留言）
# 換一個懂 -P／-p 的假 glab。真的 GitLab 每頁最多給 100 筆、要再多也不給——假伺服器照比例縮成
# 每頁最多 2 筆（FAKE_MAX），腳本的 _PAGE 也縮成 2。上限一定要做在假伺服器這邊：只靠 _PAGE 的話，
# 「要 -P 100 一次拿完」的舊寫法會被寬鬆的假伺服器照單全給，測試就抓不到截斷。頁數超出回空陣列。
_PAGE=2
FAKE_MAX=2
glab() {
  printf '%s\n' "$*" >> "$CALLS"
  local per=20 page=1 prev="" a
  for a in "$@"; do
    case "$prev" in -P) per=$a ;; -p) page=$a ;; esac
    prev=$a
  done
  [ "$per" -le "$FAKE_MAX" ] || per=$FAKE_MAX
  local from=$(( (page - 1) * per ))
  case "$1 $2" in
    "issue list")   jq -c --argjson f "$from" --argjson n "$per" '.[$f:$f+$n]' <<< "$FAKE_LIST" ;;
    "issue view")   jq -c --argjson f "$from" --argjson n "$per" '.Notes = .Notes[$f:$f+$n]' <<< "$FAKE_VIEW" ;;
    "issue create") printf 'created\n' ;;
  esac
  return 0
}
mk_list() { jq -cn --argjson n "$1" '[range(1; $n + 1) | {iid: ., labels: [], title: "單\(.)", web_url: "https://example/\(.)"}]'; }

FAKE_LIST=$(mk_list 5)
: > "$CALLS"; out=$(run _all_open)
check "清單 5 張分三頁全拿到" "$(jq 'length' <<< "$out")" "5"
check "清單取到不滿一頁才停" "$(grep -c '^issue list' "$CALLS")" "3"

FAKE_LIST=$(mk_list 4)
: > "$CALLS"; out=$(run _all_open)
check "清單剛好兩整頁（邊界）也全拿到" "$(jq 'length' <<< "$out")" "4"

# 同名的單排在第三頁：第一頁沒命中不能當成沒有，否則會重複開單
FAKE_LIST=$(mk_list 5 | jq -c '.[4].title = "已存在的單"')
: > "$CALLS"; out=$(run main new "已存在的單" "說明"); status=$?
check "同名單在後面的頁也找得到" "$status ${out%%：*}" "0 已經有同名的單，不重建"
check "同名單在後面的頁就不開單" "$(grep -c '^issue create' "$CALLS")" "0"

# show：5 則留言＋1 則系統訊息分三頁，全部照時間順序印出、系統訊息不列
note() { jq -cn --argjson i "$1" --arg b "$2" --argjson s "$3" \
  '{id: $i, body: $b, system: $s, author: {username: "MiskaWu"}, created_at: "2026-10-0\($i)T01:02:03.456Z"}'; }
FAKE_VIEW=$(jq -cn --argjson notes "[$(note 1 第一則 false),$(note 2 '標籤變更' true),$(note 3 第二則 false),$(note 4 第三則 false),$(note 5 第四則 false),$(note 6 第五則 false)]" \
  '{iid: 9, title: "測試單", state: "opened", author: {username: "MiskaWu"}, labels: ["workspace::acme"],
    assignees: [], description: "本文", user_notes_count: 5, Notes: $notes}')
: > "$CALLS"; out=$(run main show 9); status=$?
check "show 留言跨頁全部印出" "$status $(grep -oE '^第.則' <<< "$out" | tr '\n' ' ')" "0 第一則 第二則 第三則 第四則 第五則 "
check "show 不列系統訊息" "$(grep -c '標籤變更' <<< "$out")" "0"
check "show 標頭照 glab 格式" "$(sed -n '1,2p;5p' <<< "$out" | tr '\n' '|')" "title:	測試單|state:	open|comments:	5|"
check "show 留言時間格式" "$(grep -m1 ' commented ' <<< "$out")" "MiskaWu commented 2026-10-01 01:02:03 UTC"

# 反向：單上說有 6 則、實際只拿得到 5 則——要大聲失敗，不能印一份看起來完整的單
FAKE_VIEW=$(jq -c '.user_notes_count = 6' <<< "$FAKE_VIEW")
: > "$CALLS"; out=$(run main show 9); status=$?
check "show 留言數對不上就失敗" "$status $out" "1 錯誤：#9 的留言沒取齊：單上有 6 則，只拿到 5 則（這不是全部，別照這份判斷）"

echo
if [ "$fail" -ne 0 ]; then echo "有失敗"; exit 1; fi
echo "全部通過"
