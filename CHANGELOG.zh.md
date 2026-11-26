# 更新日志

Mini Panel 的所有重要变更都记录在这里。版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。

本文译自 [CHANGELOG.md](CHANGELOG.md)（英文）。另有[越南语版](CHANGELOG.vi.md)。

## [1.2.5] - 2026-11-27

### 新增

- **网站用户。** 管理员可以创建只能访问指定网站的账户，每个网站一个角色：*view* 可查看网站的页面、文件、备份和活动；*manage* 还可修改文件、数据库、计划任务、后台服务、WordPress 和邮件；*full* 还可修改 SSL、vhost、SSH 和终端，并恢复备份。“用户与权限”（`/users`）页面可创建账户、设置网站和角色、重置密码、停用、删除或关闭两步验证。账户无法使用的页面和按钮会被隐藏，面板也会拒绝相应请求。网站用户有自己的两步验证，并可在“设置”中修改自己的密码；服务器设置仍只属于管理员。只有在管理员允许且该用户可管理拥有该数据库的网站时，网站用户才能使用 phpMyAdmin；已打开的 phpMyAdmin 窗口会继续可用，直到其临时数据库用户在打开 30 分钟后过期。“活动”只向网站用户显示其网站的事件。`mpn web user-off 名称` 可在 SSH 中停用账户。
- **后台服务。** PHP、Node.js 或 Python 网站最多可运行 10 个长期运行的命令（如队列 worker 或机器人），以网站自己的账户作为 systemd 服务运行（“后台服务”标签页，或 `mpn bg`）。每个服务可开启、关闭、重启并设置内存上限，标签页显示其是否在运行。网站的 PHP、Node.js 或 Python 版本变更时会重新写入这些服务，删除网站时一并删除。`mpn bg sync -site 名称`（或 `-all`）从数据库重新写入所有服务并删除多余的部分。
- **救援菜单。** 在终端运行 `sudo mpn` 会显示找回访问权限的命令：重置管理员密码、关闭两步验证、停用网站用户、生成新的初始设置链接、关闭面板证书、查看或取消端口变更、解封地址、关闭防火墙以及重启面板。每一项在修改前都会确认，并打印所运行的命令。在脚本或管道中，`mpn` 仍像以前一样打印用法。

## [1.2.4] - 2026-11-25

### 新增

