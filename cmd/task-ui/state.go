package main

// 資料快照層：單況、token、無頭憑證、SKILL.md 提示、log 清單、整頁狀態。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"taskwire/internal/config"
)

var (
	logFileRe = regexp.MustCompile(`^(dispatch\.log|run-[0-9]{8}-[0-9]{6}-[0-9]+\.log)$`)
	skillMark = "## 判斷提示"
)

type cache struct {
	mu      sync.Mutex
	at      time.Time
	payload map[string]any
}

func (c *cache) get(ttl time.Duration) (map[string]any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.payload != nil && time.Since(c.at) < ttl {
		return c.payload, true
	}
	return nil, false
}

func (c *cache) put(payload map[string]any) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at, c.payload = time.Now(), payload
	return payload
}

var issuesCache, tokenCache cache

// issuesSnapshot：單況（唯讀）。快取 30 秒——這一頁會被反覆重新整理，
// 每次都打一輪 GitLab API 既慢又浪費配額。
// 失敗一律回 ok=false 帶原始錯誤，絕不回空清單。
func issuesSnapshot(force bool) map[string]any {
	if !force {
		if p, ok := issuesCache.get(30 * time.Second); ok {
			return p
		}
	}
	repo := config.Effective()["TASK_REPO"]
	rc, out := run(45*time.Second, nil,
		"glab", "issue", "list", "-R", repo, "--output", "json", "-P", "100")
	if rc != 0 {
		return issuesCache.put(map[string]any{
			"ok":    false,
			"error": "glab 查詢失敗（這不是「沒有單」，是查詢根本沒成功）：\n" + out,
		})
	}
	var raw []map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		snip := out
		if len(snip) > 2000 {
			snip = snip[:2000]
		}
		return issuesCache.put(map[string]any{"ok": false, "error": "glab 回的不是 JSON：\n" + snip})
	}
	flow := []string{"todo", "doing", "blocked", "done"}
	buckets := map[string][]map[string]any{"inbox": {}}
	for _, name := range flow {
		buckets[name] = []map[string]any{}
	}
	for _, item := range raw {
		var labels []string
		if arr, ok := item["labels"].([]any); ok {
			for _, l := range arr {
				if s, ok := l.(string); ok {
					labels = append(labels, s)
				}
			}
		}
		row := map[string]any{
			"iid":        item["iid"],
			"title":      item["title"],
			"url":        item["web_url"],
			"labels":     labels,
			"updated_at": item["updated_at"],
		}
		placed := false
		for _, name := range flow {
			for _, l := range labels {
				if l == name {
					buckets[name] = append(buckets[name], row)
					placed = true
					break
				}
			}
		}
		if !placed {
			buckets["inbox"] = append(buckets["inbox"], row)
		}
	}
	return issuesCache.put(map[string]any{
		"ok": true, "buckets": buckets, "total": len(raw), "repo": repo,
		"fetched_at": time.Now().Format("15:04:05"),
	})
}

// tokenInfo：glab token 到期資訊，快取 6 小時——它一天不會變兩次，
// 別每次刷新頁面都打 API。
func tokenInfo() map[string]any {
	if p, ok := tokenCache.get(6 * time.Hour); ok {
		return p
	}
	host := strings.SplitN(config.Effective()["TASK_REPO"], "/", 2)[0]
	rc, out := run(20*time.Second, nil,
		"glab", "api", "personal_access_tokens/self", "--hostname", host)
	if rc != 0 {
		return tokenCache.put(map[string]any{
			"ok": false, "error": "查不到 token 資訊（可能缺 Personal Access Token: Read 權限）",
		})
	}
	var data struct {
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return tokenCache.put(map[string]any{"ok": false, "error": "token 回應解析失敗"})
	}
	payload := map[string]any{"ok": true, "expires_at": data.ExpiresAt, "days": nil}
	if data.ExpiresAt != "" {
		exp, err := time.ParseInLocation("2006-01-02", data.ExpiresAt, time.Local)
		if err != nil {
			return tokenCache.put(map[string]any{"ok": false, "error": "token 回應解析失敗"})
		}
		payload["days"] = int(time.Until(exp).Hours() / 24)
	}
	return tokenCache.put(payload)
}

func findExpiresAt(node any) (float64, bool) {
	switch v := node.(type) {
	case map[string]any:
		if raw, ok := v["expiresAt"]; ok {
			if f, ok := raw.(float64); ok {
				return f, true
			}
		}
		for _, val := range v {
			if f, ok := findExpiresAt(val); ok {
				return f, true
			}
		}
	case []any:
		for _, item := range v {
			if f, ok := findExpiresAt(item); ok {
				return f, true
			}
		}
	}
	return 0, false
}

// credsState：無頭憑證健康。與 doctor／dispatch 預檢同判準：檔案不存在或落後
// 48h 未刷新才算異常，accessToken 短期過期是常態（refresh 會救），不拿它誤報。
func credsState() map[string]any {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".claude", ".credentials.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{"ok": false, "label": "找不到憑證檔", "hint": "終端機跑 claude /login"}
	}
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return map[string]any{"ok": false, "label": "讀不出憑證檔", "hint": "格式可能改了，跑 task doctor 看細節"}
	}
	expMs, found := findExpiresAt(parsed)
	if !found {
		return map[string]any{"ok": true, "label": "檔案在", "hint": "讀不出到期欄位，靠事後判定"}
	}
	age := time.Since(time.UnixMilli(int64(expMs)))
	if age > 48*time.Hour {
		return map[string]any{"ok": false,
			"label": fmt.Sprintf("%d 天沒刷新", int(age.Hours()/24)),
			"hint":  "可能失效——終端機跑 claude /login"}
	}
	return map[string]any{"ok": true, "label": "正常", "hint": "refresh 活著（48 小時內有成功刷新）"}
}

