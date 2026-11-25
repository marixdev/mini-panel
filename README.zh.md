<p align="center"><img src="logo.svg" alt="Mini Panel" width="72"></p>

<h1 align="center">Mini Panel</h1>

<p align="center">
面向小型 VPS 的轻量主机控制面板，使用 Go 编写。只有一个可执行文件，在 1 vCPU / 1 GB 内存的机器上运行良好。<br>
<a href="https://mpanel.biz">mpanel.biz</a> · <a href="https://mpanel.biz/install/">安装</a> · <a href="https://github.com/marixdev/mini-panel/releases">发布版本</a> · <a href="https://demo.mpanel.biz">演示</a> · <a href="https://t.me/minipanelbiz">Telegram</a>
</p>

<p align="center"><a href="README.md">English</a> · <a href="README.vi.md">Tiếng Việt</a> · <b>简体中文</b></p>

---

## 安装

以 root 登录 VPS 后运行：

```sh
curl -fsSL https://mpanel.biz/install.sh | sh
```

安装程序会检查操作系统，从 GitHub 最新发布版本下载 `mpn`，并与 `SHA256SUMS` 比对 SHA-256。随后安装 nginx 1.30.7 及 PageSpeed 模块，并在 2610 端口安装使用自签名证书的面板。只有在“安全”标签页中开启 Web 应用防火墙时才会安装 ModSecurity 模块。安装程序**不会**安装 PHP、Node.js 或数据库，这些可以稍后在面板的“服务”页面中选择。

安装完成后，打开输出的设置链接（`https://<IP>:2610/setup?t=…`）创建管理员账户。该链接带有一次性密钥，在账户创建之前有效；`mpn web setup-link` 可再次输出它。

可选项：`MPN_VERSION=1.0.2` 安装指定版本，`MPN_PANEL_PORT=2610` 修改面板端口。

**操作系统：** Ubuntu 22.04、24.04、26.04，Debian 12、13，以及 AlmaLinux、Rocky Linux、CentOS Stream 9、10，x86_64 或 arm64（aarch64）架构。在 RHEL 系列上 SELinux 保持开启，firewalld 会被关闭（面板用 nftables 管理防火墙）。

## 功能

