# Nhật ký thay đổi

Mọi thay đổi đáng kể của Mini Panel được ghi ở đây. Số phiên bản theo [Semantic Versioning](https://semver.org/lang/vi/).

Bản chính bằng tiếng Anh: [CHANGELOG.md](CHANGELOG.md). Có thêm bản [tiếng Trung](CHANGELOG.zh.md).

## [1.2.4] - 2026-11-25

### Thêm mới

- **Thông báo cập nhật.** Panel tự kiểm tra bản mới 30 giây sau khi khởi động, sau đó mỗi 6 giờ, và giữ kết quả qua các lần khởi động lại. Khi có bản mới, trang Cập nhật hiện nhật ký thay đổi của mọi bản sau bản đang chạy, theo ngôn ngữ của panel, kèm nút cài đặt; các trang khác hiện một thông báo ở góc dưới bên phải dẫn tới đó. Khi chương trình đã chạy bản mới, thông báo tự biến mất. Nút × ẩn thông báo trên trình duyệt đó cho đến bản tiếp theo. "Kiểm tra ngay" hỏi GitHub tức thì.
- **PHP 5.6**, cho các site cũ chưa chuyển được. Trên Debian và Ubuntu lấy từ Sury (`php5.6-*`, Sury vẫn vá); trên AlmaLinux, Rocky Linux và CentOS Stream 9, 10 lấy từ [mpn-packages](https://github.com/marixdev/mpn-packages/releases/tag/el-r3), build kèm OpenSSL 1.0.2u liên kết riêng. Có mysqli, PDO và các hàm `mysql_*` cũ, opcache, gd, curl, mbstring, xml và zip. Bản thân PHP không còn vá bảo mật cho 5.6 từ tháng 1/2019: chỉ dùng cho site không chạy được trên 7.4 trở lên.
- **Truy cập cơ sở dữ liệu từ xa.** Một cơ sở dữ liệu có thể được kết nối từ các địa chỉ chọn trước qua cổng 3306 (Cơ sở dữ liệu › Truy cập từ xa). Máy chủ chỉ nghe mọi địa chỉ khi có ít nhất một cơ sở dữ liệu cho phép; tường lửa chỉ mở 3306 cho đúng các địa chỉ đó, và user tạo cho chúng chỉ có quyền trên cơ sở dữ liệu đó. Đổi mật khẩu thì đổi cho mọi địa chỉ. Lệnh `mpn db remote` làm tương tự qua SSH.
- **Cấu hình máy chủ cơ sở dữ liệu.** Dịch vụ › Cơ sở dữ liệu › Cấu hình đổi `max_connections`, buffer pool, kích thước gói, thời gian chờ và vài giá trị khác, mỗi giá trị trong một khoảng được kiểm tra. Máy chủ khởi động lại với tệp mới; nếu không lên được, tệp cũ được đặt lại và máy chủ chạy lại với tệp đó.
- **SSL qua DNS Cloudflare.** Site nằm sau proxy Cloudflare (mây cam), hoặc tên miền chưa trỏ về máy này, có thể lấy chứng chỉ bằng bản ghi TXT: nhập API token có quyền Zone:Read và DNS:Edit ở tab SSL. Token được giữ để gia hạn, chỉ root đọc được.
- **Ghi nhớ trình duyệt.** Trang đăng nhập có thể giữ một trình duyệt đăng nhập 30 ngày. Phiên đăng nhập giờ còn sau khi panel khởi động lại hay cập nhật. Trang Cài đặt hiện số trình duyệt đang đăng nhập và đăng xuất tất cả trình duyệt khác; đổi mật khẩu đăng xuất mọi trình duyệt, kể cả bằng `mpn web passwd`.

### Sửa lỗi

- Cài đặt dừng ở bước swap với "fallocate failed: Text file busy" trên máy mà nhà cung cấp đã tạo sẵn `/swapfile` và đang dùng nó. Swap có sẵn giờ được giữ nguyên, phần còn thiếu nằm trong tệp riêng `/swapfile-mpn`. Hệ tệp không hỗ trợ fallocate thì tệp được tạo bằng dd.
- Gia hạn chứng chỉ lỗi với site sau Cloudflare chế độ "Full" có "Always use HTTPS": Let's Encrypt đi theo chuyển hướng sang https, nơi site chuyển mọi đường dẫn đi chỗ khác, kể cả đường dẫn xác minh. Máy chủ https giờ tự trả lời `/.well-known/acme-challenge/`, và bước kiểm tra trước khi xin chứng chỉ đi theo chuyển hướng cùng tên miền như Let's Encrypt.
- Trình cài đặt phpBB lỗi ở cuối với "The ../styles/prosilver/template directory does not exist": panel xoá `/install/` ngay khi có tài khoản quản trị, trong lúc trình cài đặt vẫn đang chạy từ thư mục đó. Giờ panel chờ trình cài đặt chạy xong.
- Trong trình sửa tệp, Ctrl+A ngoài vùng văn bản và sao chép từ menu chỉ lấy các dòng đang hiện trên màn hình. Giờ lấy cả tệp.

### Ghi chú khi nâng cấp

- Lần kiểm tra bản mới đầu tiên diễn ra 30 giây sau khi panel khởi động; trước đó trang Cập nhật ghi là chưa kiểm tra.

## [1.2.3] - 2026-11-22

### Thêm mới

- **Nhật ký hoạt động.** Trang Nhật ký mới ghi lại những gì đã làm qua panel: đăng nhập, website, tệp, cơ sở dữ liệu, SSL, sao lưu, tường lửa và bảo mật, kèm thời gian, địa chỉ IP, website và kết quả. Mỗi website có tab Nhật ký riêng. Lọc theo website, loại, địa chỉ và ngày; xuất CSV tối đa 1000 dòng. Nhật ký giữ 90 ngày và chỉ tiến trình root ghi, nên không sửa hay xóa được qua panel. Mật khẩu, khóa, chứng chỉ, nội dung tệp và những gì gõ trong terminal không bao giờ được ghi. Xem [Nhật ký hoạt động](https://mpanel.biz/vi/activity/).
- **Cảnh báo Telegram.** Trang Telegram mới gửi tin khi một dịch vụ ngừng, website không trả lời, CPU, RAM hoặc ổ đĩa vượt ngưỡng, có người đăng nhập từ IP mới, fail2ban chặn một địa chỉ, sao lưu lỗi, chứng chỉ sắp hết hạn, hoặc có thay đổi được đánh dấu Quan trọng trong nhật ký. Một panel làm máy trung tâm và giữ bot; các máy khác ghép nối bằng mã dùng một lần và báo về mỗi phút, máy nào im lặng 3 phút sẽ bị báo. Cảnh báo chạy trong dịch vụ riêng `mpn-notify`, nên vẫn hoạt động khi panel khởi động lại hay cập nhật. Xem [Cảnh báo Telegram](https://mpanel.biz/vi/notify/).
- **Cổng dịch vụ.** Thẻ mới trên trang Tường lửa chuyển SSH, panel, phpMyAdmin hoặc webmail sang cổng khác. SSH và panel đổi qua hai bước: cổng mới mở cạnh cổng cũ, cổng cũ chỉ đóng sau khi bạn xác nhận cổng mới dùng được; không xác nhận thì sau 10 phút hệ thống tự trả về cổng cũ. Luật tường lửa và jail fail2ban cập nhật theo. Lệnh `mpn port status|set|finish|cancel` làm việc tương tự qua SSH. Xem [Tường lửa › Cổng dịch vụ](https://mpanel.biz/vi/firewall/#cổng-dịch-vụ).
- **phpBB.** Chọn được phpBB 3.3.19 làm mã nguồn cho website PHP mới. Panel tải bản chính thức, kiểm SHA-256, tạo cơ sở dữ liệu và viết cấu hình nginx riêng cho phpBB (định tuyến `app.php`, PATH_INFO, chặn các thư mục nội bộ). Bạn hoàn tất ở trình cài đặt `/install/` của phpBB với thông tin cơ sở dữ liệu ở trang kết quả. Khi đã có tài khoản quản trị, panel đóng `/install/` và xóa thư mục này trong vòng một phút.

### Sửa lỗi

- Terminal và Tệp của mọi website không chạy trên máy có SSH nghe cổng khác 22, như một số nhà cung cấp đặt sẵn. Panel luôn nối tới `127.0.0.1:22`. Giờ panel hỏi tiến trình root cổng mà sshd đang nghe, nhớ kết quả trong một phút, và hỏi lại khi kết nối bị từ chối, nên cổng đổi sau này cũng được nhận mà không cần khởi động lại.
- Trên Ubuntu 24.04 trở lên, sshd chỉ được `ssh.socket` khởi động khi có kết nối tới, nên tường lửa không đọc được cổng SSH và mở cổng 22 thay cho cổng thật. Máy có SSH ở cổng khác 22 bị khóa mất SSH. Giờ panel đọc cả cổng của `ssh.socket`, và chỉ khi socket đang chạy: systemd vẫn in địa chỉ của socket đang tắt, nên trên Debian cổng cũ (22) vẫn hiện và vẫn mở cạnh cổng mới sau khi đổi.
- Trên AlmaLinux, Rocky Linux và CentOS Stream, bước SELinux luôn gán nhãn cổng 2612 cho webmail dù panel dùng cổng nào. Nếu cổng panel khác 2610, nginx không nghe được cổng của webmail. Nhãn giờ đi theo cổng panel, và khi bật webmail cổng của nó được gán nhãn lại.

### Thay đổi

- Ubuntu 26.04 giờ lấy nginx 1.30.7 từ nginx.org như mọi hệ khác, cùng module PageSpeed và ModSecurity của [mpn-packages](https://github.com/marixdev/mpn-packages/releases/tag/nginx-1.30.7-r2). Trước đây máy giữ nginx 1.28.3 của Ubuntu và panel khoá nó bằng `apt-mark hold`. Gói đã khoá không nhận bản cập nhật bảo mật, nên máy cài trước ngày 18/11 vẫn chưa có bản vá CVE-2026-123007.

### Ghi chú khi nâng cấp

- Trên Ubuntu 26.04, `mpn update` thay nginx của Ubuntu, `nginx-module-pagespeed` (We-Amp) và `libnginx-mod-http-modsecurity` bằng nginx 1.30.7 từ nginx.org cùng các module build cho bản đó, trong một lần chạy apt. Các site và cài đặt của panel trong `/etc/nginx` được giữ nguyên. Nếu nginx mới không qua `nginx -t`, các gói và tệp apt cũ được đặt lại và việc cập nhật dừng; tệp trước khi đổi nằm ở `/var/backups/mpn-nginx-upgrade/old`.

## [1.2.2] - 2026-11-19

### Bảo mật

- nginx lên 1.30.7 từ 1.28.3, vá các lỗ hổng nginx công bố ngày 18/11/2026, trong đó có CVE-2026-123007 (hỏng bộ nhớ trong điều khiển luồng HTTP/2, khai thác được mà không cần đăng nhập trên mọi site bật HTTP/2) và CVE-2026-118899. Nhánh 1.28 không có bản vá. Module PageSpeed và ModSecurity được build lại cho 1.30.7 ([nginx-1.30.7-r1](https://github.com/marixdev/mpn-packages/releases/tag/nginx-1.30.7-r1), [el-r2](https://github.com/marixdev/mpn-packages/releases/tag/el-r2)) và vẫn được kiểm SHA-256 như trước.

### Sửa

- `mpn update` khôi phục bản cũ trên máy chưa tạo tài khoản quản trị, vì ở đó `/login` chuyển sang `/setup` và bước kiểm tra tưởng panel không trả lời. Bản sửa có tác dụng khi cập nhật từ 1.2.2 trở đi.
- Phông chữ của chính panel (JetBrains Mono) bị chính sách bảo mật nội dung (CSP) chặn trên mọi trang.

### Ghi chú khi nâng cấp

- `mpn update` (và trang Cập nhật) đưa nginx của máy cài bằng bản cũ lên 1.30.7, cùng các module máy đang nạp. Cấu hình và module cũ được giữ ở `/var/backups/mpn-nginx-upgrade/old`. Nếu nginx mới không qua `nginx -t`, gói, module và chỗ ghim cũ được đặt lại và việc cập nhật dừng. Ubuntu 26.04 giữ nginx của bản phân phối.

## [1.2.1] - 2026-11-18

### Sửa

- Các trang của panel chậm hơn ở bản 1.2.0, vì rclone làm chương trình `mpn` nặng 109 MB và mỗi lần chạy lâu thêm khoảng 100 ms, trong khi panel chạy `mpn` vài lần cho mỗi trang. rclone giờ nằm trong một chương trình thứ hai, `/usr/local/bin/mpn-cloud` (cùng mã nguồn, build kèm rclone), và `mpn` chỉ gọi nó khi kết nối kho lưu trữ hoặc chép, liệt kê, tải bản sao lưu. `mpn` trở lại 19 MB và khởi động trong 0,02 s thay vì 0,18 s trên máy 1 vCPU.
- Danh sách kho lưu trữ ngoài máy chủ hiện mã dịch như `cld.kind.azureblob` cho các kho sinh từ danh sách của rclone. Giờ mỗi kho có tên, mô tả và gợi ý, kèm ô tìm kiếm để lọc danh sách.

### Đổi

- Cập nhật có trang riêng, mục **Cập nhật** trên menu chính, thay cho một phần của trang Cài đặt.
- Panel và trang tài liệu mặc định bằng tiếng Anh. Trình duyệt chọn tiếng Việt hay tiếng Trung thì nhận ngôn ngữ đó, và vẫn đổi được bằng menu ngôn ngữ. Địa chỉ tài liệu cũ dưới `/en/` dẫn sang địa chỉ mới.

### Lưu ý khi nâng cấp

- `mpn update`, nút Cập nhật của panel và bộ cài tải cả hai chương trình. Máy cập nhật bằng bản 1.2.0 (chỉ biết `mpn`) sẽ nhận `mpn-cloud` khi `mpn` mới chạy `mpn panel migrate -apply` ở cuối lần cập nhật.
## [1.2.0] - 2026-11-18

### Thêm

- Cập nhật panel từ giao diện (Cài đặt, mục Cập nhật) hoặc bằng `mpn update -apply`. `SHA256SUMS` của mỗi bản phát hành được ký bằng minisign, và panel chỉ nhận bản có chữ ký đúng với khoá dựng sẵn trong chương trình. Phần chú thích có ký ghi số phiên bản, nên không ai đem được bản cũ ra giả làm bản mới. Bản đang chạy được giữ ở `/usr/local/bin/mpn.prev`, và nếu panel không trả lời sau khi cập nhật thì bản đó được đặt lại.
- Chép bản sao lưu ra ngoài máy chủ, giờ dùng rclone: Google Drive, S3 (Cloudflare R2, AWS, Wasabi…), Backblaze B2, SFTP, WebDAV (Nextcloud, ownCloud) và FTP/FTPS. Kết nối chỉ được lưu khi panel đã ghi, đọc lại và xoá được một tệp thử. Biểu mẫu chỉ chuyển cho rclone những trường có trong danh sách cố định, nên không có tuỳ chọn nào chạy được lệnh hay đọc tệp của máy.
- Lựa chọn lấy địa chỉ thật của khách khi site đi qua Cloudflare (Bảo mật, hoặc `mpn security cloudflare -on -apply`). nginx lấy địa chỉ trong `CF-Connecting-IP`, nhưng chỉ trên kết nối đến từ dải mạng Cloudflare công bố. Mặc định tắt.
- Cài MariaDB hoặc MySQL thì phpMyAdmin được cài kèm, để tab Cơ sở dữ liệu mở được ngay.
- `mpn panel migrate` đưa máy cài bằng bản cũ về đúng cấu hình của bản cài mới. Bộ cài và `mpn update` tự chạy lệnh này.

### Đổi

- Panel nghe trên socket Unix `/run/mpn-web/web.sock` do systemd tạo và giữ (`mpn-web.socket`), không còn nghe `127.0.0.1:8780`. Chỉ nginx và phpMyAdmin (nhóm `mpn-front`) kết nối được. Socket vẫn còn khi panel khởi động lại, nên không chương trình nào chiếm được chỗ của panel. Địa chỉ khách trong `X-Real-IP` chỉ được tin khi panel nghe trên socket đó.
- Site Node.js và Python mới (Debian, Ubuntu) nghe trên socket Unix `/run/mpn-app/<site>/app.sock`, không mở cổng TCP. Chỉ nginx kết nối được. Đường dẫn socket nằm trong biến `SOCKET`, và `PORT` cũng mang đúng đường dẫn đó, vì `listen(process.env.PORT)` của Node.js nhận được đường dẫn. Để trống ô cổng nghĩa là dùng socket, còn nhập số cổng thì app nghe `127.0.0.1` như trước. Site đã có giữ nguyên cổng. Trên họ RHEL, app vẫn dùng cổng.
- Trang thiết lập `/setup` chỉ mở bằng liên kết có khoá dùng một lần. Bộ cài in liên kết này, lời chào khi SSH vào máy cũng hiện nó cho tới khi có tài khoản, và `mpn web setup-link` in lại khi cần. Trước đây ai mở `/setup` trước thì người đó tạo tài khoản.
- Node.js chỉ được cài khi `SHASUMS256.txt` có chữ ký hợp lệ của người phát hành Node.js. Chữ ký được kiểm bằng `gpgv` với 29 khoá dựng sẵn trong mpn, nên nodejs.org hay một bản sao của nó không tự chọn được khoá. `gpgv` có trong bước gói cơ bản.
- MANIFEST của bản sao lưu có SHA-256 của từng tệp và một HMAC ký bằng khoá riêng của máy (`/etc/mpn/backup.key`, quyền 0600). Bản tải về từ kho ngoài phải có chữ ký của chính máy này, và mỗi tệp phải khớp tổng của nó.
- vHost viết tay không được ghi log ra ngoài `/var/log/mpn/sites/<site>/`. Các chỉ thị đọc tệp hay ảnh hưởng tới cả máy (`include`, `load_module`, `ssl_certificate` ngoài thư mục chứng chỉ của site, `*_temp_path`, `*_cache_path`, `user`, `pid`…) bị từ chối. Lý do là nginx mở tệp log bằng quyền root trước khi đổi sang tài khoản worker.
- Laravel ghi `DB_HOST=localhost` (qua socket Unix) như WordPress và CodeIgniter.
- Mã QR của xác thực hai bước được vẽ bằng mã riêng của panel, thay thư viện go-qrcode đã ngừng phát triển.
- Lệnh và thao tác chạy bằng root tạo thư mục trong home của site qua mô tả thư mục đã mở và không theo liên kết tượng trưng, nên tài khoản site không dùng symlink đưa root sang tệp khác được.
- Không tạo được cơ sở dữ liệu có tên thuộc về site khác rõ hơn (ví dụ `blog_shop` khi có site `blog-shop`).
- Khi quá nhiều lượt đăng nhập cùng chờ băm mật khẩu, panel trả lời 429 ngay thay vì xếp hàng, để một đợt dò mật khẩu không chiếm hết CPU.
- Mã nguồn công khai có kèm test, và GitHub Actions chạy gofmt, vet, test và govulncheck ở mỗi lần đẩy.

### Lưu ý khi nâng cấp

- Bản sao lưu làm bằng 1.1.x chưa có chữ ký, nên không tải về từ kho ngoài được nữa. Bản sao lưu nằm trên máy vẫn khôi phục được như cũ.
- Site có vHost viết tay đang `include` tệp khác hoặc ghi log ra chỗ khác sẽ báo lỗi ở lần áp dụng tiếp theo, cho tới khi sửa phần đó.

## [1.1.1] - 2026-11-14

### Sửa

- Sao lưu giờ chép cả thư mục `htdocs` khi thư mục gốc web nằm bên dưới nó, ví dụ `htdocs/public` của Laravel, Flarum hay Symfony. Trước đây bản sao lưu chỉ có `public/` và thiếu `config.php`/`.env`, `storage` (tệp tải lên) và `vendor`, nên khôi phục từ đó không chạy lại được site. Site có thư mục gốc là `htdocs` không đổi gì.
- MANIFEST của các bản sao lưu đó có thêm dòng `files_root=htdocs`, và khôi phục dựa vào dòng này: bản sao lưu cũ (chỉ có thư mục gốc web) vẫn được bung về thư mục gốc web như trước. mpn 1.1.0 trở về trước không đọc được MANIFEST mới, nên hãy nâng cấp trước khi khôi phục các bản sao lưu mới.

## [1.1.0] - 2026-11-13

### Thêm

- Chạy trên AlmaLinux, Rocky Linux và CentOS Stream 9 và 10, cả x86_64 lẫn arm64. Nginx 1.28.3 lấy từ nginx.org và khoá bằng `dnf versionlock`; PHP 7.4 trở lên từ Remi (kèm EPEL); PHP 7.0 đến 7.3, module ModSecurity và OWASP CRS 4.25.2 lấy từ bản [el-r1](https://github.com/marixdev/mpn-packages/releases/tag/el-r1) của mpn-packages; PageSpeed từ bản [nginx-1.28.3-r3](https://github.com/marixdev/mpn-packages/releases/tag/nginx-1.28.3-r3). Mọi tệp tải về đều được kiểm SHA-256 đã ghim.
- MariaDB lấy từ rpm.mariadb.org và MySQL từ repo.mysql.com; khoá ký của hai kho được kiểm theo fingerprint. EL10 chỉ có MySQL 8.4.
- SELinux vẫn bật: panel gán nhãn cho thư mục website, log, socket PHP-FPM, venv của site Python, Node.js, hộp thư và cổng của panel, và bật `httpd_can_network_connect` để site gọi được cơ sở dữ liệu và mạng ngoài.
- Bộ cài tắt firewalld, vì panel quản lý tường lửa bằng nftables.
- Mail trên họ RHEL: mật khẩu hộp thư băm bằng bcrypt (`BLF-CRYPT`), vì Dovecot ở đó không có argon2; OpenDKIM nghe ở cổng 8891 trên loopback, vì chính sách SELinux không cho Postfix nối vào socket của nó.

### Đổi

- Module nginx giờ nằm ở repo mpn-packages (trước là mpn-nginx-modules; địa chỉ cũ vẫn chuyển tiếp tới đây).

## [1.0.3] - 2026-11-13

### Thêm

- Chạy trên máy ARM (arm64, aarch64). Bộ cài nhận máy aarch64 và tải `mpn_linux_arm64`. Module PageSpeed và ModSecurity cho arm64 lấy từ bản [nginx-1.28.3-r2](https://github.com/marixdev/mpn-packages/releases/tag/nginx-1.28.3-r2) (build trên runner ARM của GitHub, ghim SHA-256 theo từng hệ điều hành và kiến trúc). Web mail lên [mpn-mail 1.0.2](https://github.com/marixdev/mpn-mail/releases/tag/v1.0.2), có bản `mpn-mail_linux_arm64`.
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
- README có thêm bản tiếng Anh và tiếng Trung ([README.zh.md](README.zh.md)), kèm bảng các repo đi kèm (mpn-mail, mpn-nginx-modules).

## [1.0.0] - 2026-11-12

Bản phát hành đầu tiên: website PHP, Node.js, Python, tĩnh và proxy; SSL Let's Encrypt; MariaDB/MySQL và phpMyAdmin; email và web mail mpn-mail 1.0.0; sao lưu; tường lửa, ModSecurity, fail2ban, 2FA; giám sát; giao diện tiếng Việt và tiếng Anh.

[1.2.0]: https://github.com/marixdev/mini-panel/compare/v1.1.1...v1.2.0
[1.1.1]: https://github.com/marixdev/mini-panel/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/marixdev/mini-panel/compare/v1.0.3...v1.1.0
[1.0.3]: https://github.com/marixdev/mini-panel/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/marixdev/mini-panel/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/marixdev/mini-panel/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/marixdev/mini-panel/releases/tag/v1.0.0
