package web

// Vietnamese text of the sign-in, first-run and not-found pages.
func init() {
	for k, v := range map[string]string{
		"notfound.description":     "Địa chỉ bạn mở không tồn tại hoặc đã được đổi. Hãy kiểm tra lại đường dẫn, hoặc quay về trang chính để tiếp tục.",
		"setup.description":        "Đây là tài khoản duy nhất để đăng nhập vào Mini Panel. Hãy chọn tên và mật khẩu bạn muốn.",
		"setup.password.hint":      "Ít nhất 12 ký tự. Có thể dùng cả câu làm mật khẩu.",
		"login.code.hint":          "Nhập mã 6 chữ số từ ứng dụng xác thực của bạn.",
		"login.description":        "Dùng tài khoản quản trị để tiếp tục.",
		"notfound.document.title":  "Không tìm thấy trang - Mini Panel",
		"setup.username.hint":      "chỉ gồm chữ thường, số, dấu gạch",
		"setup.submit":             "Tạo tài khoản và đăng nhập",
		"notfound.heading":         "Không tìm thấy trang này",
		"login.document.title":     "Đăng nhập - Mini Panel",
		"setup.heading":            "Tạo tài khoản của bạn.",
		"foot.cpu":                 "%s CPU",
		"foot.ram":                 "%s RAM",
		"login.heading":            "Đăng nhập vào panel.",
		"setup.document.title":     "Thiết lập Mini Panel",
		"notfound.back":            "Quay lại trang trước",
		"setup.badge":              "THIẾT LẬP ĐẦU TIÊN",
		"login.code.heading":       "Nhập mã xác thực.",
		"setup.password.confirm":   "Nhập lại mật khẩu",
		"notfound.home":            "Về trang chính",
		"auth.username":            "Tên đăng nhập",
		"auth.password.show.label": "Hiện mật khẩu",
		"auth.skip":                "Bỏ qua form",
		"auth.code":                "Mã xác thực",
		"auth.signin":              "Đăng nhập",
		"auth.confirm":             "Xác nhận",
		"auth.password":            "Mật khẩu",
		"auth.password.show":       "Hiện",
		"auth.password.hide":       "Ẩn",
		"language.label":           "Ngôn ngữ",
	} {
		vi[k] = v
	}
}
