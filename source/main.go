// GKM - So Sánh Hóa Đơn: đối chiếu số lượng giữa hóa đơn nhà cung cấp và hàng thực nhận.
// Ứng dụng chạy hoàn toàn offline: một máy chủ cục bộ (127.0.0.1) phục vụ giao diện, mở bằng Edge/Chrome ở chế độ ứng dụng.
// Mạng chỉ dùng khi người dùng bấm "Cập nhật ứng dụng" (đọc update/latest.json trên GitHub).
package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	version     = "1.0.1"
	repo        = "bmabongden/gkm-so-sanh-hoa-don"
	defaultPort = 47652
	appName     = "GKM-SoSanhHoaDon"
	maxExeSize  = 100 << 20
)

//go:embed web
var webFS embed.FS

var (
	mu       sync.Mutex
	lastPing time.Time // lần cuối cửa sổ báo còn mở
	closeAt  time.Time // cửa sổ vừa báo đóng: thoát sau thời điểm này nếu không có ping mới
	started  = time.Now()

	listener   net.Listener
	listenPort int
	installing atomic.Bool
)

func main() {
	var (
		port     = flag.Int("port", defaultPort, "cổng cục bộ ưu tiên (chỉ chạy một bản)")
		noWindow = flag.Bool("no-window", false, "không mở cửa sổ (dùng khi tự khởi động lại sau cập nhật)")
		showVer  = flag.Bool("version", false, "in phiên bản")
	)
	flag.Parse()
	if *showVer {
		fmt.Println(appName, version)
		return
	}
	if u := os.Getenv("GKM_UPDATE_URL"); u != "" { // chỉ dùng khi kiểm thử
		manifestURL, manifestTrusted = u, true
	}
	cleanupOld()

	l, already := listen(*port)
	if already {
		// Đã có đúng bản này đang chạy: chỉ mở thêm cửa sổ tới bản đó.
		if !*noWindow {
			openWindow(fmt.Sprintf("http://127.0.0.1:%d/", *port))
		}
		return
	}
	listener = l
	listenPort = l.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d/", listenPort)

	go watchdog()
	if !*noWindow {
		go openWindow(url)
	} else {
		fmt.Println(url)
	}
	srv := &http.Server{Handler: guard(routes()), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.Serve(l); err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, http.ErrServerClosed) {
		fatal("Ứng dụng gặp lỗi: " + err.Error())
	}
	select {} // đang thoát hoặc khởi động lại: chờ os.Exit
}

// listen mở cổng cố định để chỉ chạy một bản. Cổng bận vì đúng bản này đang chạy thì trả already=true;
// bận vì bản GKM khác phiên bản thì bảo bản đó thoát rồi chiếm cổng; bận vì chương trình khác thì dùng cổng ngẫu nhiên.
func listen(port int) (l net.Listener, already bool) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", addr)
	if err == nil {
		return l, false
	}
	if v, ok := runningVersion(port); ok {
		if v == version {
			return nil, true
		}
		askQuit(port)
		for i := 0; i < 25; i++ { // chờ bản cũ nhả cổng (tối đa 5 giây)
			time.Sleep(200 * time.Millisecond)
			if l, err = net.Listen("tcp", addr); err == nil {
				return l, false
			}
		}
	}
	for i := 0; i < 5; i++ { // bản cũ có thể vừa thoát (ví dụ ngay sau khi tự cập nhật): thử lại cổng cố định
		time.Sleep(200 * time.Millisecond)
		if l, err = net.Listen("tcp", addr); err == nil {
			return l, false
		}
	}
	if l, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
		fatal("Không mở được cổng cục bộ cho ứng dụng: " + err.Error())
	}
	return l, false
}

func runningVersion(port int) (string, bool) {
	c := &http.Client{Timeout: 1500 * time.Millisecond}
	res, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/version", port))
	if err != nil {
		return "", false
	}
	defer res.Body.Close()
	var v struct {
		Version string `json:"version"`
		Repo    string `json:"repo"`
	}
	_ = json.NewDecoder(io.LimitReader(res.Body, 1<<12)).Decode(&v)
	return v.Version, v.Repo == repo && v.Version != ""
}

func askQuit(port int) {
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/api/quit", port), nil)
	req.Header.Set("X-GKM", "1")
	c := &http.Client{Timeout: 1500 * time.Millisecond}
	if res, err := c.Do(req); err == nil {
		res.Body.Close()
	}
}

