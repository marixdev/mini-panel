package web

// Vietnamese text of the WordPress manager tab.
func init() {
	for k, v := range map[string]string{
		"tab.wordpress": "WordPress", "page.wordpress": "WordPress",
		"wp.version": "Phiên bản WordPress", "wp.url": "Địa chỉ", "wp.admin": "Trang quản trị", "wp.admin.open": "Mở wp-admin",
		"wp.uptodate": "Mới nhất", "wp.update.to": "có bản %s", "wp.core.update": "Cập nhật lên %s",
		"wp.note":          "Panel chạy WP-CLI bằng chính tài khoản của website, nên tệp luôn thuộc đúng chủ sở hữu. Nên sao lưu trước khi cập nhật.",
		"wp.install.title": "Cài plugin hoặc giao diện",
		"wp.install.sub":   "Tìm trên wordpress.org rồi bấm Cài đặt, hoặc nhập thẳng tên gói (slug) nếu bạn đã biết.",
		"wp.search":        "Tìm kiếm", "wp.search.btn": "Tìm", "wp.kind": "Loại", "wp.kind.plugin": "Plugin", "wp.kind.theme": "Giao diện", "wp.term": "Từ khóa",
		"wp.slug.title": "Cài theo tên gói", "wp.slug": "Tên gói (slug)", "wp.install.btn": "Cài đặt",
		"wp.slug.hint":  "Là phần cuối địa chỉ trên wordpress.org, ví dụ contact-form-7. Chỉ cài từ wordpress.org.",
		"wp.found.none": "Không tìm thấy kết quả nào.",
		"wp.plugins":    "Plugin", "wp.themes": "Giao diện",
		"wp.col.name": "Tên", "wp.col.version": "Phiên bản", "wp.col.status": "Trạng thái",
		"wp.active": "Đang dùng", "wp.inactive": "Chưa bật", "wp.empty": "Chưa có mục nào.",
		"wp.do.activate": "Bật", "wp.do.deactivate": "Tắt", "wp.do.update": "Cập nhật", "wp.do.delete": "Xóa",
		"wp.delete.ask": "Xóa %s khỏi website? Tệp của nó bị gỡ.", "wp.delete.btn": "Xóa hẳn",
		"wp.fail":        "Chưa thực hiện",
		"wp.err.notwp":   "Website này không có WordPress do panel cài.",
		"wp.err.broken":  "Không đọc được WordPress của website này. Tệp của nó có thể bị hỏng hoặc đã bị di chuyển.",
		"wp.err.search":  "Không tìm được trên wordpress.org. Máy chủ có ra được Internet không? Thử lại sau.",
		"wp.err.term":    "Từ khóa chỉ gồm chữ, số, khoảng trắng và . _ + - , tối đa 60 ký tự.",
		"wp.err.action":  "Thao tác hoặc tên gói không hợp lệ.",
		"act.wp.install": "Cài %s cho website %s", "act.wp.activate": "Bật %s của website %s", "act.wp.deactivate": "Tắt %s của website %s",
		"act.wp.update": "Cập nhật %s của website %s", "act.wp.delete": "Xóa %s khỏi website %s",
	} {
		vi[k] = v
	}
}
