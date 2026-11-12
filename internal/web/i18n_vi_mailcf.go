package web

// Vietnamese text of the page that adds the DNS records of a mail domain to Cloudflare.
func init() {
	for k, v := range map[string]string{
		"page.emailcf":          "Thêm DNS qua Cloudflare",
		"act.mail.cf":           "Thêm bản ghi DNS của tên miền thư %s vào Cloudflare",
		"mail.cf.btn":           "Thêm DNS qua Cloudflare",
		"mail.cf.back":          "Quay lại Email",
		"mail.cf.title":         "Thêm bản ghi DNS vào Cloudflare",
		"mail.cf.sub":           "Tên miền %s đang dùng máy chủ tên của Cloudflare. Panel có thể thêm giúp các bản ghi dưới đây, bạn không phải sao chép tay.",
		"mail.cf.recs":          "Các bản ghi sẽ được thêm",
		"mail.cf.hostnote":      "Nếu tên máy chủ thư %s chưa có bản ghi A và máy chủ này có địa chỉ IP công khai, panel thêm cả bản ghi A (không qua proxy của Cloudflare, vì thư không đi qua proxy). Bản ghi đã có thì được giữ nguyên.",
		"mail.cf.token.title":   "Cho panel quyền sửa DNS của tên miền này",
		"mail.cf.step1.a":       "Vào trang tạo API token của Cloudflare:",
		"mail.cf.step2":         "Chọn Create Token, rồi dùng mẫu \"Edit zone DNS\".",
		"mail.cf.step3":         "Ở phần Zone Resources, chọn đúng zone %s. Quyền cần có: Zone · DNS · Edit và Zone · Zone · Read.",
		"mail.cf.step4":         "Tạo token, sao chép và dán vào ô dưới đây. Dùng xong bạn nên xóa token trong Cloudflare.",
		"mail.cf.token":         "API token",
		"mail.cf.token.hint":    "Token chỉ dùng cho lần này, không được lưu trên máy chủ.",
		"mail.cf.add":           "Thêm bản ghi",
		"mail.cf.zone":          "Zone: %s",
		"mail.cf.st.added":      "Đã thêm",
		"mail.cf.st.exists":     "Đã có sẵn",
		"mail.cf.st.conflict":   "Khác với bản ghi đang có",
		"mail.cf.st.failed":     "Lỗi",
		"mail.cf.nohostip":      "Máy chủ này không có địa chỉ IP công khai nên bản ghi A của %s chưa được thêm. Hãy tự thêm bản ghi A trỏ về địa chỉ máy chủ.",
		"mail.cf.conflict":      "Có bản ghi khác đang đứng ở chỗ này và panel không ghi đè hay xóa. Hãy kiểm tra và sửa tay trong Cloudflare nếu bạn muốn đổi.",
		"mail.cf.err.token":     "API token không đúng dạng. Token gồm 20 đến 128 chữ cái, chữ số, dấu gạch dưới hoặc gạch ngang.",
		"mail.cf.err.nodomain":  "Tên miền này chưa được thêm vào máy chủ thư.",
		"mail.cf.err.badtoken":  "Cloudflare không chấp nhận API token này. Hãy kiểm tra lại token.",
		"mail.cf.err.noperm":    "Token không đủ quyền. Cần Zone · DNS · Edit và Zone · Zone · Read cho đúng zone của tên miền.",
		"mail.cf.err.nozone":    "Token không thấy zone nào của tên miền này. Hãy chọn đúng zone khi tạo token.",
		"mail.cf.err.ratelimit": "Cloudflare đang giới hạn số yêu cầu. Hãy thử lại sau một phút.",
		"mail.cf.err.network":   "Không kết nối được tới Cloudflare từ máy chủ.",
		"mail.cf.err.off":       "Hãy bật máy chủ thư trước.",
	} {
		vi[k] = v
	}
}
