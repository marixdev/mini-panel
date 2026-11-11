package web

import "fmt"

// vi holds every sentence the panel itself writes, in Vietnamese. Templates call {{T "key"}}; Go code calls T. A key
// with no entry is shown as the key, which the template test catches. Another language is another map of the same keys.
// Text that comes from the mpn commands themselves (the output of an operation) is not translated.
var vi = map[string]string{
	"app.name":     "Mini Panel",
	"theme.toggle": "Đổi giao diện sáng/tối",
	"nav.main":     "Điều hướng chính",
	"nav.skip":     "Bỏ qua, vào nội dung",
	"login.title":  "Đăng nhập",
	"login.sub":    "Quản lý máy chủ web của bạn",
	"login.user":   "Tên đăng nhập",
	"login.pass":   "Mật khẩu",
	"login.submit": "Đăng nhập",
	"login.wrong":  "Sai tên đăng nhập hoặc mật khẩu.",
	"login.locked": "Bạn đã nhập sai quá nhiều lần. Hãy thử lại sau %d giây.",
	"page.login":   "Đăng nhập",
	"page.setup":   "Thiết lập Mini Panel",
	"page.notfound": "Không tìm thấy trang",
	"setup.err.user":   "Tên đăng nhập không hợp lệ. Chỉ dùng chữ thường, số và dấu gạch ngang/dưới, bắt đầu bằng chữ cái.",
	"setup.err.short":  "Mật khẩu phải có ít nhất %d ký tự.",
	"setup.err.long":   "Mật khẩu quá dài (tối đa 256 ký tự).",
	"setup.err.match":  "Hai mật khẩu không khớp nhau.",
	"setup.err.hash":   "Không thể mã hóa mật khẩu. Hãy thử lại.",
	"setup.err.nosave": "Chưa thể lưu tài khoản: chức năng lưu chưa được cấu hình.",
	"setup.err.write":  "Không thể lưu tài khoản: %s",
}

// T returns the text for key, filled in with args when there are any.
func T(key string, args ...any) string {
	s, ok := vi[key]
	if !ok {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}
