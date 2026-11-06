# Mini Panel (`mpn`)

Control panel nhẹ cho VPS RAM thấp (kiểu CloudPanel). Tên sản phẩm là **Mini Panel**, viết tắt **`mpn`**.

Mục tiêu: chạy ổn trên VPS 1 vCPU / 1GB RAM, hỗ trợ PHP 7.0 đến 8.x, Node.js 18/20/22/24, MySQL và MariaDB.

## Quy ước tên

| Thứ | Tên |
|---|---|
| Tệp chạy | `mpn` (cài vào `/usr/local/bin/mpn`) |
| Module Go | `mpn` |
| Cơ sở dữ liệu của mpn | `/var/lib/mpn/mpn.db` |
| Chứng chỉ, khóa tài khoản ACME | `/etc/mpn/certs/<site>`, `/etc/mpn/acme` |
| Thư mục thử thách ACME | `/var/lib/mpn/acme` |
| Log nginx của từng site | `/var/log/mpn/sites/<site>` (cấu hình xoay vòng `/etc/logrotate.d/mpn-sites`) |
| Gia hạn chứng chỉ | `mpn-renew.service` và `mpn-renew.timer` |
| Tệp cấu hình do mpn ghi | hậu tố `99-mpn.*` hoặc tên `mpn.conf` |

Máy đã cài bằng bản trước khi đổi tên (đường dẫn `panel`) không được tự chuyển sang tên mới: dự án chưa phát hành bản nào, các máy thử sẽ bị xóa.

## Quyết định đã chốt

| Mục | Quyết định |
|---|---|
| Ngôn ngữ | Go, build `CGO_ENABLED=0`, SQLite thuần Go (`modernc.org/sqlite`), một binary tĩnh |
| Giao diện | Go template + htmx, nhúng sẵn trong binary |
| Vai trò | Bản đầu chỉ một admin, đa người dùng làm sau |
| OS hỗ trợ | Ubuntu 22.04, 24.04, 26.04. Debian 12, 13 (giống CloudPanel) |
| PHP | Repo `packages.sury.org`, 7.0 đến 8.5 (8.6 coi là thử nghiệm) |
| Database | MariaDB 11.8 LTS (mặc định) và MySQL 8.4 LTS, hỗ trợ nhiều database server |
| Node.js | Bản chính thức từ nodejs.org cài vào `/opt/node/<phiên bản>`, dùng chung, không NVM |
| Chạy ứng dụng Node | Một unit systemd cho mỗi app (có `MemoryMax`), không PM2 |
| Web server | nginx, mỗi site một khối `server`, PHP dùng `fastcgi_pass` unix socket |
| Cache | Mặc định dùng `fastcgi_cache`/`proxy_cache` của nginx, Varnish là tùy chọn |
| Phân quyền | Tiến trình web chạy user thường, helper root nhận thao tác theo danh sách cố định qua unix socket |

## Phát hiện từ CloudPanel 6.0.8 (đọc trên bay.onl và 2fa.pw)

- Panel là Symfony (PHP 8.1) + SQLite `db.sq3`, nginx riêng ở cổng 8443.
- `clp-agent` viết bằng Go, chỉ thu số liệu CPU/RAM/đĩa vào SQLite và gửi telemetry.
- User `clp` có `NOPASSWD: ALL` trong sudoers. Panel bị chiếm là mất root. Không sao chép mô hình này.
- Vhost sinh từ mẫu có biến `{{...}}` (bảng `vhost_template`, khoảng 31 mẫu CMS/framework).
- PHP: mỗi phiên bản một master PHP-FPM, mỗi site một pool (`pm = ondemand`, cổng cấp dần từ 19001 cho 8.4).
- Node: một PM2 God Daemon cho mỗi site, NVM riêng từng user nên phiên bản lệch nhau.
- Cấu hình mặc định quá lớn cho 1GB (`innodb_buffer_pool_size` 512M, `max_connections` 512, opcache 768M).
- RSS của master PHP-FPM nhàn rỗi bị thổi phồng. PSS thực chỉ khoảng 3 đến 5MB mỗi master, phần còn lại nằm trong swap.

## Lộ trình

1. Bộ cài đặt, công thức repo theo từng OS, bộ sinh profile nginx/PHP-FPM/DB theo RAM.
2. Lõi: SQLite, tạo/xóa site (tĩnh, PHP, Node), sinh nginx, pool PHP, unit systemd, tự rollback khi `nginx -t` lỗi.
3. SSL (Let's Encrypt), database, cron, user SSH và key, sao lưu.
4. Giám sát, firewall (nftables), chặn IP/bot, cache, mẫu vhost, CLI và API.
5. Tăng cường bảo mật, kiểm thử trên VPS 1GB thật. Công cụ nhập site từ CloudPanel (nếu làm) là tiện ích riêng, tùy chọn, chỉ đọc dữ liệu của chính người dùng, không đóng gói mã hay mẫu của CloudPanel.

## Còn chưa xác minh

- PHP 8.6 là bản chính thức hay thử nghiệm.
- Chế độ cgroup mặc định và Node.js bản chính thức trên cả năm OS (kiểm tra khi cài thử).
- Nội dung `{{settings}}` trong mẫu vhost (basic auth, chặn IP, chỉ cho Cloudflare).
- Cách ứng dụng Python chạy trong CloudPanel (chưa có site Python để xem).

## Nguyên tắc để không sao chép CloudPanel

- Viết từ yêu cầu chức năng và tài liệu của nginx, PHP-FPM, systemd. Không chép mã, mẫu vhost, tên bảng, tên biến, câu chữ hay giao diện của CloudPanel. Các ghi chú phân tích ở trên chỉ mô tả hành vi để hiểu bài toán, không dùng làm nguồn để chép.
- Mô hình dữ liệu riêng: một bảng `sites` có cột `kind`, tên miền ở bảng `hostnames`, cổng Node là cột có ràng buộc UNIQUE. Không có bảng riêng cho từng loại site.
- Vhost sinh từ cấu trúc Go có kiểu qua `text/template`, kiểm tra đầu vào trước khi sinh. Không dùng mẫu chuỗi `{{biến}}` cho người dùng sửa.
- Tên, logo, giao diện và văn bản của sản phẩm là của riêng dự án. Chỉ nhắc tên CloudPanel trong tiện ích nhập (nếu có).
- Chưa đọc điều khoản hay giấy phép của CloudPanel. Cần đọc trước khi phát hành.
- Thư viện bên thứ ba ghi giấy phép trong NOTICE khi thêm vào.

## Môi trường phát triển

- Thư mục dự án: `D:\mini-panel`.
- Máy Windows chưa có Go và Git. Cần cài Go để build, cross-compile ra `GOOS=linux`.
- Không build trên VPS 1GB. Build trên Windows rồi chép binary lên.
- Không thử nghiệm trên bay.onl hoặc 2fa.pw (đang chạy production).
