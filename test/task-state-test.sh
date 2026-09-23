#!/usr/bin/env bash
# bin/task 的狀態判斷測試：_position，以及 start／block／done 對已關閉單的拒絕。
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
  '{"state":"opened","labels":["workspace::hydrogen","mode::read"]}=inbox' \
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

echo
if [ "$fail" -ne 0 ]; then echo "有失敗"; exit 1; fi
echo "全部通過"
