package web

// Vietnamese text of the page of one website.
func init() {
	for k, v := range map[string]string{
		"page.site": "Website", "site.back": "Tất cả website",
		"tab.overview": "Tổng quan", "tab.ssh": "SSH/SFTP", "tab.databases": "Cơ sở dữ liệu", "tab.backups": "Sao lưu",
		"ov.type": "Loại", "ov.hosts": "Tên miền", "ov.detail": "Chi tiết", "ov.user": "Tài khoản", "ov.root": "Thư mục web", "ov.https": "SSL",
		"ov.https.on": "Đã bật", "ov.https.off": "Chưa bật", "ov.actions": "Thao tác",
		"ssh.title": "Đăng nhập SSH/SFTP", "ssh.sub": "Dùng để tải tệp lên và quản lý website bằng dòng lệnh hoặc phần mềm SFTP.",
		"ssh.state": "Trạng thái", "ssh.on": "Đang bật", "ssh.off": "Đang tắt",
		"ssh.host": "Máy chủ", "ssh.port": "Cổng", "ssh.user": "Tài khoản",
		"ssh.key": "Khóa của bảng điều khiển", "ssh.key.ok": "Đăng nhập được", "ssh.key.broken": "Không đăng nhập được, hãy khôi phục", "ssh.key.unknown": "Chưa kiểm tra",
		"ssh.enable": "Bật đăng nhập", "ssh.reset": "Đặt lại mật khẩu", "ssh.disable": "Tắt đăng nhập", "ssh.restore": "Khôi phục khóa",
		"ssh.note":     "Mật khẩu chỉ hiện một lần, ngay sau khi tạo hoặc đặt lại, và không được lưu ở đâu cả. Terminal và trình quản lý tệp của bảng điều khiển đăng nhập bằng khóa riêng nên không cần mật khẩu này.",
		"ssh.off.note": "Đăng nhập đang tắt: tài khoản này không vào được bằng SSH hay SFTP, và terminal, trình quản lý tệp của bảng điều khiển cũng không dùng được.",
		"act.ssh.on":   "Bật đăng nhập SSH/SFTP cho %s", "act.ssh.reset": "Đặt lại mật khẩu SSH/SFTP của %s", "act.ssh.off": "Tắt đăng nhập SSH/SFTP của %s", "act.ssh.key": "Khôi phục khóa của bảng điều khiển cho %s",
		"ssh.reset.title": "Đặt lại mật khẩu của %s", "ssh.reset.btn": "Đặt mật khẩu mới",
		"ssh.reset.warn":    "Mật khẩu cũ ngừng dùng được ngay. Phần mềm SFTP hoặc người đang dùng tài khoản này phải nhập mật khẩu mới. Mật khẩu mới chỉ hiện một lần.",
		"ssh.disable.title": "Tắt đăng nhập của %s", "ssh.disable.btn": "Tắt đăng nhập",
		"ssh.disable.warn": "Tài khoản không vào được bằng SSH hay SFTP nữa, và mật khẩu bị khóa. Website vẫn chạy bình thường. Bật lại sẽ tạo mật khẩu mới.",
		"site.dbs.add":     "Thêm cơ sở dữ liệu cho website này", "site.bk.create": "Sao lưu website này ngay",
		"err.nosite": "Không có website này.",
	} {
		vi[k] = v
	}
}
