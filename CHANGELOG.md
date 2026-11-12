# Nhật ký thay đổi

Mọi thay đổi đáng kể của Mini Panel được ghi ở đây. Số phiên bản theo [Semantic Versioning](https://semver.org/lang/vi/).

## [1.0.1] - 2026-11-12

### Thêm

- Giao diện tiếng Trung giản thể (`zh`), cạnh tiếng Việt và tiếng Anh. Chọn ở nút ngôn ngữ trên thanh trên cùng, kể cả ở trang đăng nhập và `/setup`; lần đầu vào, panel chọn theo `Accept-Language` của trình duyệt.
- Phông chữ có thêm PingFang SC, Microsoft YaHei và Noto Sans CJK SC, vì JetBrains Mono không có chữ Hán.

### Thay đổi

- Web mail lên [mpn-mail 1.0.1](https://github.com/marixdev/mpn-mail/releases/tag/v1.0.1), bản có tiếng Trung và đi theo ngôn ngữ của panel.
- Mô tả các thiết lập PHP trong tab Tổng quan nay hiện theo ngôn ngữ đang chọn, trước đây luôn là tiếng Việt.

## [1.0.0] - 2026-11-12

Bản phát hành đầu tiên: website PHP, Node.js, Python, tĩnh và proxy; SSL Let's Encrypt; MariaDB/MySQL và phpMyAdmin; email và web mail mpn-mail 1.0.0; sao lưu; tường lửa, ModSecurity, fail2ban, 2FA; giám sát; giao diện tiếng Việt và tiếng Anh.

[1.0.1]: https://github.com/marixdev/mini-panel/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/marixdev/mini-panel/releases/tag/v1.0.0