func routes() http.Handler {
	sub, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"version": version, "repo": repo})
	})
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastPing, closeAt = time.Now(), time.Time{}
		mu.Unlock()
		writeJSON(w, map[string]any{"version": version})
	})
	mux.HandleFunc("/api/closing", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		closeAt = time.Now().Add(6 * time.Second) // tải lại trang cũng báo đóng: ping mới sẽ hủy
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		go func() { time.Sleep(100 * time.Millisecond); os.Exit(0) }()
	})
	mux.HandleFunc("/api/update/check", handleCheck)
	mux.HandleFunc("/api/update/install", handleInstall)
	return mux
}

// guard chỉ nhận yêu cầu từ chính giao diện của app: địa chỉ phải là 127.0.0.1/localhost,
// yêu cầu thay đổi (POST) phải có tiêu đề X-GKM (trang web khác không gắn được) và đúng Origin.
func guard(next http.Handler) http.Handler {
	local := func(o string) bool {
		return strings.HasPrefix(o, "http://127.0.0.1:") || strings.HasPrefix(o, "http://localhost:")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			// sendBeacon (khi đóng cửa sổ) không gắn được tiêu đề riêng nên /api/closing chỉ kiểm tra Origin.
			if r.Header.Get("X-GKM") != "1" && r.URL.Path != "/api/closing" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if o := r.Header.Get("Origin"); o != "" && !local(o) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// watchdog tự thoát khi cửa sổ đã đóng. Cửa sổ thu nhỏ lâu thì trình duyệt giãn nhịp ping (tới 1 phút/lần),
// nên chỉ coi là mất cửa sổ khi im lặng hẳn 15 phút; đóng cửa sổ thì trang báo /api/closing và app thoát sau 6 giây.
func watchdog() {
	for now := range time.Tick(time.Second) {
		mu.Lock()
		c, last := closeAt, lastPing
		mu.Unlock()
		switch {
		case installing.Load():
		case !c.IsZero() && now.After(c):
			os.Exit(0)
		case last.IsZero() && now.Sub(started) > 5*time.Minute:
			os.Exit(0)
		case !last.IsZero() && now.Sub(last) > 15*time.Minute:
			os.Exit(0)
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

/* ---------------- Cập nhật ---------------- */

// Tệp mô tả bản mới nhất nằm trong kho GitHub công khai: update/latest.json
// {"version":"1.0.1","notes":"...","url":"https://github.com/<repo>/raw/main/app/GKM_SoSanhHoaDon.exe","sha256":"...","size":123}
type manifest struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

var (
	manifestURL     = "https://raw.githubusercontent.com/" + repo + "/main/update/latest.json"
	manifestTrusted = false // true khi kiểm thử: cho phép tải exe từ địa chỉ ngoài kho GitHub
)

var errOffline = errors.New("offline")

func latestManifest() (*manifest, error) {
	c := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", manifestURL+"?t="+strconv.FormatInt(time.Now().Unix(), 10), nil)
	req.Header.Set("User-Agent", appName+"/"+version)
	req.Header.Set("Cache-Control", "no-cache")
	res, err := c.Do(req)
	if err != nil {
		return nil, errOffline
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("máy chủ trả mã %d", res.StatusCode)
	}
	var m manifest
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<16)).Decode(&m); err != nil {
		return nil, errors.New("tệp thông tin cập nhật không hợp lệ")
	}
	m.Version = strings.TrimPrefix(strings.TrimSpace(m.Version), "v")
	if _, ok := parseVer(m.Version); !ok {
		return nil, errors.New("tệp thông tin cập nhật không hợp lệ")
	}
	return &m, nil
}

// installable: bản mới hơn, có đủ địa chỉ tải + SHA-256, và địa chỉ tải nằm trong đúng kho GitHub của app.
func (m *manifest) installable() bool {
	if cmpVer(m.Version, version) <= 0 || len(m.SHA256) != 64 || m.Size <= 0 || m.Size > maxExeSize {
		return false
	}
	return manifestTrusted ||
		strings.HasPrefix(m.URL, "https://github.com/"+repo+"/") ||
		strings.HasPrefix(m.URL, "https://raw.githubusercontent.com/"+repo+"/")
}

func handleCheck(w http.ResponseWriter, r *http.Request) {
	m, err := latestManifest()
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "offline": errors.Is(err, errOffline), "error": err.Error(), "current": version})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "current": version, "latest": m.Version, "newer": m.installable(), "notes": m.Notes})
}

