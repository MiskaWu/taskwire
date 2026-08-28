#!/usr/bin/env bash
# taskwire-config.sh —— 設定的單一真相來源（2026-08-28 拍板：全部納管）。
#
# 在這之前，旋鈕散在三個地方：腳本裡的硬編預設、臨時的環境變數、systemd unit 檔。
# 於是「要改一個值」沒有固定的地方可去，控制台也無從下手。現在統一成一個
# ~/.config/taskwire/config.env，純 KEY=value，人手改得動、python 也讀得動。
#
# 讀取優先序：**環境變數 > config.env > 腳本內硬編預設**。
# 這個順序是刻意的：臨時測試仍然可以用 `TASK_REPO=... task mine` 覆蓋，
# 不會被設定檔綁死。實作方式是「只在該變數還沒設定時才套用」——
# 不是無條件 source，那樣會反過來讓設定檔壓過環境變數。
#
# 各腳本這樣接：
#   _TW_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
#   . "$_TW_DIR/taskwire-config.sh"
# readlink -f 不能省——bin/task 是 symlink 進 ~/.local/bin/，
# 直接用 dirname "$0" 會指到 ~/.local/bin，找不到這支共用檔。

TASKWIRE_CONFIG="${TASKWIRE_CONFIG:-$HOME/.config/taskwire/config.env}"

taskwire_load_config() {
  [ -f "$TASKWIRE_CONFIG" ] || return 0
  local line k v
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in ''|'#'*) continue ;; esac
    case "$line" in *=*) : ;; *) continue ;; esac
    k="${line%%=*}"; v="${line#*=}"
    # 去掉 key 前後空白與 value 的成對引號；不做 shell 展開（設定檔不是腳本）。
    k="${k#"${k%%[![:space:]]*}"}"; k="${k%"${k##*[![:space:]]}"}"
    case "$k" in [A-Za-z_]*) : ;; *) continue ;; esac
    v="${v#"${v%%[![:space:]]*}"}"; v="${v%"${v##*[![:space:]]}"}"
    case "$v" in
      \"*\") v="${v#\"}"; v="${v%\"}" ;;
      \'*\') v="${v#\'}"; v="${v%\'}" ;;
    esac
    # 已經有值（來自環境變數）就不動它。
    [ -n "${!k:-}" ] || export "$k=$v"
  done < "$TASKWIRE_CONFIG"
}

taskwire_load_config
