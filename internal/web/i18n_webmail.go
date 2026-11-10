package web

// Vietnamese text of the web mail card of a website.
func init() {
	for k, v := range map[string]string{
		"act.webmail": "Đổi webmail của website %s",
		"wm.title":    "Webmail tại tên miền này",
		"wm.sub":      "Đọc và gửi thư ngay tại /webmail/ của chính website, không cần thêm tên miền hay chứng chỉ. Dùng được với cả tên miền phụ.",
		"wm.on":       "Đang bật", "wm.off": "Đang tắt",
		"wm.btn.on": "Bật webmail", "wm.btn.off": "Tắt webmail",
		"wm.note":      "Đăng nhập bằng địa chỉ email đầy đủ và mật khẩu hộp thư, nên vào từ tên miền nào cũng được. Webmail dùng chung gốc với website: nếu website có lỗ hổng chạy mã, người đang mở webmail ở đó có thể bị đọc thư. Chỉ bật ở website bạn tin cậy.",
		"wm.needs.ssl": "Cần bật SSL cho website trước, vì mật khẩu không được nhập trên kết nối thường.",
		"wm.custom":    "Website đang dùng vHost tự sửa, panel không tự thêm vào được. Hãy thêm đoạn cấu hình /webmail/ vào vHost bằng tay.",
		"wm.err.ssl":   "Hãy bật SSL cho website trước: webmail chỉ chạy qua https.",
		"wm.err.off":   "Hãy bật máy chủ thư và webmail ở trang Email trước.",
		"wm.err.old":   "Chương trình webmail trên máy chủ là bản cũ, chưa chạy được dưới /webmail/. Hãy tắt rồi bật lại webmail ở trang Email để cập nhật.",
	} {
		vi[k] = v
	}
}
