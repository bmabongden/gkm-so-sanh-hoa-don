//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func fatal(msg string) { fmt.Fprintln(os.Stderr, msg); os.Exit(1) }

// openWindow: trên máy không phải Windows chỉ in địa chỉ (dùng để kiểm thử).
func openWindow(url string) {
	fmt.Println("Mở giao diện tại:", url)
	if os.Getenv("GKM_NO_BROWSER") == "" {
		_ = exec.Command("xdg-open", url).Start()
	}
}

func restart(exe string) { _ = exec.Command(exe).Start() }