- **网站：** PHP 5.6 及 7.0 至 8.5（每个站点一个 PHP-FPM 池）、Node.js 18 至 24、Python、静态站点和反向代理。每个站点都有独立的系统账户、域名、SSH/SFTP、文件管理器、网页终端、计划任务、日志和可编辑的 vHost（加载前用 `nginx -t` 检查）。
- **应用安装：** WordPress、Laravel、CodeIgniter 4、CakePHP、Yii 2，并附带 WordPress 管理页面（更新、插件、主题）。
- **SSL：** Let's Encrypt（先检查 DNS，自动续期）、自定义证书、生成 CSR。面板本身也有独立的证书。
- **数据库：** MariaDB 或 MySQL，每个数据库一个用户，phpMyAdmin 使用临时账户登录。
- **邮件**（默认关闭）：Postfix、Dovecot、OpenDKIM。域名、邮箱和别名在各网站内管理，提供现成的 MX/SPF/DKIM/DMARC 记录，并可直接添加到 Cloudflare。网页邮件 [mpn-mail](https://github.com/marixdev/mpn-mail) 无人使用时自动退出。
- **备份：** 每晚备份，保留 7 份，可在界面中恢复，可通过 rclone 复制到异地：Google Drive、S3（Cloudflare R2、AWS、Wasabi）、Backblaze B2、SFTP、WebDAV 或 FTP。每份备份的 MANIFEST 都用本机专属密钥签名。
- **安全：** 不会误锁 SSH 和面板端口的 nftables 防火墙、带 OWASP CRS 的 ModSecurity、fail2ban、双因素认证（TOTP）、经 Cloudflare 代理时获取访客真实地址。
- **监控：** 24 小时内的 CPU、内存、负载、网络、磁盘和服务状态。
- **更新：** 在主菜单的 **更新** 或用 `mpn update -apply`；只有签名与程序内置密钥相符的发布版本才会安装。
- **界面** 支持英语、越南语和简体中文，浅色和深色主题。

## 工作原理

- `mpn web` 以无 root 权限的用户提供界面，监听由 systemd 保持的 Unix 套接字 `/run/mpn-web/web.sock`（只有 nginx 和 phpMyAdmin 能打开），前面由启用 TLS 的 nginx 代理。
- Node.js 和 Python 应用监听各自的 Unix 套接字（`/run/mpn-app/<站点>/app.sock`，路径在 `SOCKET` 和 `PORT` 中），只有 nginx 能连接。在 RHEL 系列上应用监听 `127.0.0.1` 的端口。
- `mpn root` 是以 root 运行的辅助进程，监听 Unix 套接字，只接受固定的操作列表。每个值在成为命令行参数之前都会再次检查，不经过 shell。
- 密码、私钥和证书通过标准输入传递，不会出现在参数、日志或数据库中。
- 对机器的每项更改都是一个分步骤的计划，默认只预览，加上 `-apply` 才真正执行。如果 `nginx -t` 或某一步失败，会恢复原有文件。
- nginx 在所有操作系统上固定为 1.30.7（来自 nginx.org，用 `apt-mark hold` 或 `dnf versionlock` 锁定），因此 `apt upgrade` 或 `dnf upgrade` 不会破坏为该版本构建的模块（[mpn-packages](https://github.com/marixdev/mpn-packages)）。所有下载的文件都会与源代码中写明的 SHA-256 比对，Node.js 发布版本还必须带有 Node.js 发布者的有效签名。
- 面板数据保存在 SQLite 中：`/var/lib/mpn/mpn.db`，权限 0600。

## 配套仓库

面板从以下组件的发布版本下载文件，并校验源代码中固定的 SHA-256：

| 仓库 | 面板用途 | 固定位置 |
|---|---|---|
| [marixdev/mpn-mail](https://github.com/marixdev/mpn-mail) | `/webmail/` 网页邮件（lilmail 的定制版） | `internal/mail/webmail.go` |
| [marixdev/mpn-packages](https://github.com/marixdev/mpn-packages) | 为 nginx 1.30.7 构建的 PageSpeed 和 ModSecurity 模块；适用于 AlmaLinux、Rocky Linux、CentOS Stream 的 PHP 5.6、7.0 至 7.3 与 OWASP CRS | `internal/installer/nginx.go`、`internal/installer/el.go` |

## 命令行

```
mpn version | update | tune | plan | site | db | backup | cron | firewall | security | panel | cloud | app | wp | mail | services | root | web
```

在子命令后加 `-h` 查看参数。会更改机器的命令默认只预览，需要加 `-apply` 才执行。忘记密码：`mpn web passwd`。丢失验证器应用：`mpn web 2fa-off`。

| 项目 | 位置 |
|---|---|
| 可执行文件 | `/usr/local/bin/mpn`，以及 `/usr/local/bin/mpn-cloud`（带 rclone 的同一程序，用于异地复制） |
| 面板数据库 | `/var/lib/mpn/mpn.db` |
| 各站点证书 | `/etc/mpn/certs/<站点>/` |
| 各站点 nginx 日志 | `/var/log/mpn/sites/<站点>/` |
| 备份 | `/var/backups/mpn/<站点>/<时间>/` |
| 备份签名密钥 | `/etc/mpn/backup.key` |
| 服务 | `mpn-root`、`mpn-web.socket`、`mpn-web`、`mpn-renew.timer`、`mpn-backup.timer` |

## 从源代码构建

需要 Go 1.27 或更高版本：

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o mpn ./cmd/mpn
```

用 `go test ./...` 运行测试；每次推送时 CI 会运行 gofmt、vet、测试和 govulncheck（[ci.yml](.github/workflows/ci.yml)）。推送 `v*` 标签时由 GitHub Actions 构建并签名发布版本（[release.yml](.github/workflows/release.yml)），发布说明取自 [CHANGELOG.md](CHANGELOG.md)（英文）中对应版本的章节。修改 `internal/web` 中的模板后，用 Tailwind v4 重新生成 CSS：`tailwindcss -i assets/input.css -o assets/app.css --minify`。CodeMirror 编辑器在 `tools/editor` 中构建。

## 许可证

Mini Panel 以 [GNU General Public License v3.0](LICENSE) 发布。第三方库及其许可证列于 [NOTICE](NOTICE)。
