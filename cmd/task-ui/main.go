// task-ui —— taskwire 的本機控制台（2026-08-28 拍板；2026-08-31 改寫成 Go，
// 對齊 canopy 的技術棧，前端 build 產物以 go:embed 嵌進執行檔）。
//
// **範圍是拍板過的**：設定、觀測，加上唯讀的單況。刻意不含「拉 todo」與「關單」——
// 不是因為技術上擋不住（無頭 claude 手上有 Bash，它要關單根本不需要經過這裡），
// 而是因為那道防線是協定性的：`task help`、SKILL.md、這個控制台，每一處都說
// 「這兩件不歸代理」，模型才不會誤讀成授權。控制台上擺一顆關單按鈕，等於從
// 內部把那道一致性拆掉。要加就先改設計。
//
// **宿主機動作會自動降級。** 開關服務、調排程、讀 journal 都要碰得到 user systemd
// 的 D-Bus。碰得到就直接執行；碰不到（典型情況：跑在沒 mount /run/user/<uid>/bus
// 的 podman 容器裡）就把該跑的指令原樣回給頁面，由使用者貼到終端機。同一份程式碼
// 兩種部署都能用，容器模式只是某些動作從「幫你做」變成「給你指令」。
//
// **查詢失敗必須長得不像空清單**（設計拍板三號）在這裡的落實：每個 API 一律回
// {"ok": bool, ...}，失敗時帶 error 字串。前端在 ok=false 時顯示錯誤區塊，
// 絕不渲染成空表格——token 過期那天，單況面板要說「連不上」，不能說「沒有單」。
//
// 只綁 127.0.0.1。仍然驗 Host 標頭（擋 DNS rebinding：外部網頁把某網域解析到
// 127.0.0.1 之後就能對本服務發請求），寫入動作另外要求一個自訂標頭——
// HTML 表單發不出自訂標頭，跨站的 fetch 帶自訂標頭會先觸發 preflight 而被擋下。
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"taskwire/internal/config"
	"taskwire/internal/webui"
)

const csrfHeader = "X-Taskwire-UI"

var (
	repoDir      = resolveRepoDir()
	distFS       = webui.Dist()
	onCalendarRe = regexp.MustCompile(`^[A-Za-z0-9*,\-./: ]+$`)
	configKeyRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// resolveRepoDir：SKILL.md 與 bin/task-dispatch 還是從 repo 讀（它們不適合嵌進
// 執行檔——SKILL.md 是控制台要寫回的旋鈕）。預設拿執行檔位置的上一層，
// 對應「binary 放在 repo 的 bin/」的部署慣例；擺去別處就設 TASKWIRE_REPO_DIR。
func resolveRepoDir() string {
	if v := os.Getenv("TASKWIRE_REPO_DIR"); v != "" {
		return v
	}
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(filepath.Dir(exe))
}

var mimeTypes = map[string]string{
	".html": "text/html; charset=utf-8", ".js": "application/javascript; charset=utf-8",
	".css": "text/css; charset=utf-8", ".svg": "image/svg+xml", ".ico": "image/x-icon",
	".map": "application/json", ".woff2": "font/woff2", ".png": "image/png",
}

func send(w http.ResponseWriter, code int, ctype string, body []byte) {
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write(body)
}

func sendJSON(w http.ResponseWriter, code int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte(`{"ok":false,"error":"JSON 編碼失敗"}`)
		code = 500
	}
	send(w, code, "application/json; charset=utf-8", body)
}

// hostOK 擋 DNS rebinding：外部網站把自己的網域指到 127.0.0.1 之後，
// 使用者的瀏覽器就會拿著那個網域對本服務發請求。只認回環的主機名。
//
// nip.io 的固定回環名（127-0-0-1.nip.io）也放行——WSL 桌面板從對話裡開連結
// 走的是這個形式，擋掉的話點進來只會看到 403。放行它不等於放棄防線：
// 那個名字永遠解析到 127.0.0.1，不是攻擊者控制的網域（rebinding 靠的正是
// 攻擊者能改自己網域的解析）。真正擋住惡意頁面的是同源政策——本服務不回
// 任何 CORS 標頭，跨來源的讀取拿不到回應，寫入所需的自訂標頭會先觸發
// preflight 而失敗。Host 檢查是外面那一層。
func hostOK(r *http.Request) bool {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	switch host {
	case "127.0.0.1", "localhost", "::1", "", "127-0-0-1.nip.io", "localhost.nip.io":
		return true
	}
	return false
}

