#!/usr/bin/env bash
# Build bản .exe Windows từ Linux/macOS (cross-compile, không cần Windows). Cần Go 1.23+ và go-winres (icon + thông tin file).
# Đổi số phiên bản ở source/main.go (const version) và source/winres/winres.json trước khi build bản mới.
set -euo pipefail
cd "$(dirname "$0")/../source"
command -v go-winres >/dev/null && go-winres make --arch amd64 --in winres/winres.json || echo "(bỏ qua go-winres, dùng rsrc_windows_amd64.syso có sẵn)"
go vet .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H=windowsgui" -o ../app/GKM_SoSanhHoaDon.exe .
ls -la ../app/GKM_SoSanhHoaDon.exe