// handleInstall tải bản mới, kiểm tra kích thước + SHA-256 + chạy thử, thay file exe đang chạy
// (Windows cho đổi tên file đang chạy), rồi khởi động lại trên cùng cổng để cửa sổ đang mở tự tải lại.
func handleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST", http.StatusMethodNotAllowed)
		return
	}
	fail := func(msg string) { writeJSON(w, map[string]any{"ok": false, "error": msg}) }
	if !installing.CompareAndSwap(false, true) {
		fail("Đang cập nhật, vui lòng chờ")
		return
	}
	done := false
	defer func() {
		if !done {
			installing.Store(false)
		}
	}()

	m, err := latestManifest()
	if err != nil {
		fail("Không kết nối được máy chủ cập nhật")
		return
	}
	if !m.installable() {
		fail("Không có bản mới để cài")
		return
	}
	exe, err := os.Executable()
	if err != nil {
		fail("Không xác định được file ứng dụng: " + err.Error())
		return
	}
	if p, e := filepath.EvalSymlinks(exe); e == nil {
		exe = p
	}
	tmp := exe + ".new"
	if err := download(m, tmp); err != nil {
		os.Remove(tmp)
		fail(err.Error())
		return
	}
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		os.Remove(tmp)
		fail("Không thay được file ứng dụng (hãy chép app ra thư mục khác rồi thử lại): " + err.Error())
		return
	}
	if err := os.Rename(tmp, exe); err != nil {
		if e2 := os.Rename(old, exe); e2 != nil {
			fail(fmt.Sprintf("Thay file thất bại và không khôi phục được bản cũ: hãy đổi tên %s về %s", old, exe))
			return
		}
		os.Remove(tmp)
		fail("Không thay được file ứng dụng: " + err.Error())
		return
	}
	done = true
	writeJSON(w, map[string]any{"ok": true, "version": m.Version})
	if fl, ok := w.(http.Flusher); ok {
		fl.Flush()
	}
	go func() {
		time.Sleep(500 * time.Millisecond)
		listener.Close() // nhả cổng trước để bản mới dùng lại đúng cổng này
		if err := restart(exe, "-no-window", "-port="+strconv.Itoa(listenPort)); err != nil {
			// bản mới không mở được: trả lại bản cũ rồi mở lại
			if os.Rename(exe, tmp) == nil && os.Rename(old, exe) == nil {
				os.Remove(tmp)
				_ = restart(exe, "-no-window", "-port="+strconv.Itoa(listenPort))
			}
		}
		os.Exit(0)
	}()
}

func download(m *manifest, dst string) error {
	bad := errors.New("File tải về bị lỗi, vui lòng thử lại")
	c := &http.Client{Timeout: 10 * time.Minute}
	res, err := c.Get(m.URL)
	if err != nil {
		return errors.New("Tải bản mới không thành công, vui lòng kiểm tra mạng rồi thử lại")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("Tải bản mới không thành công (máy chủ trả mã %d)", res.StatusCode)
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return errors.New("Không ghi được vào thư mục chứa ứng dụng (hãy chép app ra thư mục khác như Desktop rồi thử lại)")
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(res.Body, maxExeSize+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return errors.New("Tải bản mới bị gián đoạn, vui lòng thử lại")
	}
	if n != m.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), m.SHA256) {
		return bad
	}
	if runtime.GOOS == "windows" && !looksLikeExe(dst) {
		return bad
	}
	// chạy thử bản mới: phải khởi động được và báo đúng phiên bản
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, dst, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(out), m.Version) {
		return errors.New("Bản tải về không chạy được trên máy này, giữ nguyên bản hiện tại")
	}
	return nil
}

func looksLikeExe(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 2)
	_, err = io.ReadFull(f, b)
	return err == nil && string(b) == "MZ"
}

func cleanupOld() {
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".old")
		os.Remove(exe + ".new")
	}
}

func parseVer(s string) ([3]int, bool) {
	var v [3]int
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func cmpVer(a, b string) int {
	x, _ := parseVer(a)
	y, _ := parseVer(b)
	for i := 0; i < 3; i++ {
		if x[i] != y[i] {
			return x[i] - y[i]
		}
	}
	return 0
}
