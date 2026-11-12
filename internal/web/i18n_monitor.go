package web

// Vietnamese text of the monitoring page.
func init() {
	for k, v := range map[string]string{
		"nav.monitor": "Giám sát", "page.monitor": "Giám sát",
		"mon.title": "Giám sát máy chủ", "mon.sub": "Tình trạng bộ xử lý, bộ nhớ, ổ đĩa và mạng. Số liệu lấy mỗi 30 giây và giữ 24 giờ.",
		"mon.range": "Khoảng thời gian", "mon.r.1h": "1 giờ", "mon.r.6h": "6 giờ", "mon.r.24h": "24 giờ",
		"mon.now": "Hiện tại", "mon.empty": "Chưa có số liệu", "mon.empty.d": "Panel mới bắt đầu đo. Quay lại sau vài phút.",
		"mon.nochart": "Biểu đồ sẽ hiện sau khi có ít nhất hai lần đo (khoảng một phút).",
		"mon.cpu":     "Bộ xử lý", "mon.cpu.sub": "%d nhân",
		"mon.mem": "Bộ nhớ", "mon.mem.sub": "Tổng %s, không tính bộ nhớ đệm hệ thống có thể thu hồi", "mon.mem.used": "Đang dùng", "mon.swap": "swap %s MB",
		"mon.load": "Tải trung bình", "mon.load.sub": "Trong 1 phút, trên %d nhân. Bằng số nhân là đầy tải.",
		"mon.load.ok": "Bình thường", "mon.load.warn": "Khá cao", "mon.load.crit": "Quá tải",
		"mon.uptime": "Thời gian chạy", "mon.uptime.sub": "Từ lần khởi động gần nhất",
		"mon.up.dh": "%d ngày %d giờ", "mon.up.hm": "%d giờ %d phút", "mon.up.m": "%d phút",
		"mon.disk": "Ổ đĩa", "mon.disk.warn": "Sắp đầy", "mon.disk.crit": "Gần hết chỗ",
		"mon.net": "Mạng", "mon.net.sub": "Tốc độ qua các card mạng thật", "mon.net.rx": "Nhận", "mon.net.tx": "Gửi",
		"mon.services": "Dịch vụ", "mon.svc.on": "Đang chạy", "mon.svc.off": "Đã dừng", "mon.svc.failed": "Lỗi",
		"svc.unit.mpn-root": "Mini Panel (trình hỗ trợ)", "svc.unit.mpn-web": "Mini Panel (giao diện)", "svc.unit.mpn-firewall": "Tường lửa",
		"mon.table": "Xem dạng bảng", "mon.t.time": "Thời điểm",
	} {
		vi[k] = v
	}
}
