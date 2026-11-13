# Changelog

All notable changes to Mini Panel are recorded here. Version numbers follow [Semantic Versioning](https://semver.org/).

This is a translation of [CHANGELOG.md](CHANGELOG.md) (Vietnamese). Also in [Chinese](CHANGELOG.zh.md).

## [1.1.1] - 2026-11-14

### Fixed

- Backups now archive the whole `htdocs` folder when the web root lies below it, for example `htdocs/public` for Laravel, Flarum or Symfony. Before, a backup held only `public/` and lacked `config.php`/`.env`, `storage` (uploaded files) and `vendor`, so the site could not run again after a restore. Sites whose web root is `htdocs` itself are unchanged.
- The MANIFEST of those backups has a new line, `files_root=htdocs`, and restore relies on it: older backups (which hold only the web root) are still extracted into the web root as before. mpn 1.1.0 and older cannot read the new MANIFEST, so upgrade before restoring new backups.

## [1.1.0] - 2026-11-13

### Added

- Runs on AlmaLinux, Rocky Linux and CentOS Stream 9 and 10, on both x86_64 and arm64. Nginx 1.28.3 comes from nginx.org and is held with `dnf versionlock`; PHP 7.4 and later from Remi (with EPEL); PHP 7.0 to 7.3, the ModSecurity module and OWASP CRS 4.25.2 from the [el-r1](https://github.com/marixdev/mpn-packages/releases/tag/el-r1) release of mpn-packages; PageSpeed from the [nginx-1.28.3-r3](https://github.com/marixdev/mpn-packages/releases/tag/nginx-1.28.3-r3) release. Every download is checked against a pinned SHA-256.
- MariaDB comes from rpm.mariadb.org and MySQL from repo.mysql.com; the signing keys of both repositories are checked by fingerprint. EL10 has MySQL 8.4 only.
- SELinux stays enabled: the panel labels website folders, logs, PHP-FPM sockets, the venvs of Python sites, Node.js, mailboxes and the panel's port, and turns on `httpd_can_network_connect` so sites can reach their databases and outside services.
- The installer turns off firewalld, because the panel manages the firewall with nftables.
- Mail on the RHEL family: mailbox passwords are hashed with bcrypt (`BLF-CRYPT`), because Dovecot there has no argon2; OpenDKIM listens on port 8891 on loopback, because the SELinux policy does not let Postfix connect to its socket.

### Changed

- The nginx modules now live in the mpn-packages repository (formerly mpn-nginx-modules; the old address still redirects there).

## [1.0.3] - 2026-11-13

### Added

- Runs on ARM machines (arm64, aarch64). The installer recognises aarch64 and downloads `mpn_linux_arm64`. The PageSpeed and ModSecurity modules for arm64 come from the [nginx-1.28.3-r2](https://github.com/marixdev/mpn-packages/releases/tag/nginx-1.28.3-r2) release (built on GitHub's ARM runners, SHA-256 pinned per operating system and architecture). Web mail moves to [mpn-mail 1.0.2](https://github.com/marixdev/mpn-mail/releases/tag/v1.0.2), which has an `mpn-mail_linux_arm64` build.
- Turning on mail removes existing mail servers that would hold port 25 (Exim, Sendmail, OpenSMTPD) before installing Postfix, instead of stopping and asking for them to be removed by hand.

### Fixed

- Mail can be turned on on Debian 13 (and Ubuntu 26.04): the panel recognises Dovecot 2.4 and writes `dovecot.conf` in the new syntax (`dovecot_config_version`, `mail_driver`, the `quota_storage_size` quota). Dovecot 2.3 keeps the old configuration.
- The base packages step installs and enables `cron`: some Debian 13 images do not have it, so the Cron tab could not run any job.

## [1.0.2] - 2026-11-12

### Added

- The file manager has Cut, Copy and Paste, in the right-click menu, on the bar when several items are selected, and with Ctrl+X, Ctrl+C, Ctrl+V. Cut or copied items are remembered per website in the open browser tab, so you can open another folder and paste there. Right-clicking a folder pastes into that folder.
- Copying into the folder that holds the original names the copy `name (copy).ext`, `name (copy 2).ext` and so on. Copies keep the permissions of the original; links and special files are skipped.

### Safety

- Pasting never overwrites: if a name already exists in the target folder, nothing is moved or copied. A folder cannot be pasted into itself or one of its subfolders, and nothing can leave the website's home folder.

## [1.0.1] - 2026-11-12

### Added

- Simplified Chinese interface (`zh`), beside Vietnamese and English. Pick it with the language button in the top bar, also on the sign-in page and `/setup`; on the first visit, the panel chooses by the browser's `Accept-Language`.
- The font stack adds PingFang SC, Microsoft YaHei and Noto Sans CJK SC, because JetBrains Mono has no Chinese characters.

### Changed

- Web mail moves to [mpn-mail 1.0.1](https://github.com/marixdev/mpn-mail/releases/tag/v1.0.1), which has Chinese and follows the panel's language.
- The descriptions of the PHP settings in the Overview tab now follow the selected language; before, they were always in Vietnamese.
- The README is now also in English ([README.en.md](README.en.md)) and Chinese ([README.zh.md](README.zh.md)), with a table of the companion repositories (mpn-mail, mpn-nginx-modules).

## [1.0.0] - 2026-11-12

First release: PHP, Node.js, Python, static and proxy websites; Let's Encrypt SSL; MariaDB/MySQL and phpMyAdmin; email and the mpn-mail 1.0.0 web mail; backups; firewall, ModSecurity, fail2ban, 2FA; monitoring; Vietnamese and English interface.

[1.1.1]: https://github.com/marixdev/mini-panel/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/marixdev/mini-panel/compare/v1.0.3...v1.1.0
[1.0.3]: https://github.com/marixdev/mini-panel/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/marixdev/mini-panel/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/marixdev/mini-panel/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/marixdev/mini-panel/releases/tag/v1.0.0