// writeOK 是寫入動作的額外門檻。HTML 表單送不出自訂標頭，跨站 fetch 帶自訂標頭
// 會先觸發 preflight 而被同源政策擋下——所以這一個標頭就足以擋掉
// 「使用者剛好開著某個惡意分頁」這條路。
func writeOK(r *http.Request) bool {
	return r.Header.Get(csrfHeader) == "1"
}

func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func handleGET(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/state":
		sendJSON(w, 200, baseState())
	case "/api/issues":
		sendJSON(w, 200, issuesSnapshot(r.URL.Query().Get("force") == "1"))
	case "/api/doctor":
		rc, out := run(90*time.Second, nil, "task", "doctor")
		errMsg := ""
		if rc != 0 {
			errMsg = "task doctor 回報有問題（內容在下面）"
		}
		sendJSON(w, 200, map[string]any{"ok": rc == 0, "text": out, "error": errMsg})
	case "/api/healthz":
		port := config.Effective()["TASK_WEBHOOK_PORT"]
		client := http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
		text, ok := "（沒有回應）", false
		if err == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			text = string(body)
			ok = resp.StatusCode == 200 && strings.TrimSpace(text) == "ok"
		}
		errMsg := ""
		if err != nil {
			errMsg = "門鈴 :" + port + " 沒有回應"
		}
		sendJSON(w, 200, map[string]any{"ok": ok, "text": text, "error": errMsg})
	case "/api/tokeninfo":
		sendJSON(w, 200, tokenInfo())
	case "/api/log/stream":
		handleLogStream(w, r)
	case "/api/log":
		name := valOr(r.URL.Query().Get("name"), "dispatch.log")
		sendJSON(w, 200, readLog(name, queryInt(r, "lines", 300)))
	case "/api/journal":
		unit := valOr(r.URL.Query().Get("unit"), "task-webhook.service")
		sendJSON(w, 200, journal(unit, queryInt(r, "lines", 60)))
	case "/api/skill":
		sendJSON(w, 200, readSkillHints())
	case "/api/secret":
		// 明文密鑰要顯式索取，不隨 /api/state 一起送——頁面預設是遮蔽的。
		name := r.URL.Query().Get("name")
		if !secretKnown(name) {
			sendJSON(w, 400, map[string]any{"ok": false, "error": "沒有這個密鑰：" + name})
			return
		}
		sendJSON(w, 200, map[string]any{"ok": true, "name": name, "value": config.ReadSecret(name)})
	default:
		serveStatic(w, r.URL.Path)
	}
}

func secretKnown(name string) bool {
	for _, s := range config.Secrets {
		if s.Name == name {
			return true
		}
	}
	return false
}

func handlePOST(w http.ResponseWriter, r *http.Request) {
	if !writeOK(r) {
		sendJSON(w, 403, map[string]any{"ok": false,
			"error": fmt.Sprintf("缺少 %s 標頭，拒絕寫入。", csrfHeader)})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		sendJSON(w, 400, map[string]any{"ok": false, "error": "讀不到請求內容"})
		return
	}
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		sendJSON(w, 400, map[string]any{"ok": false, "error": "請求不是合法 JSON"})
		return
	}
	dispatchPOST(w, r.URL.Path, body)
}

func str(body map[string]any, key string) string {
	if v, ok := body[key].(string); ok {
		return v
	}
	return ""
}

