package web

// Vietnamese text of the pages after login, kept apart from the shell text in i18n.go.
func init() {
	for k, v := range map[string]string{
		"nav.websites": "Website", "user.logout": "Đăng xuất",
		"page.sites": "Website", "page.result": "Kết quả", "page.confirm": "Xác nhận",

		"sites.title": "Website", "sites.sub": "Các website đang chạy trên máy chủ này.",
		"sites.col.name": "Tên", "sites.col.type": "Loại", "sites.col.hosts": "Tên miền", "sites.col.detail": "Chi tiết",
		"sites.empty": "Chưa có website nào.", "sites.empty.hint": "Tạo website đầu tiên bằng biểu mẫu bên dưới.",
		"sites.apply": "Áp dụng", "sites.remove": "Gỡ", "sites.https": "SSL",
		"sites.add.title": "Thêm website", "sites.add.sub": "Tạo tài khoản hệ thống riêng, cấu hình máy chủ web và áp dụng ngay.",
		"f.name": "Tên", "f.name.hint": "Chữ thường, số và dấu gạch ngang, tối đa 31 ký tự.",
		"f.kind": "Loại", "f.hosts": "Tên miền", "f.hosts.hint": "Cách nhau bằng dấu phẩy hoặc khoảng trắng. Tên đầu tiên là tên chính.",
		"f.php": "Phiên bản PHP", "f.node": "Phiên bản Node.js", "f.python": "Phiên bản Python", "f.entry": "Tệp khởi động", "f.upstream": "Địa chỉ đích", "f.site": "Website",
		"sites.create": "Tạo và áp dụng",
		"kind.static":  "Tĩnh", "kind.php": "PHP", "kind.node": "Node.js", "kind.python": "Python", "kind.proxy": "Proxy ngược",

		"db.title": "Cơ sở dữ liệu", "db.sub": "Mỗi cơ sở dữ liệu có một người dùng riêng, chỉ truy cập được chính nó.",
		"db.col.name": "Cơ sở dữ liệu và người dùng", "db.col.site": "Website", "db.passwd": "Đổi mật khẩu", "db.remove": "Xóa",
		"db.empty": "Chưa có cơ sở dữ liệu nào.", "db.add.title": "Thêm cơ sở dữ liệu",
		"db.name": "Tên (tùy chọn)", "db.name.hint": "Để trống thì dùng tên website (dấu - đổi thành _). Hoặc chỉ gõ phần đuôi, ví dụ gõ shop sẽ thành blog_shop. Chỉ dùng chữ thường, số và dấu gạch dưới.",
		"db.name.ph": "phần đuôi, ví dụ: shop", "db.name.hint2": "Để trống thì tạo cơ sở dữ liệu tên %s. Hoặc gõ phần đuôi, ví dụ gõ shop sẽ thành %[1]s_shop. Chỉ dùng chữ thường, số và dấu gạch dưới.",
		"db.err.taken": "Tên %s đã có (trong panel hoặc trên máy chủ cơ sở dữ liệu). Hãy chọn tên khác.",
		"page.pmaopen": "Mở phpMyAdmin", "pma.title": "Mở phpMyAdmin", "pma.busy": "Đang có quá nhiều lượt mở phpMyAdmin chưa dùng. Hãy thử lại sau ít phút.",
		"pma.notfound": "Panel không quản lý cơ sở dữ liệu này nên không mở được phpMyAdmin cho nó.",
		"pma.gone":     "Cơ sở dữ liệu này có trong panel nhưng không còn trên máy chủ cơ sở dữ liệu (có thể đã bị xóa bằng cách khác).",
		"pma.failed":   "Không mở được phpMyAdmin: %s",
		"pma.open":     "phpMyAdmin", "pma.opening": "Đang mở phpMyAdmin cho cơ sở dữ liệu %s…", "pma.link": "Mở phpMyAdmin",
		"pma.note":        "Lượt đăng nhập này dùng một tài khoản tạm chỉ có quyền trên cơ sở dữ liệu %s, tự hết hạn sau 30 phút hoặc khi bạn đăng xuất khỏi phpMyAdmin. Mật khẩu của cơ sở dữ liệu không bị đụng tới và không được lưu ở đâu.",
		"db.err.chars":    "Tên chỉ được gồm chữ cái a-z, số và dấu gạch dưới (_).",
		"db.err.long":     "Tên quá dài. Tên cơ sở dữ liệu tối đa 32 ký tự, đã gồm phần %s_ ở đầu, nên phần đuôi còn tối đa %d ký tự.",
		"db.err.sitelong": "Tên website quá dài để làm tên cơ sở dữ liệu (%s). Tên tối đa 32 ký tự.", "db.add": "Thêm cơ sở dữ liệu",
		"db.note": "Mật khẩu chỉ hiện một lần, ngay sau khi tạo hoặc đổi. Hãy lưu lại: hệ thống không lưu mật khẩu ở đâu cả.",

		"bk.title": "Sao lưu", "bk.sub": "Tệp và cơ sở dữ liệu của từng website.",
		"bk.col.site": "Website", "bk.col.time": "Thời điểm", "bk.col.state": "Trạng thái", "bk.col.content": "Nội dung", "bk.col.size": "Dung lượng",
		"bk.restore": "Khôi phục", "bk.empty": "Chưa có bản sao lưu nào.", "bk.files": "Tệp", "bk.db": "CSDL",
		"backup.state.ok": "Hoàn tất", "backup.state.incomplete": "Dở dang", "backup.state.unreadable": "Không đọc được",
		"bk.create.title": "Sao lưu ngay", "bk.create": "Sao lưu",
		"bk.prune.title": "Xóa các bản cũ", "bk.prune.keep": "Số bản mới nhất giữ lại", "bk.prune.confirm": "Gõ tên website để xác nhận", "bk.prune": "Xóa các bản cũ hơn",
		"bk.note": "Mỗi đêm khoảng 03:30 mọi website được sao lưu và giữ 7 bản mới nhất. Các bản nằm ngay trên máy chủ này (/var/backups/mpn), nên không bảo vệ được khi mất máy chủ: hãy sao chép chúng ra nơi khác.",

		"res.ok": "Đã hoàn tất.", "res.back": "Quay lại", "res.output": "Chi tiết",
		"res.once":   "Kết quả này chỉ hiện một lần. Tải lại trang sẽ không còn.",
		"c.type.pre": "Gõ", "c.type.post": "để xác nhận", "c.cancel": "Hủy",
		"c.purge": "Đồng thời xóa tài khoản hệ thống, thư mục home và toàn bộ tệp của website",

		"act.site.create": "Tạo website %s", "act.site.apply": "Áp dụng website %s", "act.site.remove": "Gỡ website %s",
		"site.rm.title": "Gỡ website %s", "site.rm.btn": "Gỡ website", "site.rm.fail": "Chưa gỡ website",
		"site.rm.warn": "Website sẽ bị gỡ khỏi máy chủ và địa chỉ web ngừng hoạt động. Nếu không chọn ô bên dưới, tệp và tài khoản của website vẫn được giữ lại.",
		"act.db.add":   "Thêm cơ sở dữ liệu cho %s", "act.db.passwd": "Mật khẩu mới cho cơ sở dữ liệu %s", "act.db.remove": "Xóa cơ sở dữ liệu %s",
		"db.pw.btn": "Đặt mật khẩu mới", "db.rm.btn": "Xóa cơ sở dữ liệu",
		"db.pw.warn":    "Mật khẩu hiện tại ngừng hoạt động ngay. Tệp cấu hình của website (config, .env) phải được cập nhật bằng mật khẩu mới. Mật khẩu mới chỉ hiện một lần.",
		"db.rm.warn":    "Cơ sở dữ liệu cùng toàn bộ dữ liệu và người dùng của nó sẽ bị xóa, không hoàn tác được. Hãy sao lưu trước nếu dữ liệu quan trọng.",
		"act.bk.create": "Sao lưu %s", "act.bk.prune": "Xóa các bản sao lưu cũ của %s", "act.bk.restore": "Khôi phục %s từ %s",
		"bk.rs.title": "Khôi phục %s từ %s", "bk.rs.btn": "Khôi phục", "bk.rs.fail": "Chưa khôi phục", "bk.pr.fail": "Chưa xóa gì",
		"bk.rs.files": "Tệp của website", "bk.rs.db": "Cơ sở dữ liệu %s",
		"bk.rs.warn": "Thao tác này ghi đè các tệp có trong bản sao lưu và các bảng trong bản dump cơ sở dữ liệu. Tệp và bảng tạo sau thời điểm sao lưu vẫn còn. Nếu trạng thái hiện tại quan trọng, hãy sao lưu mới trước.",

		"err.confirm": "Chữ bạn gõ không khớp với tên cần xác nhận.", "err.nothing": "Chưa thay đổi gì",
		"err.pick": "Hãy chọn thứ cần khôi phục: tệp, cơ sở dữ liệu, hoặc cả hai.", "err.keep": "Số bản giữ lại phải từ 1 đến 365.",
		"err.badsite": "Tên website không hợp lệ.", "err.baddb": "Tên cơ sở dữ liệu không hợp lệ.", "err.badbackup": "Tên bản sao lưu không hợp lệ.",
		"err.nobackup": "Không có bản sao lưu hoàn tất với tên này.",
		"err.nohelper": "Tiến trình quản trị chưa được cấu hình.", "err.helper": "Không liên lạc được với tiến trình quản trị: %s",
		"err.badanswer": "Không hiểu câu trả lời của tiến trình quản trị.",
	} {
		vi[k] = v
	}
}
