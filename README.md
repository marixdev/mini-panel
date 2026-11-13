<p align="center"><img src="logo.svg" alt="Mini Panel" width="72"></p>

<h1 align="center">Mini Panel</h1>

<p align="center">
Bảng điều khiển hosting nhẹ cho VPS nhỏ, viết bằng Go. Một tệp chạy duy nhất, chạy tốt trên máy 1 vCPU / 1 GB RAM.<br>
<a href="https://mpanel.biz">mpanel.biz</a> · <a href="https://mpanel.biz/install/">Cài đặt</a> · <a href="https://github.com/marixdev/mini-panel/releases">Bản phát hành</a>
</p>

<p align="center"><b>Tiếng Việt</b> · <a href="README.en.md">English</a> · <a href="README.zh.md">简体中文</a></p>

---

## Cài đặt

Đăng nhập VPS bằng root rồi chạy:

```sh
curl -fsSL https://mpanel.biz/install.sh | sh
```

Bộ cài kiểm tra hệ điều hành, tải `mpn` từ bản phát hành mới nhất trên GitHub và so mã SHA-256 với `SHA256SUMS`. Sau đó nó cài nginx 1.28.3 cùng mô-đun pagespeed, rồi cài panel ở cổng 2610 với chứng chỉ tự ký. Mô-đun ModSecurity chỉ được cài khi bạn bật tường lửa ứng dụng web trong tab Bảo mật. Bộ cài **không** cài PHP, Node.js hay cơ sở dữ liệu: bạn chọn những thứ đó sau, trong trang Dịch vụ của panel.

Cài xong, mở địa chỉ được in ra (`https://<IP>:2610/setup`) để tạo tài khoản quản trị. Hãy làm ngay, vì khi chưa có tài khoản, ai mở `/setup` trước sẽ là người tạo.

Tùy chọn: `MPN_VERSION=1.0.2` để cài một bản cố định, `MPN_PANEL_PORT=2610` để đổi cổng panel.

**Hệ điều hành:** Ubuntu 22.04, 24.04, 26.04 và Debian 12, 13, kiến trúc x86_64 hoặc arm64 (aarch64).

## Tính năng

