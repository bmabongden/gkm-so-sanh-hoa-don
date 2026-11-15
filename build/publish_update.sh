#!/usr/bin/env bash
# Tạo update/latest.json cho bản vừa build (chạy sau build/build.sh), rồi commit + push lên nhánh main.
# Máy nhân viên bấm "Cập nhật ứng dụng" sẽ đọc file này, tải exe, kiểm tra SHA-256 rồi tự thay bản mới.
# Cách dùng: bash build/publish_update.sh "Ghi chú bản này"
set -euo pipefail
cd "$(dirname "$0")/.."
EXE=app/GKM_SoSanhHoaDon.exe
VER=$(grep -oE 'version += "[0-9]+\.[0-9]+\.[0-9]+"' source/main.go | grep -oE '[0-9]+\.[0-9]+\.[0-9]+')
SHA=$(sha256sum "$EXE" | cut -d' ' -f1); SIZE=$(stat -c %s "$EXE")
mkdir -p update
python3 - "$VER" "$SHA" "$SIZE" "${1:-}" <<'PY'
import json,sys
v,sha,size,notes=sys.argv[1:5]
url="https://github.com/bmabongden/gkm-so-sanh-hoa-don/raw/main/app/GKM_SoSanhHoaDon.exe"
json.dump({"version":v,"notes":notes,"url":url,"sha256":sha,"size":int(size)},open("update/latest.json","w",encoding="utf-8"),ensure_ascii=False,indent=2)
PY
echo "Đã tạo update/latest.json: bản $VER, $SIZE byte"
