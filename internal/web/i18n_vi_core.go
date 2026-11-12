package web

// Vietnamese text of the shell of every page: the name, the navigation, the sign-in form and the errors of the first-run page.
func init() {
	for k, v := range map[string]string{
		"app.name":         "Mini Panel",
		"theme.toggle":     "Đổi giao diện sáng/tối",
		"nav.main":         "Điều hướng chính",
		"nav.skip":         "Bỏ qua, vào nội dung",
		"login.title":      "Đăng nhập",
		"login.wrong":      "Sai tên đăng nhập hoặc mật khẩu.",
		"login.locked":     "Bạn đã nhập sai quá nhiều lần. Hãy thử lại sau %d giây.",
		"page.login":       "Đăng nhập",
		"page.setup":       "Thiết lập Mini Panel",
		"page.notfound":    "Không tìm thấy trang",
		"setup.err.user":   "Tên đăng nhập không hợp lệ. Chỉ dùng chữ thường, số và dấu gạch ngang/dưới, bắt đầu bằng chữ cái.",
		"setup.err.short":  "Mật khẩu phải có ít nhất %d ký tự.",
		"setup.err.long":   "Mật khẩu quá dài (tối đa 256 ký tự).",
		"setup.err.match":  "Hai mật khẩu không khớp nhau.",
		"setup.err.hash":   "Không thể mã hóa mật khẩu. Hãy thử lại.",
		"setup.err.nosave": "Chưa thể lưu tài khoản: chức năng lưu chưa được cấu hình.",
		"setup.err.write":  "Không thể lưu tài khoản: %s",
	} {
		vi[k] = v
	}
}