func skillPath() string {
	return filepath.Join(repoDir, "skill", "SKILL.md")
}

// readSkillHints 讀 SKILL.md 的判斷提示區塊。設計拍板一號指定它是調整模型行為的
// 唯一手段（不准加機械閘門），所以它算旋鈕，該在控制台上轉得動。
func readSkillHints() map[string]any {
	path := skillPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{"ok": false, "error": fmt.Sprintf("讀不到 %s：%v", path, err)}
	}
	text := string(data)
	idx := strings.Index(text, skillMark)
	if idx < 0 {
		return map[string]any{"ok": false, "error": fmt.Sprintf("SKILL.md 裡找不到「%s」段落", skillMark)}
	}
	headEnd := idx + strings.Index(text[idx:], "\n")
	return map[string]any{
		"ok": true, "heading": text[idx:headEnd],
		"body": strings.TrimSpace(text[headEnd+1:]), "path": path,
	}
}

func writeSkillHints(body string) map[string]any {
	cur := readSkillHints()
	if cur["ok"] != true {
		return cur
	}
	path := cur["path"].(string)
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	text := string(data)
	idx := strings.Index(text, skillMark)
	headEnd := idx + strings.Index(text[idx:], "\n")
	newText := text[:headEnd+1] + "\n" + strings.TrimSpace(body) + "\n"
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(newText), 0o644); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	if err := os.Rename(tmp, path); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true}
}

func listRunLogs() []string {
	entries, err := os.ReadDir(config.StateDir)
	if err != nil {
		return []string{}
	}
	names := []string{}
	for _, e := range entries {
		if logFileRe.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	if len(names) > 40 {
		names = names[:40]
	}
	return names
}

// readLog 只讀 StateDir 底下、名字符合白名單樣式的檔。
// 路徑不接受任何目錄成分——這是本機服務，但路徑穿越沒有便宜可佔。
func readLog(name string, lines int) map[string]any {
	if !logFileRe.MatchString(name) {
		return map[string]any{"ok": false, "error": "不是可讀的 log 檔名：" + name}
	}
	data, err := os.ReadFile(filepath.Join(config.StateDir, name))
	if err != nil {
		return map[string]any{"ok": false, "error": fmt.Sprintf("讀不到 %s：%v", name, err)}
	}
	all := strings.SplitAfter(string(data), "\n")
	if n := len(all); n > 0 && all[n-1] == "" {
		all = all[:n-1]
	}
	tail := all
	if len(all) > lines {
		tail = all[len(all)-lines:]
	}
	return map[string]any{
		"ok": true, "name": name, "text": strings.Join(tail, ""),
		"truncated": len(all) > lines, "total_lines": len(all),
	}
}

func baseState() map[string]any {
	onDisk := config.ReadFile()
	eff := config.Effective()
	settings := []map[string]any{}
	for _, item := range config.Settings {
		settings = append(settings, map[string]any{
			"key": item.Key, "label": item.Label, "default": item.Default,
			"type": item.Type, "help": item.Help, "restart": item.Restart,
			"value":     onDisk[item.Key],
			"effective": eff[item.Key],
			"source":    config.SourceOf(item.Key),
		})
	}
	secretsState := []map[string]any{}
	for _, item := range config.Secrets {
		val := config.ReadSecret(item.Name)
		path := config.SecretPath(item.Name)
		var mode any
		if st, err := os.Stat(path); err == nil {
			// "0o600" 的字串格式沿用 Python oct()——前端比對的就是這個字面。
			mode = fmt.Sprintf("0o%o", st.Mode().Perm())
		}
		hint := ""
		if len(val) > 12 {
			hint = val[:4] + "…" + val[len(val)-4:]
		} else if val != "" {
			hint = "已設定"
		}
		secretsState = append(secretsState, map[string]any{
			"name": item.Name, "label": item.Label, "help": item.Help,
			"generate": nilIfEmpty(item.Generate),
			"present":  val != "", "length": len(val), "hint": hint,
			"path": path, "mode": mode,
		})
	}
	threads := map[string]any{}
	if data, err := os.ReadFile(filepath.Join(config.StateDir, "discord-threads.json")); err == nil {
		_ = json.Unmarshal(data, &threads)
	}
	unitRows := []map[string]any{}
	for _, def := range units {
		unitRows = append(unitRows, unitState(def))
	}
	return map[string]any{
		"ok":               true,
		"settings":         settings,
		"secrets":          secretsState,
		"units":            unitRows,
		"systemd":          systemdAvailable(),
		"threads":          threads,
		"run_logs":         listRunLogs(),
		"config_path":      config.ConfigPath,
		"state_dir":        config.StateDir,
		"repo_dir":         repoDir,
		"webhook_url_hint": ":" + eff["TASK_WEBHOOK_PORT"] + "/hook",
		"creds":            credsState(),
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
