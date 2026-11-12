package web

// Vietnamese text of the terminal.
func init() {
	for k, v := range map[string]string{
		"tab.terminal":    "Terminal",
		"term.hint":       "Đăng nhập bằng chính tài khoản của website. Bảng điều khiển không lưu gì bạn gõ hay thấy ở đây.",
		"term.connecting": "Đang kết nối…",
		"term.connected":  "Đã kết nối",
		"term.closed":     "Phiên đã kết thúc",
		"term.failed":     "Không kết nối được",
		"term.off":        "Terminal cần bật đăng nhập SSH/SFTP cho website này. Hãy bật ở tab",
		"term.loginfail":  "Không đăng nhập được vào tài khoản của website. Vào tab SSH/SFTP để kiểm tra khóa của bảng điều khiển.",
		"term.toomany":    "Đang có quá nhiều terminal mở. Hãy đóng bớt rồi thử lại.",
	} {
		vi[k] = v
	}
}
