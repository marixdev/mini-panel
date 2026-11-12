package web

// Vietnamese text of the vHost tab.
func init() {
	for k, v := range map[string]string{
		"tab.vhost":        "vHost",
		"vh.title":         "Cấu hình nginx của website",
		"vh.sub":           "Sửa trực tiếp khối server của nginx cho website này. Panel kiểm tra bằng nginx -t trước khi nạp.",
		"vh.custom.badge":  "Cấu hình tự sửa",
		"vh.default.badge": "Panel tạo",
		"vh.custom.warn":   "Website đang dùng cấu hình bạn tự viết. Đổi tên miền, bật hoặc gỡ SSL, đổi chuyển hướng sẽ không tự cập nhật vào đây và panel sẽ từ chối các thao tác đó cho tới khi bạn về mặc định. Gia hạn Let's Encrypt vẫn chạy bình thường.",
		"vh.default.hint":  "Đây là cấu hình panel tạo từ tên miền, SSL và cài đặt của website. Lưu một bản đã sửa sẽ chuyển website sang dùng bản của bạn.",
		"vh.text":          "Cấu hình",
		"vh.hint":          "Tối đa %d KB. Nếu nginx -t báo lỗi thì cấu hình đang chạy được giữ nguyên.",
		"vh.hint.ssl":      "Website đang có SSL: giữ nguyên đoạn location /.well-known/acme-challenge/ để chứng chỉ Let's Encrypt gia hạn được.",
		"vh.save":          "Lưu và áp dụng",
		"vh.reset.title":   "Về cấu hình mặc định",
		"vh.reset.warn":    "Bản bạn viết bị bỏ, website dùng lại cấu hình panel tạo từ cài đặt hiện tại. Nên sao chép bản đang sửa ra nơi khác trước.",
		"vh.reset.btn":     "Về mặc định",
		"act.vhost.save":   "Lưu cấu hình nginx của website %s",
		"act.vhost.reset":  "Đặt lại cấu hình nginx của website %s",
		"vh.fail":          "Chưa lưu cấu hình",
		"vh.err.check":     "Cấu hình không dùng được: %s.",
		"vh.err.nginx":     "Không áp dụng được. Cấu hình đang chạy được giữ nguyên; lỗi của nginx nằm ở phần kết quả bên dưới.",
		"vh.err.custom":    "Website đang dùng cấu hình nginx tự sửa nên thao tác này không áp dụng được. Hãy về cấu hình mặc định ở tab vHost trước.",
	} {
		vi[k] = v
	}
}
