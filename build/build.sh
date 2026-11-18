#!/usr/bin/env bash
# Build bản .exe Windows từ Linux/macOS (cross-compile, không cần Windows). Cần Go 1.23+ và go-winres
# (go install github.com/tc-hib/go-winres@v0.3.3) để gắn icon + thông tin phiên bản vào file exe.
# Số phiên bản chỉ sửa MỘT chỗ: const version trong source/main.go. Script này tự chép sang winres.json và giao diện.
set -euo pipefail
cd "$(dirname "$0")/../source"
export PATH="$PATH:$(go env GOPATH)/bin"
VER=$(grep -oE 'version += "[0-9]+\.[0-9]+\.[0-9]+"' main.go | grep -oE '[0-9]+\.[0-9]+\.[0-9]+')
sed -i -E "s/\"(file_version|product_version|version)\": \"[0-9.]+\"/\"\1\": \"$VER.0\"/g; s/\"(FileVersion|ProductVersion)\": \"[0-9.]+\"/\"\1\": \"$VER\"/g" winres/winres.json
sed -i -E "s/^let APP_VERSION = '[0-9.]+';/let APP_VERSION = '$VER';/" web/index.html
go-winres make --arch amd64 --in winres/winres.json
gofmt -l . | (! grep .) || { echo "gofmt: các file trên chưa định dạng"; exit 1; }
go vet . && GOOS=windows go vet .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H=windowsgui" -o ../app/GKM_SoSanhHoaDon.exe .
echo "Đã build bản $VER:"; ls -la ../app/GKM_SoSanhHoaDon.exe
