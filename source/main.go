// GKM - So Sánh Hóa Đơn: đối chiếu số lượng giữa hóa đơn nhà cung cấp và hàng thực nhận.
// Ứng dụng chạy hoàn toàn offline: một máy chủ cục bộ (127.0.0.1) phục vụ giao diện, mở bằng Edge/Chrome ở chế độ ứng dụng.
// Mạng chỉ dùng khi người dùng bấm "Cập nhật ứng dụng" (đọc update/latest.json trên GitHub).
package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	version = "1.0.0"
	repo    = "bmabongden/gkm-so-sanh-hoa-don"
	port    = 47652
)

//go:embed web
var webFS embed.FS

var lastPing atomic.Int64

func main() {
	if u := os.Getenv("GKM_UPDATE_URL"); u != "" { // chỉ dùng khi kiểm thử
		manifestURL = u
	}
	cleanupOld()
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		// Đã có một bản đang chạy: chỉ mở thêm cửa sổ tới bản đó.
		if c, e := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second); e == nil {
			c.Close()
			openWindow(fmt.Sprintf("http://127.0.0.1:%d/", port))
			return
		}
		if l, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			fatal("Không mở được cổng cục bộ: " + err.Error())
		}
	}
	url := fmt.Sprintf("http://%s/", l.Addr().String())

	sub, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("/", noCache(http.FileServer(http.FS(sub))))
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"version": version, "repo": repo})
	})
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		lastPing.Store(time.Now().Unix())
		w.WriteHeader(204)
	})
	mux.HandleFunc("/api/update/check", handleCheck)
	mux.HandleFunc("/api/update/install", handleInstall)

	// Tự thoát khi cửa sổ đã đóng (giao diện gửi ping mỗi 5 giây).
	lastPing.Store(time.Now().Unix() + 60) // cho trình duyệt 60 giây để mở lần đầu
	go func() {
		for range time.Tick(5 * time.Second) {
			if time.Now().Unix()-lastPing.Load() > 25 {
				os.Exit(0)
			}
		}
	}()

	go openWindow(url)
	if err := http.Serve(l, mux); err != nil {
		fatal(err.Error())
	}
}

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

/* ---------------- Cập nhật qua GitHub Releases ---------------- */

// Tệp mô tả bản mới nhất nằm trong kho GitHub công khai: update/latest.json
// {"version":"1.0.1","notes":"...","url":"https://.../GKM_SoSanhHoaDon.exe","sha256":"...","size":123}
type manifest struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

var manifestURL = "https://raw.githubusercontent.com/" + repo + "/main/update/latest.json"

func latestManifest() (*manifest, error) {
	c := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", manifestURL+"?t="+strconv.FormatInt(time.Now().Unix(), 10), nil)
	req.Header.Set("User-Agent", "GKM-SoSanhHoaDon/"+version)
	req.Header.Set("Cache-Control", "no-cache")
	res, err := c.Do(req)
	if err != nil {
		return nil, errors.New("offline")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("máy chủ trả mã %d", res.StatusCode)
	}
	var m manifest
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil || m.Version == "" {
		return nil, errors.New("tệp thông tin cập nhật không hợp lệ")
	}
	m.Version = strings.TrimPrefix(m.Version, "v")
	return &m, nil
}

func handleCheck(w http.ResponseWriter, r *http.Request) {
	m, err := latestManifest()
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "offline": err.Error() == "offline", "error": err.Error(), "current": version})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "current": version, "latest": m.Version,
		"newer": cmpVer(m.Version, version) > 0 && m.URL != "", "notes": m.Notes})
}

// handleInstall tải bản mới, kiểm tra SHA-256, thay file exe đang chạy (Windows cho đổi tên file đang chạy), rồi khởi động lại.
func handleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST", 405)
		return
	}
	fail := func(msg string) { writeJSON(w, map[string]any{"ok": false, "error": msg}) }
	m, err := latestManifest()
	if err != nil {
		fail("Không kết nối được máy chủ cập nhật")
		return
	}
	if m.URL == "" || cmpVer(m.Version, version) <= 0 {
		fail("Không có bản mới để cài")
		return
	}
	exe, err := os.Executable()
	if err != nil {
		fail(err.Error())
		return
	}
	exe, _ = filepath.EvalSymlinks(exe)
	tmp := exe + ".new"
	c := &http.Client{Timeout: 5 * time.Minute}
	res, err := c.Get(m.URL)
	if err != nil || res.StatusCode != 200 {
		fail("Tải bản mới không thành công, vui lòng thử lại")
		return
	}
	defer res.Body.Close()
	f, err := os.Create(tmp)
	if err != nil {
		fail("Không ghi được file vào thư mục ứng dụng: " + err.Error())
		return
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), res.Body)
	f.Close()
	if err != nil || (m.Size > 0 && n != m.Size) || !looksLikeExe(tmp) ||
		(m.SHA256 != "" && !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), m.SHA256)) {
		os.Remove(tmp)
		fail("File tải về bị lỗi, vui lòng thử lại")
		return
	}
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		os.Remove(tmp)
		fail("Không thay được file ứng dụng: " + err.Error())
		return
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Rename(old, exe)
		fail("Không thay được file ứng dụng: " + err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "version": m.Version})
	if fl, ok := w.(http.Flusher); ok {
		fl.Flush()
	}
	go func() {
		time.Sleep(800 * time.Millisecond)
		restart(exe)
		os.Exit(0)
	}()
}

func looksLikeExe(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 2)
	_, err = io.ReadFull(f, b)
	return err == nil && b[0] == 'M' && b[1] == 'Z'
}

func cleanupOld() {
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".old")
		os.Remove(exe + ".new")
	}
}

func cmpVer(a, b string) int {
	x, y := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		var p, q int
		if i < len(x) {
			p, _ = strconv.Atoi(x[i])
		}
		if i < len(y) {
			q, _ = strconv.Atoi(y[i])
		}
		if p != q {
			return p - q
		}
	}
	return 0
}
