# Nhật ký thay đổi

Mọi thay đổi đáng kể của Mini Panel được ghi ở đây. Số phiên bản theo [Semantic Versioning](https://semver.org/lang/vi/).

## [1.0.3] - 2026-11-13

### Thêm

- Chạy trên máy ARM (arm64, aarch64). Bộ cài nhận máy aarch64 và tải `mpn_linux_arm64`. Module PageSpeed và ModSecurity cho arm64 lấy từ bản [nginx-1.28.3-r2](https://github.com/marixdev/mpn-nginx-modules/releases/tag/nginx-1.28.3-r2) (build trên runner ARM của GitHub, ghim SHA-256 theo từng hệ điều hành và kiến trúc). Web mail lên [mpn-mail 1.0.2](https://github.com/marixdev/mpn-mail/releases/tag/v1.0.2), có bản `mpn-mail_linux_arm64`.
- Bật mail tự gỡ các máy chủ thư có sẵn sẽ chiếm cổng 25 (Exim, Sendmail, OpenSMTPD) trước khi cài Postfix, thay vì dừng lại và bắt gỡ bằng tay.

### Sửa

- Mail bật được trên Debian 13 (và Ubuntu 26.04): panel nhận ra Dovecot 2.4 và viết `dovecot.conf` theo cú pháp mới (`dovecot_config_version`, `mail_driver`, hạn mức `quota_storage_size`). Dovecot 2.3 giữ cấu hình cũ.
- Bước gói cơ bản cài `cron` và bật nó: một số bản Debian 13 không có sẵn, nên tab Cron không chạy được việc nào.

## [1.0.2] - 2026-11-12

### Thêm

- Trình quản lý tệp có Cắt, Sao chép và Dán, trong menu chuột phải, trên thanh khi chọn nhiều mục và bằng phím Ctrl+X, Ctrl+C, Ctrl+V. Mục đã cắt hoặc sao chép được nhớ theo từng website trong thẻ trình duyệt đang mở, nên có thể mở thư mục khác rồi dán vào đó. Bấm chuột phải lên một thư mục sẽ dán vào chính thư mục ấy.
- Sao chép vào chính thư mục đang chứa mục gốc thì bản sao được đặt tên `tên (copy).đuôi`, `tên (copy 2).đuôi`, v.v. Sao chép giữ quyền của tệp gốc, còn liên kết và tệp đặc biệt được bỏ qua.

### An toàn

- Dán không bao giờ ghi đè: nếu tên đã có ở thư mục đích thì không có mục nào bị chuyển hoặc chép. Không dán được một thư mục vào chính nó hay thư mục con của nó, và không đi ra ngoài thư mục home của website.

## [1.0.1] - 2026-11-12

### Thêm

- Giao diện tiếng Trung giản thể (`zh`), cạnh tiếng Việt và tiếng Anh. Chọn ở nút ngôn ngữ trên thanh trên cùng, kể cả ở trang đăng nhập và `/setup`; lần đầu vào, panel chọn theo `Accept-Language` của trình duyệt.
- Phông chữ có thêm PingFang SC, Microsoft YaHei và Noto Sans CJK SC, vì JetBrains Mono không có chữ Hán.

### Thay đổi

- Web mail lên [mpn-mail 1.0.1](https://github.com/marixdev/mpn-mail/releases/tag/v1.0.1), bản có tiếng Trung và đi theo ngôn ngữ của panel.
- Mô tả các thiết lập PHP trong tab Tổng quan nay hiện theo ngôn ngữ đang chọn, trước đây luôn là tiếng Việt.
- README có thêm bản tiếng Anh ([README.en.md](README.en.md)) và tiếng Trung ([README.zh.md](README.zh.md)), kèm bảng các repo đi kèm (mpn-mail, mpn-nginx-modules).

## [1.0.0] - 2026-11-12

Bản phát hành đầu tiên: website PHP, Node.js, Python, tĩnh và proxy; SSL Let's Encrypt; MariaDB/MySQL và phpMyAdmin; email và web mail mpn-mail 1.0.0; sao lưu; tường lửa, ModSecurity, fail2ban, 2FA; giám sát; giao diện tiếng Việt và tiếng Anh.

[1.0.2]: https://github.com/marixdev/mini-panel/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/marixdev/mini-panel/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/marixdev/mini-panel/releases/tag/v1.0.0
