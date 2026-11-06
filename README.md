# Mini Panel (`mpn`)

Bảng điều khiển hosting nhẹ, viết bằng Go, cho VPS RAM thấp (mục tiêu 1 vCPU / 1 GB). Một tệp chạy tĩnh duy nhất cài sẵn nginx, PHP-FPM nhiều phiên bản, Node.js, MariaDB hoặc MySQL, rồi quản lý website, cơ sở dữ liệu, SSL và sao lưu qua một giao diện web tiếng Việt.

Dự án được viết từ đầu theo yêu cầu chức năng. Nó không chứa mã, mẫu cấu hình hay giao diện của sản phẩm nào khác. Chi tiết nằm trong [PLAN.md](PLAN.md).

> Trạng thái: đang phát triển, chưa phát hành bản nào và chưa chọn giấy phép. Đừng đưa lên máy chủ đang chạy thật.

## Có gì

**Loại website:** PHP (7.0 đến 8.5, mỗi site một pool PHP-FPM chạy theo yêu cầu), Node.js (18, 20, 22, 24), Python (python3 của hệ thống, mỗi site một môi trường ảo), tĩnh, và proxy ngược. Node và Python chạy như dịch vụ systemd riêng, có giới hạn bộ nhớ.

**Mỗi website:**
- Một tài khoản hệ thống riêng, một tên miền, tùy chọn www chuyển hướng.
- Tab Tổng quan: trạng thái, dung lượng, thư mục web con, cài đặt theo loại site. PHP chỉ đổi được các tham số trong một danh sách an toàn cố định.
- Tab SSL: Let's Encrypt (kiểm tra DNS trước, tự gia hạn), chứng chỉ riêng đã mua (kiểm tra khóa, chuỗi, tên được phủ), tạo CSR, chuyển hướng http sang https.
- Trình quản lý tệp qua SFTP (sửa bằng CodeMirror, nén và giải nén, đổi quyền), terminal trên web, SSH/SFTP bằng khóa riêng của panel.
- Cơ sở dữ liệu theo site, mở phpMyAdmin bằng tài khoản tạm không lưu mật khẩu.
- Sao lưu hằng đêm (tệp và cơ sở dữ liệu), khôi phục từ giao diện.

**Hệ điều hành:** Ubuntu 22.04, 24.04, 26.04 và Debian 12, 13.

## Cách hoạt động và bảo mật

- `mpn web` chạy bằng người dùng thường, chỉ nghe `127.0.0.1:8780`, đứng sau nginx có TLS ở cổng 2610. Đăng nhập dùng argon2id, có khóa tạm theo IP, CSRF, kiểm tra Host và Origin.
- `mpn root` là trình hỗ trợ chạy bằng root, nghe trên socket Unix. Nó chỉ nhận một danh sách thao tác cố định (gói `internal/ctl`). Mỗi giá trị được kiểm tra lại trước khi thành đối số dòng lệnh, không qua shell.
- Nội dung nhạy cảm (mật khẩu, khóa riêng, chứng chỉ) đi qua đầu vào chuẩn, không nằm trong đối số, log hay cơ sở dữ liệu.
- Mọi thay đổi trên máy được dựng thành một kế hoạch gồm các bước, chạy mặc định ở chế độ xem thử. Chỉ chạy thật khi có `-apply`, và tự khôi phục tệp cũ nếu `nginx -t` hoặc một bước khác lỗi.
- Dữ liệu panel lưu ở SQLite (`/var/lib/mpn/mpn.db`, quyền 0600).

## Cài đặt thử

Cần Go 1.27 trở lên. Build trên máy phát triển, không build trên VPS nhỏ:

```
GOOS=linux CGO_ENABLED=0 go build -o mpn ./cmd/mpn
```

Chép `mpn` lên máy chủ vào `/usr/local/bin/mpn`, rồi:

```
mpn plan -engine mariadb -php 8.4 -node 22 -panel -pma           # xem kế hoạch, chưa thay đổi gì
mpn plan -engine mariadb -php 8.4 -node 22 -panel -pma -apply     # chạy thật (root, Linux)
mpn web passwd                                                    # đặt mật khẩu quản trị, in ra một lần
```

Panel ở `https://<IP>:2610` (chứng chỉ tự ký cho tới khi có chứng chỉ thật). phpMyAdmin ở cổng kế tiếp (2611), cần mở cả hai cổng ở tường lửa của nhà cung cấp. Chạy lại `plan` trên máy đã có cơ sở dữ liệu thì thêm `-engine none`.

## Dòng lệnh

```
mpn version | tune | plan | site | db | backup | root | web
mpn site add|list|show|delete|apply|set|info|domain|remove|access|ssl|ssl-custom|ssl-csr|ssl-off|ssl-redirect|ssl-info|ssl-renew
mpn db add|list|passwd|remove        mpn backup create|list|restore|prune
```

Thêm `-h` sau lệnh con để xem các cờ. Lệnh thay đổi máy đều mặc định xem thử và cần `-apply`.

## Đường dẫn trên máy chủ

| Thứ | Nơi |
|---|---|
| Tệp chạy | `/usr/local/bin/mpn` |
| Cơ sở dữ liệu của panel | `/var/lib/mpn/mpn.db` |
| Chứng chỉ của từng site | `/etc/mpn/certs/<site>/` |
| Khóa tài khoản ACME, thư mục thử thách | `/etc/mpn/acme`, `/var/lib/mpn/acme` |
| Log nginx của từng site | `/var/log/mpn/sites/<site>/` |
| Bản sao lưu | `/var/backups/mpn/<site>/<thời điểm>/` |
| Dịch vụ và hẹn giờ | `mpn-root`, `mpn-web`, `mpn-renew.timer`, `mpn-backup.timer` |

## Phát triển

```
internal/
  installer/   cài nginx, PHP, Node, cơ sở dữ liệu, panel; sinh kế hoạch
  provision/   kế hoạch tạo, đổi, gỡ một website
  site/        mô hình website và kiểm tra đầu vào
  store/       SQLite, migration
  vhost/       sinh cấu hình nginx
  unit/        sinh dịch vụ systemd cho Node và Python
  certs/       kiểm tra chứng chỉ riêng, tạo CSR
  acme/        Let's Encrypt (HTTP-01)
  dnscheck/    kiểm tra tên miền đã trỏ về máy chủ
  ctl/         thao tác root helper chấp nhận
  web/         giao diện (template, i18n tiếng Việt, tài nguyên nhúng)
  fm/          trình quản lý tệp qua SFTP
  dbadmin/ backup/ access/ panelkey/ panelssh/ tune/ webauth/ ...
cmd/mpn/       dòng lệnh
tools/editor/  dựng trình soạn mã CodeMirror thành một tệp nhúng
```

Sau khi sửa template hoặc class trong `internal/web`, dựng lại CSS (Tailwind v4, bản độc lập):

```
cd internal/web
tailwindcss -i assets/input.css -o assets/app.css --minify
```

Sửa trình soạn mã: `cd tools/editor && npm install && npm run build`.

Chữ trên giao diện nằm ở `internal/web/i18n_*.go`, dùng qua `T("khóa")`.

Các bài kiểm thử không nằm trong repo này (xem `.gitignore`).

## Còn thiếu

Trình sửa vhost nginx tùy ý, cron, tường lửa, giám sát tài nguyên, xem log và chặn IP, chứng chỉ thật cho chính panel, nhiều người dùng và 2FA, sao lưu ra ngoài máy chủ, xoay vòng log PHP và Node.

## Giấy phép

Chưa chọn. Thư viện bên thứ ba được liệt kê trong [NOTICE](NOTICE).
