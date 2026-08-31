package main

// systemd 互動層：探測連線方式、unit 狀態、開關服務、排程 drop-in、journal。

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"taskwire/internal/config"
)

// 控制台能碰的 unit 白名單。不在名單上的一律拒絕——這個服務不是通用的
// systemd 遙控器，它只管 taskwire 自己的東西。順序就是頁面上的順序。
type unitDef struct {
	Name  string
	Label string
	Kind  string
	Help  string
}

var units = []unitDef{
	{
		Name:  "task-webhook.service",
		Label: "門鈴（webhook）",
		Kind:  "service",
		Help: "收 GitLab 事件、驗簽章，然後往門鈴信箱投紙條——起 dispatch 的職責" +
			"歸下面的信箱監看。跑在 podman（quadlet）時改 cmd/task-webhookd 要" +
			"重 build 映像。停掉之後改單不會即時觸發，要等對帳 timer 補。",
	},
	{
		Name:  "task-doorbell.path",
		Label: "門鈴信箱監看",
		Kind:  "path",
		Help: "systemd 盯著 doorbell 信箱目錄，紙條一出現就起 task-dispatch；" +
			"dispatch 收工時信箱還有紙條會自動再起（接力）。這是門鈴容器" +
			"唯一碰得到宿主機的通道。",
	},
	{
		Name:  "task-scan.timer",
		Label: "對帳輪詢（每小時）",
		Kind:  "timer",
		Help:  "webhook 漏接時的保底，跑一次 task-dispatch。K8s 式的 periodic resync。",
	},
	{
		Name:  "task-digest.timer",
		Label: "每日巡檢日報",
		Kind:  "timer",
		Help: "球權、在途、系統健康，零 LLM。日報同時是巡檢自己的心跳——" +
			"靜默的巡檢壞掉時沒有人會發現。雲端早報上線後內容與它重疊，" +
			"拍板做法（2026-08-28）：啟用早報時按「取消自啟」把這個關掉；" +
			"關掉後「巡檢還活著嗎」的心跳職責就移交給早報，早報那側要記得接。",
	},
	{
		Name:  "task-ui.service",
		Label: "控制台（就是這一頁）",
		Kind:  "service",
		Help:  "重啟會讓這一頁斷線幾秒，重新整理即可。",
	},
}

func unitByName(name string) *unitDef {
	for i := range units {
		if units[i].Name == name {
			return &units[i]
		}
	}
	return nil
}

// run 跑一個外部指令，回 (rc, 合併輸出)。永遠不回 error——
// 控制台的每一格都可能失敗，失敗要變成畫面上的一段紅字，不是 500。
func run(timeout time.Duration, extraEnv []string, name string, args ...string) (int, string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if ctx.Err() == context.DeadlineExceeded {
		return 124, fmt.Sprintf("逾時（%d 秒）：%s %s", int(timeout.Seconds()), name, strings.Join(args, " "))
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), text
		}
		if errors.Is(err, exec.ErrNotFound) {
			return 127, "找不到指令：" + name
		}
		return 1, err.Error()
	}
	return 0, text
}

var (
	systemdOnce sync.Once
	systemdOK   bool
	systemdEnv  []string
)

// systemdAvailable：能不能碰到 user systemd，以及要用哪條路連。探一次就記住。
//
// 有兩條路：systemctl 預設走 /run/user/<uid>/systemd/private 這個私有 socket，
// 走不通才有 D-Bus。在容器裡（即使 uid 相同、socket 也 mount 進去了）private
// 這條必定失敗——systemd 對它有跨 namespace 過不了的對端檢查，錯誤訊息是
// 「Failed to connect to user scope bus via local transport: No data available」，
// 看起來像「systemd 不在」，實際上只是走錯門。SYSTEMCTL_FORCE_BUS=1 改走 D-Bus
// 就完全正常（2026-08-28 在 podman rootless 實測）。
//
// 所以這裡探兩次：直連不行就帶 FORCE_BUS 再探一次，成功就把它記進 systemdEnv，
// 之後所有 systemctl 呼叫都帶著。直跑的宿主機第一次就會過，不會多付這個成本。
func systemdAvailable() bool {
	systemdOnce.Do(func() {
		if _, err := exec.LookPath("systemctl"); err != nil {
			systemdOK = false
			return
		}
		probe := []string{"--user", "show", "--property=Version"}
		if rc, _ := run(5*time.Second, nil, "systemctl", probe...); rc == 0 {
			systemdOK, systemdEnv = true, nil
			return
		}
		if rc, _ := run(5*time.Second, []string{"SYSTEMCTL_FORCE_BUS=1"}, "systemctl", probe...); rc == 0 {
			systemdOK, systemdEnv = true, []string{"SYSTEMCTL_FORCE_BUS=1"}
			return
		}
		systemdOK = false
	})
	return systemdOK
}

// sysctl：所有 systemctl／journalctl 呼叫都走這裡，才會帶上探測出來的連線方式。
func sysctl(timeout time.Duration, name string, args ...string) (int, string) {
	systemdAvailable()
	return run(timeout, systemdEnv, name, args...)
}

