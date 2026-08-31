// task-webhookd —— 接 GitLab webhook 的門鈴。
//
// 設計原則：**webhook 是門鈴，不是資料來源。**
// 這支服務收到 issue 事件後只做一件事：往門鈴信箱投一張紙條，然後立刻回 200。
// 起 task-dispatch 的職責歸宿主機的 task-doorbell.path（systemd 盯著信箱目錄）。
// 它不解讀 payload 裡的細節——單子的真實狀態永遠由 dispatch 打 API 重新確認。
// 好處：漏接事件不會壞資料（只是慢一拍，可選的對帳 timer 會補），
// 重複事件天然冪等（dispatch 有 flock，忙碌中就跳過）。
//
// 2026-08-28 改造：以前這支程式「同時」是全機唯一迎接網路流量的行程、又握有
// 起 dispatch 的執行權——那個同時就是風險所在。拆開之後它的全部能力只剩按鈴
// （資訊量一個位元），因此可以乾淨容器化（Containerfile.webhook：只掛密鑰唯讀
// ＋信箱可寫，沒有 glab、沒有 systemd、沒有任何憑證）。直跑在宿主機也一樣成立，
// 部署二選一，行為相同。
//
// 2026-08-31 改寫成 Go（使用者拍板，對齊 canopy 的技術棧）：行為與 Python 版
// 一比一，靜態編譯後容器可以縮到 scratch——連 shell 都沒有的映像。
//
// 安全：優先驗 Webhook-Signature 標頭（GitLab 的 signing token——用密鑰對
// payload 做 HMAC-SHA256，密鑰本身不隨請求傳輸，在 http 環境下比明文 token
// 實質安全）；沒有簽章標頭時退回驗 X-Gitlab-Token（切換期的備援）。
// 密鑰共用同一份 ~/.config/taskwire/webhook-secret（0600），驗不過回 403。
//
// 這是本機少數的常駐服務之一，由 systemd user unit（task-webhook.service）
// 帶 Restart=always 看管。/healthz 給健康檢查用。
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	secretFile = expand("~/.config/taskwire/webhook-secret")
	doorbell   = expand("~/.local/state/taskwire/doorbell")
)

func expand(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~/"))
}