- **Website:** PHP 7.0 đến 8.5 (mỗi site một pool PHP-FPM), Node.js 18 đến 24, Python, site tĩnh và proxy ngược. Mỗi site có tài khoản hệ thống riêng, tên miền, SSH/SFTP, trình quản lý tệp, terminal trên web, cron, nhật ký và vHost tự sửa (kiểm bằng `nginx -t` trước khi nạp).
- **Cài ứng dụng:** WordPress, Laravel, CodeIgniter 4, CakePHP, Yii 2, kèm trang quản lý WordPress (cập nhật, plugin, giao diện).
- **SSL:** Let's Encrypt có kiểm tra DNS trước và tự gia hạn, chứng chỉ riêng, tạo CSR. Panel cũng có chứng chỉ riêng cho chính nó.
- **Cơ sở dữ liệu:** MariaDB hoặc MySQL, mỗi cơ sở dữ liệu một người dùng, phpMyAdmin đăng nhập bằng tài khoản tạm.
- **Email** (mặc định tắt): Postfix, Dovecot, OpenDKIM. Tên miền, hộp thư và bí danh quản lý trong từng website, hiện sẵn bản ghi MX/SPF/DKIM/DMARC và có thể thêm thẳng vào Cloudflare. Web mail [mpn-mail](https://github.com/marixdev/mpn-mail) tự thoát khi không ai dùng.
- **Sao lưu:** hằng đêm, giữ 7 bản, khôi phục từ giao diện, đồng bộ lên Google Drive.
- **Bảo mật:** tường lửa nftables không khóa nhầm SSH và cổng panel, ModSecurity với OWASP CRS, fail2ban, xác thực hai bước (TOTP).
- **Giám sát:** CPU, bộ nhớ, tải, mạng, ổ đĩa và trạng thái dịch vụ trong 24 giờ.
- **Giao diện** tiếng Việt, tiếng Anh và tiếng Trung giản thể, sáng và tối.

## Cách hoạt động

- `mpn web` phục vụ giao diện bằng một người dùng không có quyền root, chỉ nghe `127.0.0.1` và đứng sau nginx có TLS.
- `mpn root` là trình hỗ trợ chạy bằng root, nghe trên socket Unix và chỉ nhận một danh sách thao tác cố định. Mỗi giá trị được kiểm tra lại trước khi thành đối số dòng lệnh, không qua shell.
- Mật khẩu, khóa riêng và chứng chỉ đi qua đầu vào chuẩn, không nằm trong đối số, log hay cơ sở dữ liệu.
- Mọi thay đổi trên máy là một kế hoạch gồm nhiều bước, mặc định chỉ xem thử và chạy thật khi có `-apply`. Nếu `nginx -t` hoặc một bước lỗi, tệp cũ được khôi phục.
- nginx được ghim ở 1.28.3 trên mọi hệ điều hành (từ nginx.org, `apt-mark hold`), nên `apt upgrade` không làm hỏng các mô-đun đã build cho bản đó ([mpn-nginx-modules](https://github.com/marixdev/mpn-nginx-modules)). Mọi tệp tải về đều được so với mã SHA-256 ghi sẵn trong mã nguồn.
- Dữ liệu của panel nằm trong SQLite, `/var/lib/mpn/mpn.db` quyền 0600.

## Các repo đi kèm

Panel tải các thành phần sau từ bản phát hành của chúng và so mã SHA-256 đã ghim trong mã nguồn:

| Repo | Panel dùng để | Ghim ở |
|---|---|---|
| [marixdev/mpn-mail](https://github.com/marixdev/mpn-mail) | Web mail ở `/webmail/` (bản tùy biến của lilmail) | `internal/mail/webmail.go` |
| [marixdev/mpn-nginx-modules](https://github.com/marixdev/mpn-nginx-modules) | Mô-đun PageSpeed và ModSecurity build cho nginx 1.28.3 | `internal/installer/nginx.go` |

## Dòng lệnh

```
mpn version | tune | plan | site | db | backup | cron | firewall | security | panel | cloud | app | wp | mail | services | root | web
```

Thêm `-h` sau lệnh con để xem các cờ. Lệnh thay đổi máy mặc định chỉ xem thử và cần `-apply`. Quên mật khẩu: `mpn web passwd`. Mất ứng dụng xác thực: `mpn web 2fa-off`.

| Thứ | Nơi |
|---|---|
| Tệp chạy | `/usr/local/bin/mpn` |
| Cơ sở dữ liệu của panel | `/var/lib/mpn/mpn.db` |
| Chứng chỉ của từng site | `/etc/mpn/certs/<site>/` |
| Log nginx của từng site | `/var/log/mpn/sites/<site>/` |
| Bản sao lưu | `/var/backups/mpn/<site>/<thời điểm>/` |
| Dịch vụ | `mpn-root`, `mpn-web`, `mpn-renew.timer`, `mpn-backup.timer` |

## Build từ mã nguồn

Cần Go 1.27 trở lên:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o mpn ./cmd/mpn
```

Bản phát hành được build bằng GitHub Actions khi đẩy tag `v*` ([release.yml](.github/workflows/release.yml)). Sau khi sửa template trong `internal/web`, dựng lại CSS bằng Tailwind v4: `tailwindcss -i assets/input.css -o assets/app.css --minify`. Trình soạn mã CodeMirror được dựng trong `tools/editor`.

## Giấy phép

Mini Panel phát hành theo [GNU General Public License v3.0](LICENSE). Thư viện bên thứ ba và giấy phép của chúng được liệt kê trong [NOTICE](NOTICE).
