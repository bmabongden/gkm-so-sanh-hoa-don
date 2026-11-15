# GKM - So Sánh Hóa Đơn

Ứng dụng Windows chạy offline, đối chiếu **số lượng** giữa hóa đơn nhà cung cấp (Honda, Yamaha...) và hàng thực nhận của Gia Khánh (một hoặc nhiều phiếu nhập, tự cộng gộp). Không so sánh giá.

## Tải về

**[⬇ Tải GKM_SoSanhHoaDon.exe](https://github.com/bmabongden/gkm-so-sanh-hoa-don/raw/main/app/GKM_SoSanhHoaDon.exe)** (khoảng 7,7 MB, Windows 10/11)

Tải về rồi bấm đúp để mở, không cần cài đặt. Nếu Windows hiện "Windows protected your PC", bấm **More info → Run anyway**.

## Cách dùng

1. Khung **A**: kéo thả (hoặc bấm chọn) file hóa đơn nhà cung cấp.
2. Khung **B**: kéo thả các file phiếu nhập GK (GK1, GK2...). Nhiều file được cộng gộp theo mã hàng.
3. Bấm **So sánh số lượng**. Kết quả chia thành Khớp, Thiếu / Chưa nhận, Thừa, Ngoài hóa đơn. Bấm vào từng ô để lọc, gõ để tìm mã hoặc tên hàng.
4. Bấm **Xuất Excel** để lưu file đối soát.

App tự nhận cột "Mã hàng", "Tên hàng", "Số lượng" ở bất kỳ vị trí nào trong file (.xlsx, .xls, .csv). Mã hàng được so không phân biệt chữ hoa/thường, khoảng trắng và dấu gạch nối (`12100-KYZ-710` = `12100KYZ710`).

## Cập nhật

Bấm **Cập nhật ứng dụng** trên góc phải (cần có mạng). Nếu có bản mới, app tự tải, kiểm tra SHA-256 và mở lại bản mới.

## Phát hành bản mới (dành cho người phát triển)

1. Sửa số phiên bản ở một chỗ duy nhất: `const version` trong `source/main.go`.
2. `bash build/build.sh` (cần Go 1.23+ và `go-winres`). Script tự chép số phiên bản sang icon/thông tin file và giao diện.
3. `bash build/publish_update.sh "Ghi chú bản mới"`, rồi đưa `app/` cùng `update/` lên nhánh `main`.

Máy nhân viên chỉ nhận bản mới khi `update/latest.json` trên nhánh `main` đổi số phiên bản.

---
Ứng dụng phát triển bởi Gia Khánh Motor, vui lòng không sao chép dưới mọi hình thức.
