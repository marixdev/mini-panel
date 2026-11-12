package web

// Vietnamese text of the certificate card of the security page: the certificate of the panel and of the database browser, which share one.
func init() {
	for k, v := range map[string]string{
		"psl.title":            "Chứng chỉ SSL của panel",
		"psl.sub":              "Một chứng chỉ dùng chung cho panel và phpMyAdmin, nên cả hai cổng cùng hết cảnh báo trình duyệt khi bạn truy cập bằng tên miền.",
		"psl.mode.selfsigned":  "Tự ký",
		"psl.mode.letsencrypt": "Let's Encrypt",
		"psl.mode.custom":      "Chứng chỉ riêng",
		"psl.selfsigned.d":     "Panel đang dùng chứng chỉ tự ký nên trình duyệt báo không an toàn. Trỏ một tên miền về máy chủ này rồi cấp chứng chỉ bên dưới.",
		"psl.domain":           "Tên miền",
		"psl.urls":             "Địa chỉ truy cập",
		"psl.panel":            "Panel",
		"psl.pma":              "phpMyAdmin",
		"psl.notcover":         "Chứng chỉ đang dùng không phủ tên miền này.",
		"psl.auto":             "Let's Encrypt, tự gia hạn trước khi hết hạn 30 ngày (cùng bộ hẹn giờ với các website).",
		"psl.manual":           "Chứng chỉ riêng không tự gia hạn: hãy thay trước khi hết hạn.",
		"psl.le.title":         "Cấp bằng Let's Encrypt",
		"psl.le.desc":          "Miễn phí. Panel kiểm tra tên miền đã trỏ về máy chủ, mở một khối nginx ở cổng 80 để Let's Encrypt xác nhận, rồi cài chứng chỉ cho cả hai cổng.",
		"psl.le.btn":           "Cấp chứng chỉ",
		"psl.le.replace":       "Cấp lại",
		"psl.le.hint":          "Bản ghi A của tên miền phải trỏ về %s và cổng 80 phải mở ra Internet (tường lửa của nhà cung cấp cũng vậy). Dùng tên miền riêng cho panel, không dùng tên của một website.",
		"psl.own.title":        "Dùng chứng chỉ riêng",
		"psl.own.desc":         "Dán chứng chỉ đã mua hoặc chứng chỉ từ nơi khác. Phải phủ đúng tên miền ở trên.",
		"psl.own.btn":          "Cài chứng chỉ",
		"psl.relogin":          "Sau khi đổi, hãy mở panel bằng tên miền mới và đăng nhập lại: phiên đăng nhập gắn với địa chỉ nó được tạo.",
		"psl.off.title":        "Quay về chứng chỉ tự ký",
		"psl.off.warn":         "Panel sẽ không còn trả lời bằng tên miền này nữa, chỉ còn truy cập được bằng địa chỉ IP với chứng chỉ tự ký.",
		"psl.off.btn":          "Dùng chứng chỉ tự ký",
		"psl.fail":             "Chưa thực hiện",
		"psl.err.domain":       "Hãy nhập một tên miền viết thường, có dấu chấm, ví dụ panel.example.com. Địa chỉ IP không cấp được chứng chỉ.",
		"psl.err.nokey":        "Hãy dán khóa riêng của chứng chỉ.",
		"psl.err.site":         "Tên miền này đang là địa chỉ của một website. Panel cần một tên riêng, ví dụ panel.tenmien.com.",
		"psl.err.dns":          "Tên miền chưa trỏ về máy chủ này nên Let's Encrypt chưa xác nhận được. Kiểm tra bản ghi A rồi thử lại sau vài phút.",
		"act.psl.le":           "Cấp chứng chỉ Let's Encrypt cho panel",
		"act.psl.custom":       "Cài chứng chỉ riêng cho panel",
		"act.psl.off":          "Quay về chứng chỉ tự ký của panel",
	} {
		vi[k] = v
	}
}
