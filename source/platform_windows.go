//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

func fatal(msg string) {
	box := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString("GKM - So Sánh Hóa Đơn")
	m, _ := syscall.UTF16PtrFromString(msg)
	box.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10)
	os.Exit(1)
}

func firstExisting(paths ...string) string {
	for _, p := range paths {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// openWindow mở giao diện bằng Edge/Chrome ở chế độ ứng dụng (không thanh địa chỉ); không có thì dùng trình duyệt mặc định.
func openWindow(url string) {
	pf, pf86, local := os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LocalAppData")
	browser := firstExisting(
		filepath.Join(pf86, `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(pf, `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(pf, `Google\Chrome\Application\chrome.exe`),
		filepath.Join(pf86, `Google\Chrome\Application\chrome.exe`),
		filepath.Join(local, `Google\Chrome\Application\chrome.exe`),
	)
	if browser != "" {
		profile := filepath.Join(local, "GKM_SoSanhHoaDon", "browser-profile")
		_ = os.MkdirAll(profile, 0o755)
		c := exec.Command(browser, "--app="+url, "--user-data-dir="+profile, "--no-first-run",
			"--no-default-browser-check", "--window-size=1240,900")
		if err := c.Start(); err == nil {
			return
		}
	}
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func restart(exe string) {
	c := exec.Command(exe)
	c.Dir = filepath.Dir(exe)
	_ = c.Start()
}
