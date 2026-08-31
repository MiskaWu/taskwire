// Package config —— 設定的單一真相來源（Go 端）。
//
// bash 端是 bin/taskwire-config.sh，兩邊讀同一個 ~/.config/taskwire/config.env，
// 優先序也一致：**環境變數 > config.env > 這裡的 default**。
// （2026-08-31 起 Python 端 taskwire_config.py 由這個套件取代，語意原樣搬過來。）
//
// 這個檔案同時是「旋鈕的目錄」：Settings 一列就是一顆旋鈕，控制台的表單
// 直接由它長出來。以後要加旋鈕只加一筆，不必再去改網頁——旋鈕的定義散在
// 程式與頁面兩處，正是設定會慢慢對不上的起點。
//
// Restart 欄位記的是「改完這顆之後誰要重啟才生效」，控制台據此在存檔後
// 提示（或直接代跑）。空陣列代表下次呼叫自然生效，什麼都不用做。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Setting 的 json tag 對齊舊版 Python dict 的鍵名——前端照這些名字取值。
type Setting struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Default string   `json:"default"`
	Type    string   `json:"type"`
	Help    string   `json:"help"`
	Restart []string `json:"restart"`
}

type Secret struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Help     string `json:"help"`
	Generate string `json:"generate"` // "hex32" 或空字串
}

var (
	ConfigPath = envOr("TASKWIRE_CONFIG", expand("~/.config/taskwire/config.env"))
	ConfigDir  = filepath.Dir(ConfigPath)
	StateDir   = envOr("TASKWIRE_STATE", expand("~/.local/state/taskwire"))
)

// 密鑰不進 config.env：它們是 0600 的獨立檔，權限與內容都跟一般旋鈕不同性質。
var Secrets = []Secret{
	{
		Name:  "webhook-secret",
		Label: "Webhook 密鑰",
		Help: "GitLab 專案 Webhook 設定裡的 Signing token 要填這個值。" +
			"改了之後 GitLab 那端沒跟著改，門鈴就會聾掉（403）。",
		Generate: "hex32",
	},
	{
		Name:  "discord-webhook",
		Label: "Discord Webhook 網址",
		Help: "推播的落點，必須是**論壇頻道**的 webhook——一般文字頻道開不了討論串" +
			"（Discord 回 220003），通知會退化成不分串。留空則完全不推 Discord。",
		Generate: "",
	},
}