// hostAction：需要宿主機權限的動作。碰得到 systemd 就直接做；碰不到就把指令
// 交回頁面。這個降級不只為了容器——像 claude /login 這種本來就只能由人在
// 終端機做的事，走的也是同一條路。
func hostAction(cmd []string, why string) map[string]any {
	if !systemdAvailable() {
		hint := ""
		if why != "" {
			hint = "（" + why + "）"
		}
		return map[string]any{
			"ok":     false,
			"manual": strings.Join(cmd, " "),
			"error": "這個環境碰不到 user systemd（典型是跑在沒 mount D-Bus 的容器裡）。" +
				"把下面這行貼到終端機執行：" + hint,
		}
	}
	rc, out := sysctl(60*time.Second, cmd[0], cmd[1:]...)
	if rc != 0 {
		if out == "" {
			out = fmt.Sprintf("指令失敗（rc=%d）", rc)
		}
		return map[string]any{"ok": false, "error": out, "manual": strings.Join(cmd, " ")}
	}
	return map[string]any{"ok": true, "output": out}
}

var onCalendarInTimers = regexp.MustCompile(`OnCalendar=([^ ;}]+(?: [^ ;}]+)*)`)

// unitState 回一個 unit 的現況。systemd 碰不到時回 unknown 而不是編一個 inactive——
// 「不知道」和「停著」在畫面上必須長得不一樣。
func unitState(def unitDef) map[string]any {
	info := map[string]any{
		"name": def.Name, "label": def.Label, "kind": def.Kind, "help": def.Help,
	}
	if def.Name == "task-webhook.service" && config.ReadSecret("webhook-secret") == "" {
		info["note"] = "webhook 密鑰未設定——門鈴不會啟動（unit 的 ConditionPathExists " +
			"安靜擋下，這是設計不是故障）。要接 GitLab 就到下方密鑰區設定後" +
			"再啟動；不接的機器靠對帳 timer 每小時輪巡就夠，只是慢。"
	}
	if !systemdAvailable() {
		info["active"], info["enabled"], info["detail"] = "unknown", "unknown", "碰不到 systemd"
		return info
	}
	rc, out := sysctl(10*time.Second, "systemctl", "--user", "show", def.Name,
		"--property=ActiveState,UnitFileState,SubState,ExecMainStartTimestamp")
	props := map[string]string{}
	if rc == 0 {
		for _, line := range strings.Split(out, "\n") {
			if key, val, ok := strings.Cut(line, "="); ok {
				props[key] = val
			}
		}
	}
	info["active"] = valOr(props["ActiveState"], "unknown")
	info["enabled"] = valOr(props["UnitFileState"], "unknown")
	info["sub"] = props["SubState"]
	info["since"] = props["ExecMainStartTimestamp"]
	if strings.HasSuffix(def.Name, ".timer") {
		rc2, out2 := sysctl(10*time.Second, "systemctl", "--user", "show", def.Name,
			"--property=NextElapseUSecRealtime,TimersCalendar")
		if rc2 == 0 {
			for _, line := range strings.Split(out2, "\n") {
				key, val, _ := strings.Cut(line, "=")
				switch key {
				case "NextElapseUSecRealtime":
					info["next"] = val
				case "TimersCalendar":
					if m := onCalendarInTimers.FindStringSubmatch(val); m != nil {
						info["oncalendar"] = strings.TrimSpace(m[1])
					} else {
						info["oncalendar"] = val
					}
				}
			}
		}
	}
	return info
}

func valOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

var systemdUserDir = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user")
}()

func dropinPath(unit string) string {
	return filepath.Join(systemdUserDir, unit+".d", "override.conf")
}

// writeSchedule 把排程改動寫 drop-in，不動 unit 檔本體。
// repo 的 systemd/ 是源、~/.config/systemd/user/ 是複本（見 CLAUDE.md），
// 直接改複本的話，下次照 README 跑 cp 就會把這裡的調整默默洗掉。
// drop-in 是 systemd 為這個情境準備的機制：cp 蓋不到它，源檔永遠是出廠值。
// OnCalendar= 先給一行空值是必要的——不清空的話新值是疊加上去，變成兩個排程點。
func writeSchedule(unit, oncalendar string) map[string]any {
	path := dropinPath(unit)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	body := "# 由 task-ui 產生。repo 的 systemd/ 是出廠值，這個 drop-in 疊在它上面。\n" +
		"# 手改也可以；要回到出廠值就刪掉這個檔再 daemon-reload。\n" +
		"[Timer]\n" +
		"OnCalendar=\n" +
		"OnCalendar=" + oncalendar + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	res := hostAction([]string{"systemctl", "--user", "daemon-reload"}, "讓 drop-in 生效")
	if res["ok"] != true {
		return res
	}
	return hostAction([]string{"systemctl", "--user", "restart", unit}, "套用新排程")
}

func journal(unit string, lines int) map[string]any {
	if unitByName(unit) == nil {
		return map[string]any{"ok": false, "error": "不在白名單上的 unit：" + unit}
	}
	manual := fmt.Sprintf("journalctl --user -u %s -n %d --no-pager", unit, lines)
	if !systemdAvailable() {
		return map[string]any{"ok": false, "manual": manual,
			"error": "碰不到 systemd，journal 讀不到。到終端機跑："}
	}
	rc, out := sysctl(20*time.Second, "journalctl", "--user", "-u", unit,
		"-n", fmt.Sprint(lines), "--no-pager")
	if rc != 0 {
		return map[string]any{"ok": false, "error": out}
	}
	return map[string]any{"ok": true, "text": out}
}
