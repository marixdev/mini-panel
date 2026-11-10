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