var Settings = []Setting{
	{
		Key:     "TASK_REPO",
		Label:   "Backlog 倉庫路徑",
		Default: "gitlab.dev.baasgames.com/hydrogen/roles/engineer/miskawu/docs/backlogs",
		Type:    "text",
		Help: "**必須帶主機名**。只寫 <group>/<repo> 的話 glab 會靜默打 gitlab.com，" +
			"症狀是 404／401 而不是「找不到指令」。",
		Restart: []string{},
	},
	{
		Key:     "TASK_DISPATCH_TIMEOUT",
		Label:   "無頭取件逾時（秒）",
		Default: "3600",
		Type:    "number",
		Help: "單次無頭會話的上限，防「做錯方向做一整夜」。逾時會被 timeout 殺掉，" +
			"機械兜底接手把單子補成 blocked。",
		Restart: []string{},
	},
	{
		Key:     "TASK_WEBHOOK_PORT",
		Label:   "門鈴監聽埠",
		Default: "9587",
		Type:    "number",
		Help:    "改這個要同步改 GitLab 那端的 Webhook URL 與 Windows 防火牆入站規則。",
		Restart: []string{"task-webhook.service"},
	},
	{
		Key:     "TASK_UI_PORT",
		Label:   "控制台埠",
		Default: "9588",
		Type:    "number",
		Help: "這個控制台自己的埠，只綁 127.0.0.1。改完要重啟控制台，" +
			"重啟後這一頁的網址也跟著換。",
		Restart: []string{"task-ui.service"},
	},
	{
		Key:     "TASK_UI_BIND",
		Label:   "控制台綁定位址",
		Default: "127.0.0.1",
		Type:    "text",
		Help: "預設只綁本機。頁面上有 webhook 密鑰與服務控制權，而且是 http 明文，" +
			"**不建議改成 0.0.0.0**——真要區網存取請先想清楚誰在同一個網段。",
		Restart: []string{"task-ui.service"},
	},
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func expand(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~/"))
}

func Defaults() map[string]string {
	out := map[string]string{}
	for _, s := range Settings {
		out[s.Key] = s.Default
	}
	return out
}

var lineRe = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*?)\s*$`)

func unquote(v string) string {
	if len(v) >= 2 && v[0] == v[len(v)-1] && (v[0] == '"' || v[0] == '\'') {
		return v[1 : len(v)-1]
	}
	return v
}

// ReadFile 只讀 config.env 的內容，不套環境變數也不套預設值。
// 控制台編輯表單要顯示的是「檔案裡實際寫了什麼」，不是「跑起來會拿到什麼」。
func ReadFile() map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if m := lineRe.FindStringSubmatch(line); m != nil {
			out[m[1]] = unquote(m[2])
		}
	}
	return out
}

// Effective 是實際生效的值，套用完整優先序：環境變數 > config.env > default。
// 跟 taskwire-config.sh 的行為必須一致——兩邊分岔的話，控制台顯示的
// 就不是腳本真正在用的值，那比沒有控制台更糟。
func Effective() map[string]string {
	onDisk := ReadFile()
	out := map[string]string{}
	for key, def := range Defaults() {
		if v := os.Getenv(key); v != "" {
			out[key] = v
		} else if v := onDisk[key]; v != "" {
			out[key] = v
		} else {
			out[key] = def
		}
	}
	for key, val := range onDisk {
		if _, seen := out[key]; !seen {
			out[key] = val
		}
	}
	return out
}

// SourceOf 說明這個值現在是從哪裡來的——控制台要標示出來，
// 否則你在網頁上改了卻不生效（因為被環境變數蓋住）會找不到原因。
func SourceOf(key string) string {
	if os.Getenv(key) != "" {
		return "env"
	}
	if _, ok := ReadFile()[key]; ok {
		return "file"
	}
	return "default"
}

var boldRe = regexp.MustCompile(`\*\*(.+?)\*\*`)

// WriteFile 整檔重寫，帶說明註解。認識的旋鈕照 Settings 的順序排並附註解，
// 不認識的 key 原樣保留在檔尾——人手加的東西不該被控制台默默吃掉。
func WriteFile(values map[string]string) error {
	if err := os.MkdirAll(ConfigDir, 0o755); err != nil {
		return err
	}
	known := map[string]bool{}
	lines := []string{
		"# taskwire 設定。控制台（task-ui）讀寫這個檔，手改也可以。",
		"# 優先序：環境變數 > 這個檔 > 腳本內建預設。",
		"# 純 KEY=value，不做 shell 展開——這是設定檔，不是腳本。",
		"",
	}
	for _, setting := range Settings {
		known[setting.Key] = true
		val, ok := values[setting.Key]
		if !ok {
			continue
		}
		help := boldRe.ReplaceAllString(setting.Help, "$1")
		lines = append(lines,
			fmt.Sprintf("# %s：%s", setting.Label, help),
			fmt.Sprintf("%s=%s", setting.Key, val),
			"")
	}
	var extra []string
	for key := range values {
		if !known[key] {
			extra = append(extra, key)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		lines = append(lines, "# —— 以下不是控制台認識的旋鈕，原樣保留 ——")
		for _, key := range extra {
			lines = append(lines, fmt.Sprintf("%s=%s", key, values[key]))
		}
		lines = append(lines, "")
	}
	tmp := ConfigPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, ConfigPath)
}

func SecretPath(name string) string {
	return filepath.Join(ConfigDir, name)
}

func ReadSecret(name string) string {
	data, err := os.ReadFile(SecretPath(name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// WriteSecret 0600 寫入。先建檔設權限再寫內容，避免有一瞬間是 0644 的密鑰檔。
func WriteSecret(name, value string) error {
	if err := os.MkdirAll(ConfigDir, 0o755); err != nil {
		return err
	}
	path := SecretPath(name)
	tmp := path + ".tmp"
	fh, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := fh.WriteString(strings.TrimSpace(value) + "\n"); err != nil {
		fh.Close()
		return err
	}
	if err := fh.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
