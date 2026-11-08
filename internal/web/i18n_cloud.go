package web

// Vietnamese text of the Google Drive card of the Backups tab.
func init() {
	for k, v := range map[string]string{
		"cld.title": "Đồng bộ lên Google Drive",
		"cld.sub":   "Sau mỗi lần sao lưu hằng đêm, bản mới được chép lên Google Drive của bạn. Nếu máy chủ hỏng, bản sao lưu vẫn còn.",
		"cld.on":    "Đang bật", "cld.off": "Đang tắt", "cld.notconn": "Chưa kết nối",
		"cld.shared":    "Kết nối Google Drive dùng chung cho mọi website; mỗi website bật hoặc tắt đồng bộ riêng.",
		"cld.client.id": "Client ID", "cld.client.secret": "Client secret",
		"cld.client.hint":   "Tạo trong Google Cloud Console: bật Google Drive API, màn hình đồng ý OAuth, rồi tạo \"OAuth client ID\" loại \"TVs and Limited Input devices\". Panel chỉ xin quyền drive.file, tức chỉ thấy các tệp do chính nó tạo.",
		"cld.connect":       "Kết nối Google Drive",
		"cld.pending.title": "Cho phép truy cập",
		"cld.pending.step":  "Mở địa chỉ dưới đây, đăng nhập Google rồi nhập mã này. Mã còn hiệu lực đến %s.",
		"cld.pending.code":  "Mã", "cld.pending.url": "Địa chỉ",
		"cld.finish":  "Tôi đã cho phép",
		"cld.cancel":  "Hủy",
		"cld.account": "Tài khoản", "cld.folder": "Thư mục", "cld.folder.v": "mpn-backups trong Drive của bạn",
		"cld.site.title": "Website này",
		"cld.site.on":    "Bật đồng bộ", "cld.site.off": "Tắt đồng bộ",
		"cld.site.d": "Giữ 7 bản mới nhất trên Drive, cùng số lượng giữ trên máy chủ. Bản cũ hơn trên Drive tự được xóa.",
		"cld.sync":   "Đồng bộ ngay",
		"cld.sync.d": "Chạy nền, có thể mất vài phút đến vài giờ tùy dung lượng. Tải lên theo từng khối nên mất mạng giữa chừng chỉ phải tải lại một khối.",
		"cld.last":   "Lần đồng bộ gần nhất", "cld.last.none": "Chưa đồng bộ lần nào.",
		"cld.last.ok": "Thành công", "cld.last.fail": "Thất bại",
		"cld.last.detail":    "Đã chép %d bản mới, trên Drive có %d bản.",
		"cld.revoked":        "Google không còn chấp nhận quyền đã cấp (bạn có thể đã thu hồi). Hãy ngắt kết nối rồi kết nối lại.",
		"cld.disconnect":     "Ngắt kết nối Google Drive",
		"cld.disconnect.w":   "Panel quên kết nối và thu hồi quyền ở Google. Các tệp đã có trên Drive vẫn còn đó. Việc này áp dụng cho mọi website.",
		"cld.disconnect.btn": "Ngắt kết nối",
		"cld.note":           "Dữ liệu sao lưu gồm cả cơ sở dữ liệu và tệp của website, nên chỉ đồng bộ lên tài khoản Google mà bạn tin cậy.",
		"act.cld.connect":    "Kết nối Google Drive", "act.cld.finish": "Hoàn tất kết nối Google Drive", "act.cld.enable": "Đổi đồng bộ Google Drive của website %s",
		"act.cld.sync": "Đồng bộ sao lưu của website %s lên Google Drive", "act.cld.disconnect": "Ngắt kết nối Google Drive",
		"cld.fail":             "Chưa thực hiện",
		"cld.err.clientfmt":    "Client ID hoặc Client secret không đúng dạng. Hãy sao chép nguyên văn từ Google Cloud Console.",
		"cld.err.client":       "Google không nhận Client ID này. Kiểm tra loại \"TVs and Limited Input devices\" và đã bật Google Drive API.",
		"cld.err.notconnected": "Chưa kết nối Google Drive. Hãy kết nối trước.",
		"cld.err.pending":      "Google chưa nhận được sự cho phép của bạn. Mở địa chỉ, nhập mã, bấm Cho phép ở Google rồi quay lại bấm nút này.",
		"cld.err.denied":       "Bạn đã từ chối cấp quyền ở Google. Hãy kết nối lại nếu muốn thử.",
		"cld.err.expired":      "Mã đã hết hạn. Hãy kết nối lại để lấy mã mới.",
		"cld.err.busy":         "Một lần đồng bộ đang chạy. Đợi nó xong rồi thử lại.",
	} {
		vi[k] = v
	}
}