func dispatchPOST(w http.ResponseWriter, p string, body map[string]any) {
	switch p {
	case "/api/config":
		values, _ := body["values"].(map[string]any)
		merged := config.ReadFile()
		for key, val := range values {
			if !configKeyRe.MatchString(key) {
				sendJSON(w, 400, map[string]any{"ok": false, "error": "不合法的設定名：" + key})
				return
			}
			text := strings.TrimSpace(fmt.Sprint(val))
			if strings.Contains(text, "\n") {
				sendJSON(w, 400, map[string]any{"ok": false, "error": key + " 不能含換行"})
				return
			}
			if text != "" {
				merged[key] = text
			} else {
				delete(merged, key) // 清空＝回到內建預設
			}
		}
		if err := config.WriteFile(merged); err != nil {
			sendJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		shadowed := []string{}
		needSet := map[string]bool{}
		for key := range values {
			if os.Getenv(key) != "" {
				shadowed = append(shadowed, key)
			}
			for _, s := range config.Settings {
				if s.Key == key {
					for _, u := range s.Restart {
						needSet[u] = true
					}
				}
			}
		}
		need := []string{}
		for u := range needSet {
			need = append(need, u)
		}
		sort.Strings(need)
		sort.Strings(shadowed)
		sendJSON(w, 200, map[string]any{"ok": true, "restart_needed": need, "shadowed": shadowed})

	case "/api/secret":
		name := str(body, "name")
		if !secretKnown(name) {
			sendJSON(w, 400, map[string]any{"ok": false, "error": "沒有這個密鑰：" + name})
			return
		}
		if err := config.WriteSecret(name, fmt.Sprint(body["value"])); err != nil {
			sendJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		restart := []string{}
		if name == "webhook-secret" {
			restart = []string{"task-webhook.service"}
		}
		sendJSON(w, 200, map[string]any{"ok": true, "restart_needed": restart})

	case "/api/secret/generate":
		if str(body, "name") != "webhook-secret" {
			sendJSON(w, 400, map[string]any{"ok": false, "error": "只有 webhook-secret 能自動產生"})
			return
		}
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			sendJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		value := hex.EncodeToString(buf)
		if err := config.WriteSecret("webhook-secret", value); err != nil {
			sendJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		sendJSON(w, 200, map[string]any{"ok": true, "value": value,
			"restart_needed": []string{"task-webhook.service"}})

	case "/api/service":
		unit, action := str(body, "unit"), str(body, "action")
		if unitByName(unit) == nil {
			sendJSON(w, 400, map[string]any{"ok": false, "error": "不在白名單上的 unit：" + unit})
			return
		}
		switch action {
		case "start", "stop", "restart", "enable", "disable":
		default:
			sendJSON(w, 400, map[string]any{"ok": false, "error": "不允許的動作：" + action})
			return
		}
		cmd := []string{"systemctl", "--user", action}
		if action == "enable" || action == "disable" {
			cmd = append(cmd, "--now")
		}
		cmd = append(cmd, unit)
		sendJSON(w, 200, hostAction(cmd, action+" "+unit))

	case "/api/schedule":
		unit, when := str(body, "unit"), strings.TrimSpace(str(body, "oncalendar"))
		if unitByName(unit) == nil || !strings.HasSuffix(unit, ".timer") {
			sendJSON(w, 400, map[string]any{"ok": false, "error": "不是可調排程的 timer：" + unit})
			return
		}
		if when == "" || !onCalendarRe.MatchString(when) {
			sendJSON(w, 400, map[string]any{"ok": false,
				"error": "OnCalendar 格式看起來不對。例：hourly、daily、*-*-* 09:00、Mon..Fri *-*-* 08:30"})
			return
		}
		rc, out := run(10*time.Second, nil, "systemd-analyze", "calendar", when)
		if rc != 0 {
			sendJSON(w, 400, map[string]any{"ok": false,
				"error": "systemd 不接受這個排程式：\n" + out})
			return
		}
		res := writeSchedule(unit, when)
		res["parsed"] = out
		sendJSON(w, 200, res)

	case "/api/schedule/reset":
		unit := str(body, "unit")
		if unitByName(unit) == nil {
			sendJSON(w, 400, map[string]any{"ok": false, "error": "不在白名單上的 unit：" + unit})
			return
		}
		dp := dropinPath(unit)
		if _, err := os.Stat(dp); err == nil {
			_ = os.Remove(dp)
			_ = os.Remove(filepath.Dir(dp)) // 目錄非空會失敗，跟 Python 版的 rmdir 同義
		}
		sendJSON(w, 200, hostAction([]string{"systemctl", "--user", "daemon-reload"},
			"回到 repo 的出廠排程"))

	case "/api/notify-test":
		kind := valOr(strings.TrimSpace(str(body, "kind")), "異常")
		msg := valOr(strings.TrimSpace(str(body, "message")), "控制台測試訊息")
		// 一律加 -d：測試訊息不該掛 GitLab 健康卡，那張卡留給真異常。
		rc, out := run(30*time.Second, nil, "task-notify", "-d", "-t", kind, msg)
		errMsg := ""
		if rc != 0 {
			errMsg = "task-notify 失敗（輸出在下面）"
		}
		sendJSON(w, 200, map[string]any{"ok": rc == 0, "text": out, "error": errMsg})

	case "/api/threads/reset":
		threadsPath := filepath.Join(config.StateDir, "discord-threads.json")
		data := map[string]any{}
		if raw, err := os.ReadFile(threadsPath); err == nil {
			_ = json.Unmarshal(raw, &data)
		}
		if key := str(body, "key"); key != "" {
			delete(data, key)
		} else {
			data = map[string]any{}
		}
		out, _ := json.Marshal(data)
		if err := os.WriteFile(threadsPath, out, 0o644); err != nil {
			sendJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		sendJSON(w, 200, map[string]any{"ok": true, "threads": data})

	case "/api/skill":
		sendJSON(w, 200, writeSkillHints(str(body, "body")))

	case "/api/dispatch/run":
		// 手動叫醒 dispatch。它自己有 flock，忙碌中會直接跳過，按幾次都安全。
		// 注意這不是「指派工作」——dispatch 用 task next 重讀真實狀態決定做什麼，
		// 這裡只是提早按一次門鈴。
		script := filepath.Join(repoDir, "bin", "task-dispatch")
		_ = os.MkdirAll(config.StateDir, 0o755)
		logf, err := os.OpenFile(filepath.Join(config.StateDir, "dispatch.log"),
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			sendJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		cmd := exec.Command(script)
		cmd.Stdout, cmd.Stderr = logf, logf
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		err = cmd.Start()
		logf.Close()
		if err != nil {
			sendJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		go func() { _ = cmd.Wait() }()
		sendJSON(w, 200, map[string]any{"ok": true, "text": "已叫醒 dispatch，進度看 dispatch.log。"})

	default:
		sendJSON(w, 404, map[string]any{"ok": false, "error": "沒有這個端點：" + p})
	}
}

// serveStatic 伺服 go:embed 進來的前端 build 產物（vite-plugin-singlefile，
// 幾乎只有 index.html 一個檔）。非資產路徑一律回 index.html（SPA 後援；
// hash routing 其實用不到，保險）。
func serveStatic(w http.ResponseWriter, reqPath string) {
	rel := "index.html"
	if reqPath != "/" && reqPath != "/index.html" {
		rel = strings.TrimPrefix(path.Clean(reqPath), "/")
	}
	if !fs.ValidPath(rel) {
		send(w, 404, "text/plain; charset=utf-8", []byte("not found\n"))
		return
	}
	body, err := fs.ReadFile(distFS, rel)
	if err != nil {
		if strings.Contains(path.Base(rel), ".") {
			send(w, 404, "text/plain; charset=utf-8", []byte("not found\n"))
			return
		}
		rel = "index.html"
		if body, err = fs.ReadFile(distFS, rel); err != nil {
			send(w, 500, "text/plain; charset=utf-8",
				[]byte("嵌入的前端產物讀不到——build 時要先 cd ui && npm run build 再 go build。\n"))
			return
		}
	}
	ctype := mimeTypes[path.Ext(rel)]
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	send(w, 200, ctype, body)
}

// handleLogStream 用 SSE 跟隨 log：從檔尾開始，長出來的行即時推給頁面。
// 每秒探一次、15 秒一個心跳註解防中間層斷線；檔案被截斷（輪替）就回頭從 0 讀。
// EventSource 帶不了自訂標頭，所以這個 GET 端點只驗 Host——它是唯讀的，設計上允許。
func handleLogStream(w http.ResponseWriter, r *http.Request) {
	name := valOr(r.URL.Query().Get("name"), "dispatch.log")
	if !logFileRe.MatchString(name) {
		sendJSON(w, 400, map[string]any{"ok": false, "error": "不是可讀的 log 檔名：" + name})
		return
	}
	filePath := filepath.Join(config.StateDir, name)
	fh, err := os.Open(filePath)
	if err != nil {
		sendJSON(w, 404, map[string]any{"ok": false, "error": name + " 不存在"})
		return
	}
	defer fh.Close()
	flusher, ok := w.(http.Flusher)
	if !ok {
		sendJSON(w, 500, map[string]any{"ok": false, "error": "這個連線不支援串流"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	offset, _ := fh.Seek(0, io.SeekEnd)
	pending := ""
	buf := make([]byte, 64*1024)
	ctx := r.Context()
	for idle := 0; idle < 3600; { // 一小時沒動靜就收線，頁面會自動重連
		n, _ := fh.Read(buf)
		if n > 0 {
			offset += int64(n)
			pending += string(buf[:n])
			emitted := false
			for {
				i := strings.IndexByte(pending, '\n')
				if i < 0 {
					break
				}
				payload, _ := json.Marshal(pending[:i])
				if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
					return
				}
				pending = pending[i+1:]
				emitted = true
			}
			if emitted {
				flusher.Flush()
				idle = 0
				continue
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
		idle++
		if idle%15 == 0 {
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
		st, err := os.Stat(filePath)
		if err != nil {
			return
		}
		if st.Size() < offset {
			if _, err := fh.Seek(0, io.SeekStart); err != nil {
				return
			}
			offset, pending = 0, ""
		}
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	if !hostOK(r) {
		send(w, 403, "text/plain; charset=utf-8", []byte("forbidden\n"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		handleGET(w, r)
	case http.MethodPost:
		handlePOST(w, r)
	default:
		send(w, 405, "text/plain; charset=utf-8", []byte("method not allowed\n"))
	}
}

func main() {
	// task／task-notify 就住在 repo 的 bin/，把它補進 PATH——systemd user service
	// 的環境常常沒有 ~/.local/bin，「跑 task doctor」那顆按鈕會直接找不到指令。
	_ = os.Setenv("PATH", filepath.Join(repoDir, "bin")+":"+os.Getenv("PATH"))
	eff := config.Effective()
	bind := eff["TASK_UI_BIND"]
	port := eff["TASK_UI_PORT"]
	// 在容器裡綁 0.0.0.0 是正常的——podman 的 PublishPort 已經把它限制在
	// 宿主機的 127.0.0.1，容器內再綁回環反而連不進來。只有直跑時才該警告。
	inContainer := fileExists("/run/.containerenv") || fileExists("/.dockerenv")
	if bind != "127.0.0.1" && bind != "localhost" && bind != "::1" && !inContainer {
		fmt.Printf("⚠ 綁在 %s：這一頁有 webhook 密鑰與服務控制權，而且是 http 明文。\n", bind)
	}
	mode := "碰不到，宿主機動作會降級成給指令"
	if systemdAvailable() {
		mode = "可用"
	}
	fmt.Printf("task-ui 監聽 http://%s:%s/（systemd %s）\n", bind, port, mode)
	server := &http.Server{
		Addr:    net.JoinHostPort(bind, port),
		Handler: http.HandlerFunc(handler),
	}
	if err := server.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
