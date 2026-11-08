package web

// Vietnamese text of the restore from Google Drive: the page that lists the copies on the drive and the state of a download.
func init() {
	for k, v := range map[string]string{
		"page.cloudlist":   "Khôi phục từ Google Drive",
		"cld.restore":      "Khôi phục từ Google Drive",
		"cld.restore.d":    "Xem các bản sao lưu của website trên Drive và tải một bản về máy chủ. Sau khi tải xong, bản đó nằm trong danh sách sao lưu ở trên và bạn khôi phục như bình thường.",
		"cld.restore.btn":  "Xem các bản trên Drive",
		"cld.list.title":   "Các bản sao lưu của %s trên Google Drive",
		"cld.list.sub":     "Bấm Tải về máy chủ để chép một bản về. Việc tải chạy nền và có thể mất vài phút đến vài giờ tùy dung lượng. Panel kiểm tra từng tệp với mã kiểm tra của Google trước khi cho dùng.",
		"cld.list.empty":   "Trên Drive chưa có bản sao lưu nào của website này.",
		"cld.list.empty.d": "Bật đồng bộ ở tab Sao lưu rồi bấm Đồng bộ ngay, hoặc đợi lần sao lưu đêm.",
		"cld.list.col.at":  "Thời điểm", "cld.list.col.size": "Dung lượng", "cld.list.col.state": "Trên máy chủ",
		"cld.list.fetch": "Tải về máy chủ", "cld.list.local": "Đã có",
		"cld.list.none":         "Chưa có",
		"cld.list.back":         "Quay lại tab Sao lưu",
		"cld.list.next":         "Sau khi tải xong, vào tab Sao lưu, chọn bản đó và bấm Khôi phục. Bản tải về nằm cùng chỗ với các bản sao lưu thường, nên lần sao lưu đêm tới có thể xóa nó nếu nó cũ hơn %d bản mới nhất.",
		"cld.fetch.running":     "Đang tải bản %s từ Google Drive về máy chủ. Trang này không tự cập nhật, hãy tải lại sau ít phút.",
		"cld.fetch.ok":          "Đã tải bản %s về máy chủ. Mở danh sách sao lưu phía trên và bấm Khôi phục.",
		"cld.fetch.fail":        "Tải bản %s thất bại.",
		"cld.fetch.interrupted": "Việc tải bị dừng giữa chừng (máy chủ khởi động lại hoặc tiến trình bị tắt). Bấm tải lại.",
		"act.cld.fetch":         "Tải một bản sao lưu của website %s từ Google Drive về máy chủ",
		"cld.err.exists":        "Bản này đã có trên máy chủ rồi, không cần tải lại. Hãy khôi phục từ danh sách sao lưu.",
		"cld.err.nospace":       "Máy chủ không đủ chỗ trống để tải bản này. Xóa bớt tệp hoặc bản sao lưu cũ rồi thử lại.",
		"cld.err.nocopy":        "Bản này trên Drive không đầy đủ (thiếu tệp hoặc chưa ghi xong), nên không dùng được. Chọn bản khác.",
		"cld.err.damaged":       "Một tệp tải về bị hỏng (mã kiểm tra không khớp với Google). Panel đã bỏ bản tải dở. Thử tải lại.",
	} {
		vi[k] = v
	}
}