- **更新通知。** 面板在启动 30 秒后自动检查新版本，之后每 6 小时检查一次，结果在重启后保留。有新版本时，“更新”页面以面板的语言显示当前版本之后所有版本的更新日志，并提供安装按钮；其他页面右下角显示一条通知，点击即进入该页面。程序运行新版本后，通知自动消失。通知上的 × 会在该浏览器中隐藏它，直到下一个版本。“立即检查”会马上询问 GitHub。
- **PHP 5.6**，用于暂时无法迁移的旧网站。在 Debian 和 Ubuntu 上来自 Sury（`php5.6-*`，Sury 仍在打补丁）；在 AlmaLinux、Rocky Linux 和 CentOS Stream 9、10 上来自 [mpn-packages](https://github.com/marixdev/mpn-packages/releases/tag/el-r3)，私有链接 OpenSSL 1.0.2u 构建。包含 mysqli、PDO 和旧的 `mysql_*` 函数、opcache、gd、curl、mbstring、xml 和 zip。PHP 官方自 2019 年 1 月起不再为 5.6 发布安全修复：仅用于无法在 7.4 或更新版本上运行的网站。
- **数据库远程访问。** 可以允许指定地址通过 3306 端口连接某个数据库（数据库 › 远程访问）。只有在至少一个数据库允许远程访问时，服务器才监听所有地址；防火墙只对这些地址开放 3306，为它们创建的用户只拥有该数据库的权限。修改密码会同时修改所有地址的密码。`mpn db remote` 可在 SSH 中完成同样的操作。
- **数据库服务器配置。** 服务 › 数据库 › 配置 可修改 `max_connections`、缓冲池、数据包大小、超时等数值，每个值都在检查过的范围内。服务器使用新文件重启；若无法启动，则恢复旧文件并用它重新启动。
- **通过 Cloudflare DNS 申请 SSL。** 位于 Cloudflare 代理（橙色云朵）之后的网站，或尚未指向本机的域名，可以通过 TXT 记录获取证书：在 SSL 标签页填写具有 Zone:Read 和 DNS:Edit 权限的 API 令牌。令牌会保留用于续期，仅 root 可读。
- **记住此浏览器。** 登录页面可让浏览器保持登录 30 天。登录状态现在在面板重启或更新后依然保留。“设置”显示已登录的浏览器数量，并可退出其他所有浏览器；修改密码会退出所有浏览器，包括通过 `mpn web passwd` 修改。
- **备份保留数量。** 每个网站的“备份”标签页可设置保留的备份数量，范围 1 到 365（默认 7）。每晚备份、手动备份以及复制到外部存储的副本都保留这个数量；调小数量会立即删除该网站较早的备份。`mpn backup keep -site 名称 -n 3 -apply` 可在 SSH 中完成同样的操作。

### 修复

- 在服务商已创建并正在使用 `/swapfile` 的机器上，安装在 swap 步骤停止并报错“fallocate failed: Text file busy”。现在保留已有的 swap，缺少的部分放在单独的文件 `/swapfile-mpn` 中。不支持 fallocate 的文件系统改用 dd 创建该文件。
- 位于 Cloudflare“完全”模式且开启“始终使用 HTTPS”的网站续期证书失败：Let's Encrypt 跟随跳转到 https，而网站把所有路径（包括验证路径）都跳转到别处。https 服务器现在自行响应 `/.well-known/acme-challenge/`，申请前的检查也像 Let's Encrypt 一样跟随同一域名的跳转。
- phpBB 安装程序在最后报错“The ../styles/prosilver/template directory does not exist”：面板在管理员账户出现后立即删除 `/install/`，而安装程序仍在从该目录运行。现在面板会等待安装程序结束。
- 在文件编辑器中，在文本区域外按 Ctrl+A 或通过菜单复制时只取到屏幕上显示的行。现在会取整个文件。

### 升级说明

- 面板启动 30 秒后才进行第一次版本检查；在此之前“更新”页面显示尚未检查。

## [1.2.3] - 2026-11-22

### 新增

- **活动日志。** 新的“活动”页面记录通过面板执行的操作：登录、网站、文件、数据库、SSL、备份、防火墙和安全，并附带时间、地址、网站以及是否成功。每个网站也有自己的“活动”标签页。可按网站、类型、地址和日期筛选，并可导出最多 1000 行 CSV。日志保留 90 天，只由 root 辅助进程写入，因此无法通过面板修改或删除。密码、密钥、证书、文件内容以及在终端中输入的内容从不记录。参见[活动日志](https://mpanel.biz/zh/activity/)。
- **Telegram 告警。** 新的 Telegram 页面会在以下情况发送消息：服务停止、网站无响应、CPU、内存或磁盘持续超过阈值、有人从新地址登录、fail2ban 封禁某个地址、备份失败、证书即将过期，或活动日志中出现标记为“重要”的变更。一个面板作为中心节点并持有机器人；其他服务器通过一次性配对码与其配对并每分钟上报，3 分钟无上报的服务器会被告警。告警运行在独立服务 `mpn-notify` 中，面板重启或更新时仍会继续。参见 [Telegram 告警](https://mpanel.biz/zh/notify/)。
- **服务端口。** 防火墙页面新增一张卡片，可将 SSH、面板、phpMyAdmin 或 Webmail 改到其他端口。SSH 和面板分两步更改：新端口先与旧端口同时开放，确认新端口可用后才关闭旧端口；若未确认，10 分钟后自动恢复为仅使用旧端口。防火墙规则和 fail2ban jail 会随之更新。`mpn port status|set|finish|cancel` 可通过 SSH 完成同样的操作。参见[防火墙 › 服务端口](https://mpanel.biz/zh/firewall/#服务端口)。
- **phpBB。** 新建 PHP 网站时可选择 phpBB 3.3.19。面板下载官方安装包并校验 SHA-256，创建数据库，并写入专为 phpBB 准备的 nginx 配置（`app.php` 路由、PATH_INFO、禁止访问内部目录）。您在 phpBB 自带的安装程序 `/install/` 中使用结果页给出的数据库信息完成安装。创建管理员后，面板会在一分钟内关闭 `/install/` 并删除该目录。

### 修复

- 在 SSH 监听非 22 端口的服务器上（部分服务商默认如此），所有网站的终端和文件管理器都无法使用，因为面板始终连接 `127.0.0.1:22`。现在面板会向 root 辅助进程询问 sshd 实际监听的端口，缓存一分钟，连接被拒绝时重新询问，因此之后更改的端口无需重启即可识别。
- 在 Ubuntu 24.04 及更新版本上，sshd 仅在有连接时由 `ssh.socket` 启动，防火墙因此读不到 SSH 端口，转而开放 22 端口而非实际端口，导致 SSH 不在 22 端口的服务器被锁在外面。现在面板也会从 `ssh.socket` 读取端口，且仅在其运行时读取：systemd 也会列出已停止的 socket 的地址，因此在 Debian 上更改端口后，旧端口（22）仍会与新端口一起显示并保持开放。
- 在 AlmaLinux、Rocky Linux 和 CentOS Stream 上，SELinux 步骤无论面板端口是多少，都为 Webmail 标记端口 2612。面板端口不是 2610 时，nginx 无法监听 Webmail 的端口。现在标签随面板端口而定，开启 Webmail 时也会重新标记其端口。

### 变更

- Ubuntu 26.04 现在与其他所有受支持的系统一样，从 nginx.org 获取 nginx 1.30.7，并使用 [mpn-packages](https://github.com/marixdev/mpn-packages/releases/tag/nginx-1.30.7-r2) 的 PageSpeed 和 ModSecurity 模块。此前它保留 Ubuntu 的 nginx 1.28.3，面板用 `apt-mark hold` 锁定该软件包；被锁定的软件包收不到安全更新，因此 11 月 18 日之前安装的机器一直没有 CVE-2026-123007 的修复。

### 升级说明

- 在 Ubuntu 26.04 上，`mpn update` 会在一次 apt 运行中把 Ubuntu 的 nginx、`nginx-module-pagespeed`（We-Amp）和 `libnginx-mod-http-modsecurity` 换成 nginx.org 的 nginx 1.30.7 及为其构建的模块。`/etc/nginx` 中面板的站点和设置保持不变。如果新 nginx 未通过 `nginx -t`，会恢复旧的软件包和 apt 文件，更新随即停止；变更前的文件保存在 `/var/backups/mpn-nginx-upgrade/old`。

## [1.2.2] - 2026-11-19

### 安全

- nginx 从 1.28.3 升级到 1.30.7，修复 2026 年 11 月 18 日公布的 nginx 安全公告，其中包括 CVE-2026-123007（HTTP/2 流量控制中的内存损坏，在所有启用 HTTP/2 的站点上无需登录即可触发）和 CVE-2026-118899。1.28 分支没有修复版本。PageSpeed 和 ModSecurity 模块已针对 1.30.7 重新构建（[nginx-1.30.7-r1](https://github.com/marixdev/mpn-packages/releases/tag/nginx-1.30.7-r1)、[el-r2](https://github.com/marixdev/mpn-packages/releases/tag/el-r2)），并照旧校验 SHA-256。

### 修复

- 在尚未创建管理员账户的机器上，`mpn update` 会恢复旧版本：此时 `/login` 跳转到 `/setup`，检查误以为面板没有响应。此修复从 1.2.2 起的更新生效。
- 面板自身的字体（JetBrains Mono）在每个页面上都被内容安全策略拒绝。

### 升级说明

- `mpn update`（以及“更新”页面）会把旧版本安装的机器上的 nginx 升级到 1.30.7，并一同更换其加载的模块。旧的配置和模块保存在 `/var/backups/mpn-nginx-upgrade/old`。如果新 nginx 未通过 `nginx -t`，会恢复旧的软件包、模块和版本锁定，更新随即停止。Ubuntu 26.04 继续使用发行版自带的 nginx。

## [1.2.1] - 2026-11-18

### 修复

- 1.2.0 中面板页面变慢：rclone 使 `mpn` 程序达到 109 MB，每次启动多花约 100 ms，而面板每个页面要启动 `mpn` 数次。rclone 现在位于第二个程序 `/usr/local/bin/mpn-cloud`（同一源代码，带 rclone 构建），`mpn` 只在连接存储以及复制、列出或下载备份时调用它。`mpn` 恢复为 19 MB，在 1 vCPU 服务器上启动时间从 0.18 s 降至 0.02 s。
- 异地存储列表对由 rclone 列表生成的存储显示了 `cld.kind.azureblob` 之类的翻译键。现在每个存储都显示名称、说明和提示，并可用搜索框筛选列表。

### 变更

- 更新有了单独的页面，即主菜单中的 **更新**，不再是设置页的一部分。
- 面板和文档网站默认使用英语。浏览器要求越南语或中文时显示对应语言，语言菜单仍可切换。`/en/` 下的旧文档地址会跳转到新地址。

### 升级须知

- `mpn update`、面板的更新按钮和安装程序都会下载这两个程序。由 1.2.0 更新的机器（只知道 `mpn`）会在更新结束时新的 `mpn` 运行 `mpn panel migrate -apply` 时获得 `mpn-cloud`。
## [1.2.0] - 2026-11-18

### 新增

- 可在界面（设置 → 更新）或用 `mpn update -apply` 更新面板。每个发布版本的 `SHA256SUMS` 都用 minisign 签名，面板只接受签名与程序内置密钥相符的版本。签名注释中写有版本号，旧版本无法冒充新版本。正在运行的程序保存为 `/usr/local/bin/mpn.prev`，更新后面板若无响应会自动换回。
- 异地备份改用 rclone：Google Drive、S3（Cloudflare R2、AWS、Wasabi…）、Backblaze B2、SFTP、WebDAV（Nextcloud、ownCloud）和 FTP/FTPS。只有当面板成功写入、读回并删除一个测试文件后才会保存连接。表单只把固定列表中的字段交给 rclone，因此任何选项都无法执行命令或读取本机文件。
- 网站经 Cloudflare 代理时可获取访客真实地址（“安全”页，或 `mpn security cloudflare -on -apply`）。nginx 只在来自 Cloudflare 公布网段的连接上采用 `CF-Connecting-IP` 中的地址。默认关闭。
- 安装 MariaDB 或 MySQL 时会同时安装 phpMyAdmin，“数据库”标签页可以立即打开它。
- `mpn panel migrate` 会把旧版本安装的机器调整为全新安装时的配置。安装程序和 `mpn update` 会自动运行它。

### 变更

- 面板改为监听 systemd 创建并保持的 Unix 套接字 `/run/mpn-web/web.sock`（`mpn-web.socket`），不再监听 `127.0.0.1:8780`。只有 nginx 和 phpMyAdmin（`mpn-front` 组）能连接。面板重启时套接字仍在，其他程序无法占用它的位置。只有面板监听该套接字时才信任 `X-Real-IP` 中的客户端地址。
- 新建的 Node.js 和 Python 网站（Debian、Ubuntu）监听 Unix 套接字 `/run/mpn-app/<站点>/app.sock`，不开放 TCP 端口，只有 nginx 能连接。套接字路径在 `SOCKET` 变量中，`PORT` 也是同一路径，因为 Node.js 的 `listen(process.env.PORT)` 接受路径。端口栏留空即使用套接字，填写端口号则像以前一样监听 `127.0.0.1`。已有网站保留原端口。RHEL 系列上应用仍使用端口。
- 设置页 `/setup` 只能通过带一次性密钥的链接打开。安装程序会输出该链接，在账户创建前 SSH 登录提示也会显示它，`mpn web setup-link` 可再次输出。此前谁先打开 `/setup` 谁就能创建账户。
- 只有当 `SHASUMS256.txt` 带有 Node.js 发布者的有效签名时才会安装 Node.js。签名用 `gpgv` 对照 mpn 内置的 29 个密钥检查，nodejs.org 或其镜像都无法自行选择密钥。`gpgv` 已加入基础软件包。
- 备份的 MANIFEST 包含每个文件的 SHA-256，以及用本机专属密钥（`/etc/mpn/backup.key`，权限 0600）生成的 HMAC。从异地存储下载的备份必须由本机签名，且每个文件都要与其校验和一致。
- 手写的 vHost 不能把日志写到 `/var/log/mpn/sites/<站点>/` 之外；读取文件或影响整台机器的指令（`include`、`load_module`、站点证书目录之外的 `ssl_certificate`、`*_temp_path`、`*_cache_path`、`user`、`pid` 等）会被拒绝。原因是 nginx 在切换到 worker 账户之前以 root 身份打开日志文件。
- Laravel 使用 `DB_HOST=localhost`（经 Unix 套接字），与 WordPress 和 CodeIgniter 一致。
- 双因素认证的二维码改由面板自己的代码绘制，替换已停止维护的 go-qrcode 库。
- 以 root 运行的命令和操作在网站 home 中创建目录时，通过已打开的目录描述符进行，从不跟随符号链接，网站账户无法用符号链接把 root 引向其他文件。
- 不能创建更明确属于另一网站的数据库名称（例如已有网站 `blog-shop` 时的 `blog_shop`）。
- 当等待计算密码哈希的登录过多时，面板立即返回 429 而不是排队，避免一轮密码猜测占满 CPU。
- 公开源代码包含测试，GitHub Actions 在每次推送时运行 gofmt、vet、测试和 govulncheck。

### 升级须知

- 用 1.1.x 制作的备份没有签名，无法再从异地存储下载。保存在本机上的备份仍可照常恢复。
- 手写 vHost 中使用 `include` 或把日志写到别处的网站，下次应用时会报错，修改该部分后即可恢复。

## [1.1.1] - 2026-11-14

### 修复

- 当网站根目录位于 `htdocs` 之下时（例如 Laravel、Flarum 或 Symfony 的 `htdocs/public`），备份现在会归档整个 `htdocs` 目录。此前备份只包含 `public/`，缺少 `config.php`/`.env`、`storage`（上传的文件）和 `vendor`，恢复后网站无法再次运行。网站根目录就是 `htdocs` 的网站不受影响。
- 这类备份的 MANIFEST 新增一行 `files_root=htdocs`，恢复时以此为准：旧备份（只包含网站根目录）仍像以前一样解压到网站根目录。mpn 1.1.0 及更早版本无法读取新的 MANIFEST，因此恢复新备份前请先升级。

## [1.1.0] - 2026-11-13

### 新增

- 支持 AlmaLinux、Rocky Linux 和 CentOS Stream 9 与 10，x86_64 和 arm64 均可。Nginx 1.28.3 来自 nginx.org，并用 `dnf versionlock` 锁定；PHP 7.4 及以上来自 Remi（附带 EPEL）；PHP 7.0 至 7.3、ModSecurity 模块和 OWASP CRS 4.25.2 来自 mpn-packages 的 [el-r1](https://github.com/marixdev/mpn-packages/releases/tag/el-r1) 版本；PageSpeed 来自 [nginx-1.28.3-r3](https://github.com/marixdev/mpn-packages/releases/tag/nginx-1.28.3-r3) 版本。所有下载的文件都会与固定的 SHA-256 比对。
- MariaDB 来自 rpm.mariadb.org，MySQL 来自 repo.mysql.com；两个软件源的签名密钥都按指纹校验。EL10 只有 MySQL 8.4。
- SELinux 保持开启：面板为网站目录、日志、PHP-FPM 套接字、Python 网站的 venv、Node.js、邮箱和面板端口设置标签，并开启 `httpd_can_network_connect`，使网站可以连接数据库和外部服务。
- 安装程序会关闭 firewalld，因为面板使用 nftables 管理防火墙。
- RHEL 系列上的邮件：邮箱密码用 bcrypt（`BLF-CRYPT`）哈希，因为那里的 Dovecot 不支持 argon2；OpenDKIM 在回环地址的 8891 端口监听，因为 SELinux 策略不允许 Postfix 连接它的套接字。

### 变更

- nginx 模块现在位于 mpn-packages 仓库（原 mpn-nginx-modules，旧地址仍会跳转到这里）。

## [1.0.3] - 2026-11-13

### 新增

- 支持 ARM 机器（arm64、aarch64）。安装程序会识别 aarch64 并下载 `mpn_linux_arm64`。arm64 的 PageSpeed 和 ModSecurity 模块来自 [nginx-1.28.3-r2](https://github.com/marixdev/mpn-packages/releases/tag/nginx-1.28.3-r2) 版本（在 GitHub 的 ARM 运行器上构建，按操作系统和架构固定 SHA-256）。网页邮件升级到 [mpn-mail 1.0.2](https://github.com/marixdev/mpn-mail/releases/tag/v1.0.2)，提供 `mpn-mail_linux_arm64` 版本。
- 开启邮件时，会在安装 Postfix 之前自动卸载会占用 25 端口的现有邮件服务器（Exim、Sendmail、OpenSMTPD），不再停下来要求手动卸载。

### 修复

- 在 Debian 13（以及 Ubuntu 26.04）上可以开启邮件：面板能识别 Dovecot 2.4，并按新语法写入 `dovecot.conf`（`dovecot_config_version`、`mail_driver`、`quota_storage_size` 配额）。Dovecot 2.3 保持旧配置。
- 基础软件包步骤会安装并启用 `cron`：部分 Debian 13 镜像没有预装它，导致计划任务标签页无法运行任何任务。

## [1.0.2] - 2026-11-12

### 新增

- 文件管理器支持剪切、复制和粘贴，可在右键菜单、多选时的工具栏中使用，也可用 Ctrl+X、Ctrl+C、Ctrl+V。剪切或复制的项目按网站记在当前浏览器标签页中，因此可以打开另一个目录后再粘贴。在目录上点右键会粘贴到该目录中。
- 复制到原项目所在的目录时，副本命名为 `名称 (copy).扩展名`、`名称 (copy 2).扩展名` 等。副本保留原文件的权限，链接和特殊文件会被跳过。

### 安全

- 粘贴绝不覆盖：如果目标目录中已有同名项目，则不会移动或复制任何项目。不能把目录粘贴到它自身或其子目录中，也不能超出网站的主目录。

## [1.0.1] - 2026-11-12

### 新增

- 简体中文界面（`zh`），与越南语和英语并列。可在顶栏的语言按钮中选择，登录页和 `/setup` 也可以；首次访问时，面板根据浏览器的 `Accept-Language` 选择。
- 字体增加 PingFang SC、Microsoft YaHei 和 Noto Sans CJK SC，因为 JetBrains Mono 没有汉字。

### 变更

- 网页邮件升级到 [mpn-mail 1.0.1](https://github.com/marixdev/mpn-mail/releases/tag/v1.0.1)，支持中文并跟随面板的语言。
- 概览标签页中 PHP 设置的说明现在跟随所选语言显示，以前总是越南语。
- README 新增英文版和中文版（[README.zh.md](README.zh.md)），并附有配套仓库表（mpn-mail、mpn-nginx-modules）。

## [1.0.0] - 2026-11-12

首个版本：PHP、Node.js、Python、静态和代理网站；Let's Encrypt SSL；MariaDB/MySQL 与 phpMyAdmin；邮件与 mpn-mail 1.0.0 网页邮件；备份；防火墙、ModSecurity、fail2ban、双因素认证；监控；越南语和英语界面。

[1.2.0]: https://github.com/marixdev/mini-panel/compare/v1.1.1...v1.2.0
[1.1.1]: https://github.com/marixdev/mini-panel/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/marixdev/mini-panel/compare/v1.0.3...v1.1.0
[1.0.3]: https://github.com/marixdev/mini-panel/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/marixdev/mini-panel/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/marixdev/mini-panel/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/marixdev/mini-panel/releases/tag/v1.0.0
