package web

// Vietnamese text of the file manager and the code editor.
func init() {
	for k, v := range map[string]string{
		"tab.files": "Tệp", "page.fileedit": "Sửa tệp", "page.filerename": "Đổi tên",
		"fm.path": "Đường dẫn", "fm.back": "Về danh sách tệp", "fm.empty": "Thư mục này đang trống.", "fm.link": "liên kết",
		"fm.col.name": "Tên", "fm.col.size": "Dung lượng", "fm.col.mode": "Quyền", "fm.col.time": "Sửa lúc",
		"fm.cut":    "Thư mục có quá nhiều mục, chỉ hiện một phần. Hãy dùng terminal hoặc SFTP để xem đầy đủ.",
		"fm.upload": "Tải tệp lên", "fm.overwrite": "Ghi đè tệp trùng tên", "fm.upload.btn": "Tải lên",
		"fm.mkdir": "Thư mục mới", "fm.newfile": "Tệp mới", "fm.create": "Tạo",
		"fm.zipname": "Tên tệp zip", "fm.sel.zip": "Nén mục đã chọn", "fm.sel.delete": "Xóa mục đã chọn", "fm.unzip.over": "Ghi đè khi giải nén",
		"fm.edit": "Sửa", "fm.download": "Tải về", "fm.unzip": "Giải nén", "fm.rename": "Đổi tên",
		"fm.rename.title": "Đổi tên", "fm.rename.new": "Tên mới", "fm.rename.btn": "Đổi tên",
		"fm.save": "Lưu", "fm.saved": "Đã lưu tệp.", "fm.edit.fail": "Không mở được tệp để sửa.",
		"fm.editor.loading": "Đang tải trình soạn thảo…", "fm.editor.plain": "Dùng ô soạn thảo đơn giản.", "fm.editor.dirty": "Chưa lưu",
		"fm.editor.hint": "Ctrl+S để lưu.",
		"fm.uploaded":    "Đã tải lên %d tệp.",
		"fm.done.mkdir":  "Đã tạo thư mục.", "fm.done.newfile": "Đã tạo tệp.", "fm.done.rename": "Đã đổi tên.", "fm.done.delete": "Đã xóa.",
		"fm.done.zip": "Đã nén %d tệp và %d thư mục.", "fm.done.zip.fail": "Không nén được.",
		"fm.done.unzip": "Đã giải nén %d tệp và %d thư mục.", "fm.done.unzip.fail": "Không giải nén được.",
		"fm.skipped":      "Bỏ qua %d mục (liên kết hoặc loại tệp đặc biệt).",
		"fm.delete.title": "Xóa %d mục?", "fm.delete.warn": "Thư mục được xóa cùng mọi thứ bên trong. Không hoàn tác được.", "fm.delete.btn": "Xóa",
		"fm.err.path": "Đường dẫn không hợp lệ.", "fm.err.name": "Tên không hợp lệ.", "fm.err.exists": "Đã có mục trùng tên.",
		"fm.err.notfound": "Không tìm thấy.", "fm.err.denied": "Tài khoản của website không có quyền thao tác này.",
		"fm.err.binary": "Đây không phải tệp văn bản nên không sửa ở đây được.", "fm.err.toobig": "Tệp quá lớn.",
		"fm.err.changed": "Tệp đã bị thay đổi sau khi mở.", "fm.err.changed.keep": "Tệp đã bị thay đổi sau khi bạn mở. Nội dung của bạn còn nguyên trên trang, bấm Lưu lần nữa nếu muốn ghi đè.",
		"fm.err.isdir": "Đây là thư mục.", "fm.err.root": "Không thể thao tác trực tiếp lên thư mục gốc của website.",
		"fm.err.toomany": "Có quá nhiều mục.", "fm.err.other": "Lỗi: %s",
		"fm.err.login": "Không đăng nhập được vào tài khoản của website. Vào tab SSH/SFTP để kiểm tra khóa của bảng điều khiển.",
		"c.cancel":     "Hủy",
	} {
		if _, have := vi[k]; !have {
			vi[k] = v
		}
	}
}
