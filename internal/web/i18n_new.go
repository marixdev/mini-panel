package web

// Vietnamese text of the "add a website" flow.
func init() {
	for k, v := range map[string]string{
		"page.sitenew": "Thêm website", "sites.new": "Thêm website",
		"new.title": "Thêm website", "new.sub": "Chọn loại website bạn muốn tạo.", "new.back": "Quay lại", "new.create": "Tạo website",
		"new.desc.php":    "Website PHP, ví dụ WordPress hoặc Laravel. Mỗi website có nhóm tiến trình PHP riêng.",
		"new.desc.python": "Ứng dụng Python (Flask, Django, FastAPI…) chạy trong môi trường ảo riêng như một dịch vụ, nginx chuyển tiếp yêu cầu tới nó.",
		"new.desc.node":   "Ứng dụng Node.js chạy như một dịch vụ riêng, nginx chuyển tiếp yêu cầu tới nó.",
		"new.desc.static": "Chỉ phục vụ các tệp HTML, CSS, hình ảnh có sẵn.",
		"new.desc.proxy":  "Chuyển tiếp toàn bộ yêu cầu tới một địa chỉ khác, ví dụ một ứng dụng chạy ở máy khác.",
		"f.hosts.hint":    "Một tên miền, ví dụ example.com. Việc dùng thêm www chọn sau ở Tổng quan, còn SSL cấp ở tab SSL.",
		"f.user":          "Tài khoản website", "f.user.hint": "Tài khoản SSH/SFTP chính của website. Tự điền theo tên miền, bạn có thể sửa.",
		"f.password": "Mật khẩu tài khoản", "f.password.hint": "Dùng để đăng nhập SSH/SFTP. Sau khi tạo, mật khẩu hiện một lần ở trang kết quả: hãy lưu lại.",
		"f.password.gen": "Tạo mật khẩu mới", "f.password.empty": "Để trống để tự tạo",
		"act.site.access": "Cấp quyền SSH/SFTP cho %s",
		"res.access":      "Đăng nhập SSH/SFTP\n  máy chủ:   %s\n  tài khoản: %s\n  mật khẩu:  %s",
		"err.password":    "Mật khẩu không hợp lệ: cần 12 đến 64 ký tự, không có khoảng trắng, dấu nháy, dấu gạch ngược hay dấu huyền.",
		"err.sitename":    "Không tạo được tên website từ tên miền. Hãy kiểm tra lại tên miền.", "err.kind": "Loại website không hợp lệ.",
		"err.nohosts":      "Hãy nhập ít nhất một tên miền.",
		"err.userreserved": "Tên tài khoản %q là tên dành riêng của hệ thống. Hãy chọn tên khác.",
		"err.userexists":   "Máy chủ đã có tài khoản %q. Hãy chọn tên khác.",
		"err.userused":     "Tài khoản %q đã được website %q dùng. Hãy chọn tên khác.",
		"err.siteused":     "Đã có website tên %q.",
		"err.phpmissing":   "PHP %q chưa được cài trên máy này. Cài bằng: mpn plan -php <phiên bản> -apply",
		"err.nodemissing":  "Node.js %q chưa được cài trên máy này. Cài bằng: mpn plan -node <phiên bản> -apply",
		"new.nophp":        "Chưa cài phiên bản PHP nào trên máy này. Cài bằng: mpn plan -php 8.4 -apply",
		"new.nonode":       "Chưa cài phiên bản Node.js nào trên máy này. Cài bằng: mpn plan -node 22 -apply",
		"err.exitstatus":   "Lệnh không thực hiện được. Chi tiết nằm ở phần kết quả bên dưới.",
	} {
		vi[k] = v
	}
}
