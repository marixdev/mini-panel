package web

// Vietnamese text of the Logs tab of a website.
func init() {
	for k, v := range map[string]string{
		"tab.logs":        "Nhật ký",
		"log.title":       "Nhật ký của website",
		"log.sub":         "Những dòng cuối cùng của nhật ký, mới nhất ở cuối. Chỉ đọc, không sửa được gì.",
		"log.kinds":       "Loại nhật ký",
		"log.kind.access": "Truy cập",
		"log.kind.error":  "Lỗi",
		"log.kind.app":    "Ứng dụng",
		"log.search":      "Chỉ hiện dòng có chứa",
		"log.search.ph":   "ví dụ 404, một địa chỉ IP hoặc một đường dẫn",
		"log.lines":       "Số dòng",
		"log.show":        "Xem",
		"log.size":        "Dung lượng cả tệp",
		"log.matched":     "Số dòng khớp",
		"log.truncated":   "Tệp rất dài, chỉ tìm trong phần cuối",
		"log.view":        "Nội dung nhật ký",
		"log.missing":     "Chưa có nhật ký tại %s. Tệp được tạo khi website nhận yêu cầu đầu tiên.",
		"log.empty":       "Nhật ký đang trống.",
		"log.nomatch":     "Không có dòng nào chứa đoạn chữ này trong phần cuối của nhật ký.",
		"log.hint":        "Nhật ký truy cập ghi từng yêu cầu gửi đến website, nhật ký lỗi ghi lỗi của nginx. Với website Node hoặc Python, nhật ký ứng dụng lấy từ dịch vụ của website. Nhật ký cũ được xoay vòng hằng ngày và giữ 14 bản.",
		"log.err.query":   "Đoạn chữ tìm kiếm không hợp lệ nên không được dùng.",
	} {
		vi[k] = v
	}
}
