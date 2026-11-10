# Mini Panel (`mpn`)

Bảng điều khiển hosting nhẹ, viết bằng Go, cho VPS RAM thấp (mục tiêu 1 vCPU / 1 GB). Một tệp chạy tĩnh duy nhất cài sẵn nginx, PHP-FPM nhiều phiên bản, Node.js, MariaDB hoặc MySQL, rồi quản lý website, cơ sở dữ liệu, SSL và sao lưu qua một giao diện web tiếng Việt.

Dự án được viết từ đầu theo yêu cầu chức năng. Nó không chứa mã, mẫu cấu hình hay giao diện của sản phẩm nào khác. Chi tiết nằm trong [PLAN.md](PLAN.md).

> Trạng thái: đang phát triển, chưa phát hành bản nào và chưa chọn giấy phép. Đừng đưa lên máy chủ đang chạy thật.

## Có gì

**Loại website:** PHP (7.0 đến 8.5, mỗi site một pool PHP-FPM chạy theo yêu cầu), Node.js (18, 20, 22, 24), Python (python3 của hệ thống, mỗi site một môi trường ảo), tĩnh, và proxy ngược. Node và Python chạy như dịch vụ systemd riêng, có giới hạn bộ nhớ.

**Mỗi website:**
- Một tài khoản hệ thống riêng, một tên miền, tùy chọn www chuyển hướng.
- Tab Tổng quan: trạng thái, dung lượng, tên miền (bố trí ngang), thư mục web con, cài đặt theo loại site (PHP chỉ đổi được các tham số trong một danh sách an toàn cố định), đăng nhập SSH/SFTP, tăng tốc website và xóa website. Danh sách website chỉ có nút Quản lý để vào trang của từng site; cơ sở dữ liệu và sao lưu nằm trong tab của site.
- Tăng tốc website: bật hoặc tắt mô-đun nginx mod_pagespeed ([We-Amp/mod_pagespeed](https://github.com/We-Amp/mod_pagespeed)), tự thu nhỏ HTML/CSS/JS và tối ưu ảnh ngay trên máy chủ. Không dùng được khi site đang dùng vHost tự sửa.
- Tab SSL: Let's Encrypt (kiểm tra DNS trước, tự gia hạn), chứng chỉ riêng đã mua (kiểm tra khóa, chuỗi, tên được phủ), tạo CSR, chuyển hướng http sang https.
- Tab vHost: sửa tay cấu hình nginx của site. Panel kiểm tra bằng `nginx -t` trước khi nạp, lỗi thì giữ nguyên cấu hình đang chạy, có nút về cấu hình mặc định. Khi site dùng cấu hình tự sửa, các thao tác ghi đè vhost (đổi tên miền, SSL, chuyển hướng) bị từ chối với thông báo rõ.
- Tab Cron: công việc định kỳ cho từng site, chạy bằng tài khoản của site (không phải root), ghi vào `/etc/cron.d/mpn-<site>` với quyền 0600. Lịch và lệnh được kiểm tra chặt để không chèn được dòng vào tệp của root.
- Tường lửa (nftables): bảng `inet mpn` riêng, không đụng luật của chương trình khác. Cổng SSH và cổng panel luôn mở và không xóa được, luật chặn chính người đang thao tác bị từ chối, từ chối bật khi ufw hoặc firewalld đang chạy. Nạp luật nguyên tử và tự nạp lại khi khởi động.
- Bảo mật (tab riêng): tường lửa ứng dụng web ModSecurity với bộ luật OWASP CRS, và fail2ban. Cả hai cài khi bạn bật, không có sẵn trong bản cài mới vì ModSecurity tốn thêm RAM.
  - ModSecurity nạp một lần ở cấp `http` của nginx cho mọi website, có ba chế độ: tắt, chỉ ghi nhận (ghi vào `/var/log/mpn/modsec_audit.log`, xoay vòng 7 ngày, không chặn) và chặn (trả 403, ghi thêm vào `error.log` của website để fail2ban đọc). Nên chạy chế độ ghi nhận vài ngày trước khi chặn. Panel và phpMyAdmin được miễn qua `/etc/mpn/modsec/off.d/`.
  - fail2ban có ba jail do mpn quản lý: `sshd`, `mpn-panel` (đăng nhập panel sai, panel ghi dòng `login failed from <IP>` vào journal) và `mpn-modsec` (địa chỉ bị bộ luật OWASP chặn nhiều lần). Khóa 1 giờ bằng nftables trong bảng riêng của fail2ban, không đụng bảng `inet mpn`. Địa chỉ của người đang thao tác luôn được thêm vào danh sách không bao giờ bị khóa, và trang có nút bỏ chặn từng địa chỉ.
- Chứng chỉ SSL cho chính panel (thẻ đầu tab Bảo mật): cấp bằng Let's Encrypt (kiểm tra DNS trước, tự gia hạn cùng timer với các website) hoặc dán chứng chỉ riêng, có nút quay về chứng chỉ tự ký. Panel (cổng 2610) và phpMyAdmin (cổng 2611) đọc chung `/etc/mpn/certs/_panel`, nên một chứng chỉ phủ cả hai cổng. Tên miền được ghi vào `/etc/mpn/panel-hosts` và panel đọc lại khi chạy, không cần khởi động lại. Cổng 80 chỉ mở một khối nginx cho đúng tên miền của panel để trả thử thách ACME và chuyển hướng sang cổng panel. Tên miền không được trùng với website nào.
- Giám sát: bộ xử lý, bộ nhớ, tải, mạng, ổ đĩa và trạng thái các dịch vụ. Panel tự đo mỗi 30 giây từ `/proc`, giữ 24 giờ (lưu ra `/var/lib/mpn-web/metrics.json` để không mất khi khởi động lại), vẽ biểu đồ SVG không cần thư viện, có chú giải khi rê chuột và bảng số liệu thay thế.
- Trình quản lý tệp qua SFTP (sửa bằng CodeMirror, nén và giải nén, đổi quyền), terminal trên web, SSH/SFTP bằng khóa riêng của panel.
- Giao diện: thanh điều hướng ngang một hàng (logo, các mục, tên người dùng), chữ thường bằng sans của hệ thống, số và mã bằng JetBrains Mono (ba tệp 62 KB nhúng sẵn trong chương trình, có đủ dấu tiếng Việt, không gọi ra dịch vụ phông nào). Màu theo token oklch hue 165, sáng và tối. IP và tài khoản của website là thẻ bấm để chép.
- Tab Nhật ký của từng website: những dòng cuối của log truy cập và log lỗi nginx, với website Node hoặc Python còn có log của chính ứng dụng (đọc từ journal của dịch vụ). Tìm theo chữ thường, chọn 100 đến 1000 dòng. Chỉ đọc; không đi theo liên kết tượng trưng, bỏ ký tự điều khiển, cắt dòng quá dài, chỉ tìm trong 2 MB cuối của tệp.
- Trang Dịch vụ: liệt kê nginx, máy chủ cơ sở dữ liệu, từng bản PHP-FPM, fail2ban, Postfix, Dovecot, OpenDKIM với trạng thái và nút Khởi động, Dừng, Khởi động lại, Nạp lại (chỉ nút mà dịch vụ đó có; Dừng có bước xác nhận; nginx không dừng được từ đây). Dịch vụ chưa cài mà installer hỗ trợ thì có nút Cài đặt: PHP 7.0 đến 8.5, Node.js 18, 20, 22, 24, MariaDB hoặc MySQL (khi máy chưa có), phpMyAdmin. Panel, SSH và tường lửa không nằm trong danh sách vì dừng chúng làm mất quyền truy cập. PHP dưới 7.4 không cài thêm được khi máy dùng MySQL 8 (cần thiết lập từ lúc cài máy chủ).
- Tài khoản quản trị: bấm tên người dùng ở góc phải để mở menu Cài đặt và Đăng xuất. Cài đặt có đổi mật khẩu (đăng xuất các trình duyệt khác) và xác thực hai bước bằng mã 6 số của ứng dụng xác thực (TOTP, RFC 6238): bật bằng mã QR hoặc khóa nhập tay và phải nhập đúng mã đầu tiên; tắt cần cả mật khẩu và mã hiện tại. Khi mất ứng dụng, chạy `mpn web 2fa-off` bằng root trên máy chủ. Mã đã dùng không dùng lại được, sai năm lần thì phải đăng nhập lại.
- Địa chỉ không kèm cổng: khi panel có tên miền riêng và chứng chỉ khớp tên (Let's Encrypt hoặc chứng chỉ riêng), nginx phục vụ tên đó ở cổng 443: panel ở `https://tên-miền`, phpMyAdmin ở `https://tên-miền/pma/`, cổng 80 chuyển thẳng về đó. Vào bằng IP thì vẫn dùng các cổng 2610 và 2611. Dưới `/pma/` phpMyAdmin cùng origin với panel, nên trang của nó có header giới hạn chỉ được gọi tới `/pma/`, không bị nhúng khung và không truy cập được từ cửa sổ của panel. `mpn panel sync` dựng lại các khối nginx này theo trạng thái hiện tại và các lệnh đổi chứng chỉ, bật web mail, cài phpMyAdmin tự gọi nó.
- Cổng của panel không bị website lấy: website Node và Python không chọn được các cổng của panel (2610, 2611, 2612, 8780, 8781) và bất kỳ cổng nào mà nginx của mpn đang nghe trên máy đó.
- Trang chào mặc định: website mới có sẵn một trang chuyên nghiệp để biết site đã chạy (`index.html` cho site tĩnh, `index.php` cho PHP, tệp khởi động cho Node và Python nếu là tên tệp đơn giản). Chỉ tạo khi thư mục web còn trống, không tạo cho site đã cài WordPress hay framework và cho proxy. Trang có dấu `mpn-default-page`, nhờ đó khi cài ứng dụng lên, panel chỉ xóa đúng trang này, không bao giờ xóa tệp của bạn.
- Cài mã nguồn cho site PHP (form tạo site và thẻ trong tab Tổng quan): WordPress, Laravel, CodeIgniter 4, CakePHP, Yii 2. Panel tải bản mới nhất, tạo cơ sở dữ liệu riêng cho WordPress, Laravel và CodeIgniter, ghi cấu hình phù hợp (`wp-config.php` hoặc `.env`, quyền 0600, chế độ production), đổi thư mục web sang `public`, `webroot` hoặc `web` khi framework cần và thêm tham số PHP thích hợp. Cần thư mục web trống và phiên bản PHP đủ mới (báo rõ nếu thiếu). WP-CLI và Composer được tải một lần, cố định phiên bản và kiểm tra checksum; mật khẩu chỉ đi qua tệp tạm quyền 0600 và đầu vào chuẩn, không nằm trong đối số của tiến trình. Cài lỗi thì dọn sạch về thư mục trống.
- Quản lý WordPress (tab riêng của site WordPress): xem phiên bản, cập nhật lõi, plugin và giao diện, tìm trên wordpress.org rồi cài, bật, tắt, xóa plugin và giao diện, mở nhanh trang quản trị. Lệnh chạy bằng WP-CLI với đúng tài khoản và phiên bản PHP của site; tên plugin hoặc giao diện được kiểm tra chặt trước khi thành đối số.
- Email (trang riêng, mặc định tắt, chỉ cài khi bạn bật): máy chủ thư gồm Postfix (nhận và gửi), Dovecot (hộp thư Maildir, IMAP) và OpenDKIM (ký thư). Ba dịch vụ chạy khoảng 45 đến 85 MB RAM; tắt thì dừng hẳn và vẫn giữ hộp thư, thư đã nhận, khóa DKIM. Hộp thư ảo nằm ở `/var/vmail`, mật khẩu băm argon2id trong `/etc/mpn/mail/users` (quyền 0640), không cần cơ sở dữ liệu cho thư. Mỗi tên miền có khóa DKIM riêng (2048 bit, quyền 0600 của `opendkim`) và panel hiện sẵn bản ghi MX, SPF, DKIM, DMARC cần thêm. Cổng 25 chỉ nhận thư cho tên miền và hộp thư có thật (từ chối ngay địa chỉ lạ), không bao giờ cho đăng nhập và không chuyển tiếp hộ ai. Gửi thư chỉ qua 587 (STARTTLS) hoặc 465 (TLS) và bắt buộc đăng nhập; IMAP ra ngoài chỉ qua 993, còn cổng 143 chỉ nghe trên loopback. Bật Email mở các cổng 25, 465, 587, 993 trong tường lửa của panel (nếu đang bật), tắt thì gỡ đúng các luật đó. Mật khẩu hộp thư chỉ đi qua đầu vào chuẩn. Chứng chỉ ban đầu là loại tự ký. Nhiều nhà cung cấp VPS (như Linode) chặn cổng 25 gửi ra, cần xin mở thì mới gửi được thư ra ngoài.
  - Chứng chỉ TLS của tên máy chủ thư: lúc đầu là chứng chỉ tự ký. Nút "Lấy chứng chỉ Let's Encrypt" kiểm tra DNS của tên đó, xin chứng chỉ qua nginx cổng 80 (khối nginx chỉ phục vụ thử thách, mọi đường dẫn khác trả 404), ghi vào `/etc/mpn/certs/_mail/` rồi tải lại Postfix và Dovecot. Gia hạn nằm cùng bộ hẹn giờ với chứng chỉ của website và của panel.
  - Web mail: chương trình Go nhỏ riêng ([mpn-mail](https://github.com/marixdev/mpn-mail), bản tùy biến từ lilmail) chạy bằng tài khoản `mpn-mail` gần như không có quyền, nghe ở `127.0.0.1:8781` qua socket của systemd, tự thoát sau 15 phút không ai dùng nên lúc rảnh không chiếm RAM (đo được khoảng 18 MB khi đang chạy). Truy cập qua nginx ở cổng kế tiếp cổng phpMyAdmin (mặc định 2612) và dùng chung chứng chỉ của panel; không đặt dưới đường dẫn con vì chương trình dùng liên kết tuyệt đối. Chương trình được tải một lần từ bản phát hành và so với mã SHA-256 ghi sẵn trong mã nguồn; tệp không khớp thì không được cài. Đăng nhập bằng địa chỉ email đầy đủ và mật khẩu hộp thư. Tắt máy chủ thư thì web mail cũng tắt, cài đặt của nó được giữ.
- Cơ sở dữ liệu theo site, mở phpMyAdmin bằng tài khoản tạm không lưu mật khẩu.
- Sao lưu hằng đêm (tệp và cơ sở dữ liệu), khôi phục từ giao diện.
- Đồng bộ sao lưu lên Google Drive (thẻ cuối tab Sao lưu của từng site): đăng nhập bằng OAuth kiểu thiết bị, bạn tự tạo Client ID và Client secret trong Google Cloud Console (loại "TVs and Limited Input devices") nên không cần mở trình duyệt trên máy chủ. Panel chỉ xin quyền `drive.file`, tức chỉ thấy các tệp do chính nó tạo. Mỗi site bật hoặc tắt riêng; kết nối dùng chung. Bản sao nằm ở `mpn-backups/<site>/<thời điểm>/` trong Drive, `MANIFEST` tải lên cuối cùng nên thư mục thiếu `MANIFEST` là bản dở và được làm lại. Số bản giữ trên Drive bằng số bản giữ trên máy (7), bản cũ hơn tự xóa và chỉ đụng vào thư mục có tên dạng thời điểm. Tải lên theo từng khối 8 MB, mất mạng giữa chừng chỉ tải lại một khối. Chạy tự động sau sao lưu hằng đêm (drop-in `mpn-backup.service.d/cloud.conf`, lỗi đồng bộ không làm hỏng việc sao lưu), hoặc bấm "Đồng bộ ngay" để chạy nền. Khôi phục từ Drive: nút "Xem các bản trên Drive" liệt kê các bản hoàn chỉnh và tải một bản về `/var/backups/mpn/<site>/` (chạy nền, tiếp tục từ chỗ đứt nếu mất mạng). Bản tải về chỉ xuất hiện trong danh sách sao lưu khi đã đủ: mỗi tệp khớp mã MD5 của Google, mỗi `.gz` đọc hết không lỗi, `MANIFEST` đúng website và thời điểm, cơ sở dữ liệu thuộc đúng website, và máy chủ còn đủ chỗ trống. Sau đó khôi phục bằng đúng luồng khôi phục thường (trang xác nhận, chọn tệp hoặc cơ sở dữ liệu). Client secret và token nằm trong `/etc/mpn/gdrive.json` quyền 0600, chỉ đi qua đầu vào chuẩn, không nằm trong đối số dòng lệnh.

**Hệ điều hành:** Ubuntu 22.04, 24.04, 26.04 và Debian 12, 13.

**Một bản nginx cố định:** panel luôn chạy nginx 1.28.3 trên mọi hệ điều hành. Mô-đun động của nginx chỉ nạp được vào đúng bản nginx đã dùng để biên dịch nó, mà mỗi bản phân phối đi kèm một bản nginx khác nhau (1.18 đến 1.28), nên panel lấy nginx 1.28.3 từ nginx.org (ghim bằng apt với ưu tiên 1001 và `apt-mark hold`, kiểm tra dấu tay khóa ký trước khi thêm kho) rồi nạp hai mô-đun pagespeed và ModSecurity được build riêng cho bản đó ([mpn-nginx-modules](https://github.com/marixdev/mpn-nginx-modules), tải về có so mã SHA-256 ghi sẵn trong mã nguồn). Riêng Ubuntu 26.04 vốn đã có nginx 1.28.3 cùng mô-đun đóng gói sẵn nên dùng luôn của bản phân phối, cũng được giữ phiên bản. Nhờ vậy `apt upgrade` không bao giờ làm nginx nhảy phiên bản và làm hỏng mô-đun.

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
mpn version | tune | plan | site | db | backup | cron | firewall | security | panel | cloud | app | wp | mail | services | root | web
mpn site add|list|show|delete|apply|set|info|log|domain|vhost|remove|access|ssl|ssl-custom|ssl-csr|ssl-off|ssl-redirect|ssl-info|ssl-renew
mpn db add|list|passwd|remove        mpn backup create|list|restore|prune
mpn security status|waf|f2b|unban    mpn panel ssl-info|ssl|ssl-custom|ssl-off|sync
mpn services status|control|install  mpn web passwd|2fa-off
mpn cloud status|connect|finish|disconnect|enable|sync|list|fetch
mpn app install                      mpn wp status|run|search
mpn mail status|enable|disable|ssl|webmail|records|domain-add|domain-del|box-add|box-del|box-passwd|box-quota|alias-set|alias-del
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
  security/    cấu hình ModSecurity (OWASP CRS) và fail2ban, kiểm tra giá trị
  panelssl/    chứng chỉ của chính panel: thư mục dùng chung, tên miền, khối nginx cho thử thách ACME
  gdrive/      client Google Drive chỉ dùng net/http: đăng nhập thiết bị, làm mới token, tải lên tiếp tục được
  cloudsync/   đồng bộ sao lưu của một site lên ổ đám mây, kết nối và khóa chạy một lần
  apps/        cài WordPress và framework PHP (công thức, kế hoạch cài, lệnh WP-CLI để quản lý)
  mail/        máy chủ thư: kiểm tra địa chỉ và mật khẩu, băm argon2id, khóa và bản ghi DKIM, cấu hình Postfix/Dovecot/OpenDKIM, kế hoạch cài và cập nhật
  sitelog/     đọc cuối log của website: kiểm tra giá trị, lọc, làm sạch dòng
  services/    danh mục dịch vụ panel quản lý và cài thêm, kiểm tra khóa và thao tác
  front/       quyết định khối nginx nào cho địa chỉ không kèm cổng (panel, /pma/, web mail)
  webauth/     băm mật khẩu, tài khoản, phiên đăng nhập, mã TOTP của xác thực hai bước
  dbadmin/ backup/ access/ panelkey/ panelssh/ tune/ ...
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

Xem log trong panel, nhiều người dùng và 2FA, sao lưu ra ngoài máy chủ, xoay vòng log PHP và Node.

## Giấy phép

Chưa chọn. Thư viện bên thứ ba được liệt kê trong [NOTICE](NOTICE).




