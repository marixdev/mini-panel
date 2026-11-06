package web

// More Vietnamese text of the file manager: the list, its toolbar and the dialogs.
func init() {
	for k, v := range map[string]string{
		"fm.sel.count": "Đã chọn %d mục", "fm.sel.clear": "Bỏ chọn", "fm.sel.all": "Chọn tất cả",
		"fm.filter": "Lọc theo tên", "fm.filter.none": "Không có mục nào khớp với bộ lọc.",
		"fm.up": "Lên một cấp", "fm.actions": "Thao tác", "fm.items": "mục",
		"fm.empty.hint": "Tải tệp lên hoặc tạo tệp, thư mục mới bằng các nút phía trên.",
		"fm.drag.hint":  "Kéo thả tệp vào trang để tải lên.", "fm.drop": "Thả tệp vào đây để tải lên",
		"fm.drop.here": "Kéo tệp vào đây", "fm.choose": "Chọn tệp", "fm.close": "Đóng",
		"fm.uploading": "Đang tải lên…", "fm.upload.failed": "Tải lên không thành công",
		"fm.name": "Tên", "fm.zip.title": "Nén thành tệp zip", "fm.zip.btn": "Nén",
		"fm.unzip.title": "Giải nén tệp zip", "fm.unzip.into": "Nội dung được giải nén vào thư mục đang mở.",
		"fm.delete.one": "Xóa “%s”?", "fm.fail": "Không thực hiện được.",
		"fm.chmod": "Đổi quyền", "fm.chmod.title": "Đổi quyền truy cập", "fm.chmod.btn": "Áp dụng",
		"fm.perm.owner": "Chủ sở hữu", "fm.perm.group": "Nhóm", "fm.perm.others": "Người khác",
		"fm.perm.r": "Đọc", "fm.perm.w": "Ghi", "fm.perm.x": "Chạy / vào", "fm.perm.octal": "Dạng số",
		"fm.perm.rec": "Áp dụng cho cả những gì bên trong thư mục", "fm.perm.scope.all": "Tệp và thư mục",
		"fm.perm.scope.files": "Chỉ tệp", "fm.perm.scope.dirs": "Chỉ thư mục",
		"fm.perm.hint":    "Nếu thư mục có bit setgid mà bạn đổi quyền, hệ thống có thể gỡ bit này (tài khoản website không thuộc nhóm của thư mục). Panel sẽ báo khi điều đó xảy ra.",
		"fm.perm.lost":    "Hệ thống đã gỡ bit setgid của %d thư mục. Tệp mới trong đó sẽ thuộc nhóm của tài khoản website thay vì nhóm của thư mục.",
		"fm.perm.presets": "Mẫu thường dùng", "fm.perm.file": "Tệp thường: 644", "fm.perm.dir": "Thư mục: 755",
		"fm.done.chmod": "Đã đổi quyền cho %d tệp và %d thư mục.", "fm.done.chmod.fail": "Không đổi được quyền.",
		"fm.err.mode":  "Quyền không hợp lệ, cần ba chữ số từ 0 đến 7.",
		"fm.menu.open": "Mở", "fm.menu.select": "Chọn", "fm.menu.refresh": "Làm mới",
	} {
		if _, have := vi[k]; !have {
			vi[k] = v
		}
	}
}
