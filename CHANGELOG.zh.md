# 更新日志

Mini Panel 的所有重要变更都记录在这里。版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。

本文译自 [CHANGELOG.md](CHANGELOG.md)（越南语）。另有[英文版](CHANGELOG.en.md)。

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
- README 新增英文版（[README.en.md](README.en.md)）和中文版（[README.zh.md](README.zh.md)），并附有配套仓库表（mpn-mail、mpn-nginx-modules）。

## [1.0.0] - 2026-11-12

首个版本：PHP、Node.js、Python、静态和代理网站；Let's Encrypt SSL；MariaDB/MySQL 与 phpMyAdmin；邮件与 mpn-mail 1.0.0 网页邮件；备份；防火墙、ModSecurity、fail2ban、双因素认证；监控；越南语和英语界面。

[1.1.1]: https://github.com/marixdev/mini-panel/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/marixdev/mini-panel/compare/v1.0.3...v1.1.0
[1.0.3]: https://github.com/marixdev/mini-panel/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/marixdev/mini-panel/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/marixdev/mini-panel/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/marixdev/mini-panel/releases/tag/v1.0.0