func readSecret() string {
	data, err := os.ReadFile(secretFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// candidateKeys：Standard Webhooks 的密鑰可能是原始字串，也可能是 whsec_ 前綴的
// base64。兩種都當候選，驗得過任一種就算通過——GitLab 產生器給哪種格式版本間有出入。
func candidateKeys(secret string) [][]byte {
	keys := [][]byte{[]byte(secret)}
	s := strings.TrimPrefix(secret, "whsec_")
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	if decoded, err := base64.StdEncoding.DecodeString(s); err == nil {
		keys = append(keys, decoded)
	}
	return keys
}

// verifySignature：Standard Webhooks（GitLab signing token 採用的規格）：
// 簽名內容是 "<webhook-id>.<webhook-timestamp>.<body>"，
// HMAC-SHA256 後 base64，放在標頭裡以 "v1," 前綴、可有多組以空白分隔。
// 另保留舊假設（對 body 直接 HMAC 的十六進位）當相容路徑。
func verifySignature(secret string, hdr http.Header, body []byte, sigHeader string) bool {
	msgID := hdr.Get("Webhook-Id")
	ts := hdr.Get("Webhook-Timestamp")
	signed := []byte(msgID + "." + ts + ".")
	signed = append(signed, body...)
	var v1sigs []string
	for _, part := range strings.Fields(sigHeader) {
		if strings.HasPrefix(part, "v1,") {
			v1sigs = append(v1sigs, part[3:])
		}
	}
	for _, key := range candidateKeys(secret) {
		mac := hmac.New(sha256.New, key)
		mac.Write(signed)
		want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		for _, got := range v1sigs {
			if hmac.Equal([]byte(want), []byte(got)) {
				return true
			}
		}
	}
	// 相容：純 hex、直接簽 body 的格式（本機自測與可能的舊版 GitLab）。
	gotHex := sigHeader
	if idx := strings.Index(gotHex, "="); idx >= 0 {
		gotHex = gotHex[idx+1:]
	}
	gotHex = strings.ToLower(strings.TrimSpace(gotHex))
	for _, key := range candidateKeys(secret) {
		mac := hmac.New(sha256.New, key)
		mac.Write(body)
		if hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(gotHex)) {
			return true
		}
	}
	return false
}

func reply(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	io.WriteString(w, body)
}

func handleHook(w http.ResponseWriter, r *http.Request) {
	secret := readSecret()
	if secret == "" {
		log.Print("403 拒絕：本機沒有密鑰檔")
		reply(w, 403, "")
		return
	}
	// 簽章要對 payload 算，所以先讀 body 再驗。
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		reply(w, 400, "")
		return
	}
	sig := r.Header.Get("Webhook-Signature")
	tok := r.Header.Get("X-Gitlab-Token")
	remote := r.RemoteAddr
	switch {
	case sig != "":
		if !verifySignature(secret, r.Header, body, sig) {
			short := sig
			if len(short) > 16 {
				short = short[:16]
			}
			log.Printf("403 拒絕：簽章不符（來源 %s，標頭形如 %s…）", remote, short)
			reply(w, 403, "")
			return
		}
	case tok != "":
		// 切換期備援：舊的明文 token 驗證。
		if !hmac.Equal([]byte(secret), []byte(tok)) {
			log.Printf("403 拒絕：token 不符（來源 %s）", remote)
			reply(w, 403, "")
			return
		}
	default:
		log.Printf("403 拒絕：沒帶任何驗證標頭（來源 %s）", remote)
		reply(w, 403, "")
		return
	}
	var payload struct {
		ObjectKind string `json:"object_kind"`
	}
	_ = json.Unmarshal(body, &payload)
	kind := payload.ObjectKind
	if kind != "issue" && kind != "work_item" && kind != "note" {
		// 這版 GitLab 把 issue 統一叫 work item，事件的 object_kind 兩種都可能出現。
		// note（留言）也當門鈴：使用者補驗收條件的動作就是留言，不響的話
		// 補完得再去碰標籤才會動，彆扭。我們自己的上工／交件留言也會響，
		// 但 dispatch 的 flock 與冪等取件天然消化。其餘事件（push…）收下不動作。
		reply(w, 200, "ignored\n")
		return
	}
	// 投遞門鈴：往信箱放一張紙條就結束，不起任何子行程。宿主機的
	// task-doorbell.path 看到信箱非空會起 task-dispatch，dispatch 開場自己收信。
	// 信箱裡已有未收的紙條就不再投——門鈴的資訊量只有一個位元，多投不加分，
	// 也讓信箱天然不會被連環事件塞滿。
	_ = os.MkdirAll(doorbell, 0o755)
	entries, err := os.ReadDir(doorbell)
	if err != nil {
		entries = nil
	}
	if len(entries) == 0 {
		ring := filepath.Join(doorbell,
			fmt.Sprintf("ring-%d-%d", time.Now().UnixNano(), os.Getpid()))
		_ = os.WriteFile(ring, []byte(kind+"\n"), 0o644)
	}
	log.Print(kind + " 事件 → 已投遞門鈴")
	reply(w, 200, "ok\n")
}

func main() {
	log.SetFlags(0) // journalctl 自帶時間戳，不重複印。
	port := os.Getenv("TASK_WEBHOOK_PORT")
	if port == "" {
		port = "9587"
	}
	if _, err := os.Stat(secretFile); err != nil {
		log.Printf("錯誤：找不到密鑰檔 %s，先跑安裝步驟。", secretFile)
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, 200, "ok\n")
	})
	mux.HandleFunc("POST /hook", handleHook)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		reply(w, 404, "")
	})
	log.Printf("task-webhookd 監聽 :%s（/hook 驗簽投門鈴，/healthz 健康檢查）", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
