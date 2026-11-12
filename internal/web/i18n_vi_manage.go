package web

// Vietnamese text of the site list button and the cards at the end of the Overview tab.
func init() {
	for k, v := range map[string]string{
		"sites.manage": "Quản lý",
		"ov.del.title": "Xóa website",
		"ov.del.d":     "Xóa website sẽ gỡ vĩnh viễn tệp, cơ sở dữ liệu, tài khoản hệ thống và cấu hình của nó. Không thể khôi phục.",
		"ov.del.btn":   "Xóa website",
		"ps.title":     "Tăng tốc website",
		"ps.sub":       "Tự động thu nhỏ HTML, CSS, JS và tối ưu ảnh ngay trên máy chủ, giúp trang tải nhanh hơn mà không cần sửa code.",
		"ps.on":        "Đang bật",
		"ps.off":       "Đang tắt",
		"ps.gzip":      "Thu nhỏ và nén HTML, CSS, JS trước khi gửi về trình duyệt",
		"ps.cache":     "Tối ưu và chuyển đổi ảnh (PNG→JPEG/WebP) để giảm dung lượng",
		"ps.btn.on":    "Bật tăng tốc",
		"ps.btn.off":   "Tắt tăng tốc",
		"ps.note":      "Tệp đã tối ưu được lưu trong bộ nhớ đệm, yêu cầu đầu tiên chậm hơn một chút, từ yêu cầu thứ hai trở đi nhanh hơn.",
		"ps.custom":    "Website đang dùng cấu hình nginx tự sửa ở tab vHost nên không đổi được ở đây. Hãy thêm chỉ thị pagespeed vào cấu hình đó, hoặc về cấu hình mặc định.",
		"act.speed":    "Đổi cài đặt tăng tốc của website %s",
	} {
		vi[k] = v
	}
}
