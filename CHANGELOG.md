# Changelog

All notable changes to Mini Panel are recorded here. Version numbers follow [Semantic Versioning](https://semver.org/).

Translations: [Tiếng Việt](CHANGELOG.vi.md) · [中文](CHANGELOG.zh.md).

## [1.2.4] - 2026-11-25

### Added

- **Update notices.** The panel looks for a new release by itself, 30 seconds after it starts and then every 6 hours, and keeps what it found across restarts. When one exists, the Update page shows the changelog of every release after the one that runs, in the panel's language, with a button that installs it; every other page shows a notice in the lower right corner that leads there. Once the program runs the release, the notice goes away. The × on the notice hides it in that browser until the next release. "Check now" asks GitHub at once.
- **PHP 5.6**, for old sites that cannot move yet. On Debian and Ubuntu it comes from Sury (`php5.6-*`, which Sury still patches); on AlmaLinux, Rocky Linux and CentOS Stream 9 and 10 from [mpn-packages](https://github.com/marixdev/mpn-packages/releases/tag/el-r3), built with OpenSSL 1.0.2u linked in privately. It has mysqli, PDO and the old `mysql_*` functions, opcache, gd, curl, mbstring, xml and zip. PHP itself has made no security fix for 5.6 since January 2019: use it only for a site that cannot run on 7.4 or newer.
- **Database remote access.** A database can be reached from chosen addresses on port 3306 (Databases › Remote access). The server listens on every address only while some database allows one; the firewall opens 3306 to those addresses alone, and the user made for them has the rights on that database only. Changing the password changes it for every address. `mpn db remote` does the same from SSH.
- **Database server settings.** Services › Database › Settings changes `max_connections`, the buffer pool, the packet size, timeouts and a few other values, each within a checked range. The server is restarted with the new file; when it does not start, the previous file is put back and the server started with it.
- **SSL through Cloudflare DNS.** A site behind the Cloudflare proxy (orange cloud), or a name that does not point here yet, can get its certificate with a TXT record: give an API token with Zone:Read and DNS:Edit on the SSL tab. The token is kept for the renewals, readable by root only.
- **Remember this browser.** The sign-in page can keep a browser signed in for 30 days. Sign-ins now outlive a restart or an update of the panel. Settings shows how many browsers are signed in and signs out all the others; a new password signs out every browser, `mpn web passwd` included.
- **Number of backups kept.** The Backups tab of a website sets how many backups are kept, from 1 to 365 (7 until it is changed). The nightly backup, a backup made by hand and the copy to the storage all keep that many; a smaller number deletes the older backups of the site at once. `mpn backup keep -site NAME -n 3 -apply` does the same over SSH.

### Fixed

- The install stopped at the swap step with "fallocate failed: Text file busy" on machines whose provider made a `/swapfile` and already uses it. The swap already there is now kept, and what is missing goes into a file of its own, `/swapfile-mpn`. On a filesystem without fallocate the file is written with dd.
- Renewing a certificate failed for a site behind Cloudflare in "Full" mode with "Always use HTTPS": the authority follows the redirect to https, where the site sent every path, the challenge included, elsewhere. The https server now answers `/.well-known/acme-challenge/` itself, and the check before a request follows redirects on the same name as Let's Encrypt does.
- The phpBB installer broke at the end with "The ../styles/prosilver/template directory does not exist": the panel removed `/install/` as soon as an administrator existed, while the installer was still running from it. It now waits until the installer has finished.
- In the file editor, Ctrl+A outside the text and copying from a menu took only the lines on screen. They now take the whole file.

### Upgrade notes

- The first look for a release is made 30 seconds after the panel starts; until then the Update page says it has not looked yet.

## [1.2.3] - 2026-11-22

### Added

- **Activity log.** A new Activity page records what was done through the panel: sign-ins, websites, files, databases, SSL, backups, the firewall and security, with the time, the address, the website and whether it worked. Each website has an Activity tab of its own. Filters by website, kind, address and dates; CSV export of up to 1000 lines. Entries are kept for 90 days and are written by the root helper only, so they cannot be changed or deleted through the panel. Passwords, keys, certificates, file contents and what is typed in the terminal are never written. See [Activity log](https://mpanel.biz/activity/).
- **Telegram alerts.** The new Telegram page sends a message when a service stops, a website stops answering, CPU, memory or disk stays above a limit, someone signs in from a new address, fail2ban bans an address, a backup fails, a certificate is about to expire or a change marked Important is made in the activity log. One panel is the hub and holds the bot; other servers pair with it by a one-time code and report every minute, and a server that goes quiet for 3 minutes is reported. The alerts run in their own service, `mpn-notify`, so they go on while the panel restarts or updates. See [Telegram alerts](https://mpanel.biz/notify/).
- **Service ports.** A new card on the Firewall page moves SSH, the panel, phpMyAdmin or the web mail to another port. SSH and the panel change in two steps: the new port opens next to the old one, and the old one closes only after you confirm the new one works; with no confirmation, the old port alone is restored after 10 minutes. The firewall rules and the fail2ban jails follow the change. `mpn port status|set|finish|cancel` does the same from SSH. See [Firewall › Service ports](https://mpanel.biz/firewall/#service-ports).
- **phpBB.** phpBB 3.3.19 can be chosen as the code of a new PHP website. The panel downloads the official package, checks its SHA-256, creates the database and writes an nginx configuration made for phpBB (`app.php` routes, PATH_INFO, internal folders refused). You finish in phpBB's own installer at `/install/` with the database details the result page shows. Once an administrator exists, the panel closes `/install/` and removes the folder within a minute.

### Fixed

- The terminal and the file manager of every website failed on a server whose SSH listens on a port other than 22, as some providers set it. The panel always dialled `127.0.0.1:22`. It now asks the root helper which port sshd listens on, keeps the answer for a minute, and asks again when a connection is refused, so a port changed later is found without a restart.
- On Ubuntu 24.04 and newer, where sshd is started by `ssh.socket` only when a connection comes in, the firewall could not read the SSH port and kept port 22 open instead of the real one, which locked SSH out on a server whose SSH is not on port 22. The panel now reads the port from `ssh.socket` too, and only while it runs: systemd also prints the addresses of a socket that is off, so on Debian the old port (22) stayed listed and open next to the new one after a change.
- On AlmaLinux, Rocky Linux and CentOS Stream, the SELinux step labelled port 2612 for the web mail whatever the panel's port was. With a panel port other than 2610, nginx could not listen on the web mail's port. The label now follows the panel's port, and turning the web mail on labels its port again.

### Changed

- Ubuntu 26.04 now takes nginx 1.30.7 from nginx.org, like every other supported system, with the PageSpeed and ModSecurity modules of [mpn-packages](https://github.com/marixdev/mpn-packages/releases/tag/nginx-1.30.7-r2). It used to keep Ubuntu's nginx 1.28.3, which the panel holds with `apt-mark hold`; a held package gets no security updates, so a machine installed before 18 November stayed without the fix for CVE-2026-123007.

### Upgrade notes

- On Ubuntu 26.04, `mpn update` replaces Ubuntu's nginx, `nginx-module-pagespeed` (We-Amp) and `libnginx-mod-http-modsecurity` with nginx 1.30.7 from nginx.org and the modules built for it, in one apt run. The panel's sites and settings in `/etc/nginx` stay. If the new nginx fails `nginx -t`, the old packages and apt files are put back and the update stops; the files before the change are in `/var/backups/mpn-nginx-upgrade/old`.

## [1.2.2] - 2026-11-19

### Security

- nginx moves from 1.28.3 to 1.30.7, which fixes the nginx advisories of 18 November 2026, among them CVE-2026-123007 (memory corruption in HTTP/2 flow control, reachable without signing in on every site with HTTP/2) and CVE-2026-118899. The 1.28 branch gets no fix. The PageSpeed and ModSecurity modules are built again for 1.30.7 ([nginx-1.30.7-r1](https://github.com/marixdev/mpn-packages/releases/tag/nginx-1.30.7-r1), [el-r2](https://github.com/marixdev/mpn-packages/releases/tag/el-r2)) and checked by SHA-256 as before.

### Fixed

- `mpn update` put the previous version back on a machine where the admin account had not been made yet, because `/login` sends the browser to `/setup` there and the check took that for the panel not answering. The fix works for updates from 1.2.2 on.
- The panel's own font (JetBrains Mono) was refused by the content security policy on every page.

### Upgrade notes

- `mpn update` (and the Update page) moves nginx of a machine installed by an earlier version to 1.30.7, together with the modules it loads. The old configuration and modules are kept in `/var/backups/mpn-nginx-upgrade/old`. If the new nginx fails `nginx -t`, the old package, modules and pin are put back and the update stops. Ubuntu 26.04 keeps the nginx of the distribution.

## [1.2.1] - 2026-11-18

### Fixed

- The panel's pages were slower in 1.2.0, because rclone made the `mpn` program 109 MB and every start about 100 ms longer, and the panel starts `mpn` several times for each page. rclone now ships in a second program, `/usr/local/bin/mpn-cloud` (the same source built with rclone), which `mpn` runs only to connect a storage and to copy, list or fetch backups. `mpn` is 19 MB again and starts in 0.02 s instead of 0.18 s on a 1-vCPU server.
- The off-site storage list showed translation keys such as `cld.kind.azureblob` for the storage generated from rclone's list. Each now shows its name, a description and a hint, and a search box filters the list.

### Changed

- Updating has its own page, **Update** in the main menu, instead of a section of Settings.
- The panel and the docs site are in English by default. A browser that asks for Vietnamese or Chinese gets it, and the language menu still switches. Old docs addresses under `/en/` lead to the new ones.

### Upgrade notes

- `mpn update`, the panel's Update button and the installer fetch both programs. A machine updated by 1.2.0, which knows only `mpn`, gets `mpn-cloud` when the new `mpn` runs `mpn panel migrate -apply` at the end of the update.
## [1.2.0] - 2026-11-18

### Added

- Update the panel from the interface (Settings, Updates) or with `mpn update -apply`. Each release's `SHA256SUMS` is signed with minisign, and the panel accepts only a release whose signature matches the key built into the program. The signed comment names the version, so an old release cannot be passed off as a new one. The running program is kept as `/usr/local/bin/mpn.prev` and put back if the panel does not answer after the update.
- Off-site backup copies now go through rclone: Google Drive, S3 (Cloudflare R2, AWS, Wasabi…), Backblaze B2, SFTP, WebDAV (Nextcloud, ownCloud) and FTP/FTPS. A connection is saved only after the panel has written, read back and deleted a test file. The form passes rclone only the fields on a fixed list, so no option can run a command or read a file of the machine.
- An option to see visitors' real addresses when sites are behind Cloudflare (Security, or `mpn security cloudflare -on -apply`). nginx takes the address from `CF-Connecting-IP`, but only on connections from Cloudflare's published networks. Off by default.
- Installing MariaDB or MySQL also installs phpMyAdmin, so the Databases tab can open it straight away.
- `mpn panel migrate` brings a machine installed by an older version to the configuration a fresh install writes. The installer and `mpn update` run it.

### Changed

- The panel listens on the Unix socket `/run/mpn-web/web.sock`, which systemd creates and keeps (`mpn-web.socket`), instead of `127.0.0.1:8780`. Only nginx and phpMyAdmin (group `mpn-front`) can connect. The socket stays while the panel restarts, so no other program can take its place. The client address in `X-Real-IP` is trusted only when the panel listens on that socket.
- New Node.js and Python sites (Debian, Ubuntu) listen on the Unix socket `/run/mpn-app/<site>/app.sock` and open no TCP port. Only nginx can connect. The socket path is in `SOCKET`, and `PORT` holds the same path, because Node.js's `listen(process.env.PORT)` accepts a path. An empty port field means the socket; a port number makes the app listen on `127.0.0.1` as before. Existing sites keep their port. On the RHEL family apps still use a port.
- The setup page `/setup` opens only with a link that carries a one-time key. The installer prints it, the SSH login message shows it until the account exists, and `mpn web setup-link` prints it again. Before, whoever opened `/setup` first created the account.
- Node.js is installed only when `SHASUMS256.txt` carries a valid signature of a Node.js releaser. It is checked with `gpgv` against 29 keys built into mpn, so neither nodejs.org nor a mirror can choose the keys. `gpgv` is in the base packages.
- A backup's MANIFEST holds the SHA-256 of each file and an HMAC made with the machine's own key (`/etc/mpn/backup.key`, mode 0600). A backup downloaded from off-site storage must be signed by this machine, and each file must match its sum.
- A hand-written vHost may not log outside `/var/log/mpn/sites/<site>/`, and directives that read files or affect the whole machine (`include`, `load_module`, `ssl_certificate` outside the site's certificate folders, `*_temp_path`, `*_cache_path`, `user`, `pid`…) are refused. nginx opens log files as root before it switches to the worker account.
- Laravel gets `DB_HOST=localhost` (over the Unix socket), as WordPress and CodeIgniter already did.
- The two-factor QR code is drawn by the panel's own code, replacing the unmaintained go-qrcode library.
- Commands and operations that run as root create folders inside a site's home through open folder descriptors and never follow symbolic links, so the site account cannot use a symlink to point root at another file.
- A database cannot be created with a name that belongs more closely to another site (for example `blog_shop` when a site `blog-shop` exists).
- When too many sign-ins are waiting to hash a password, the panel answers 429 at once instead of queueing, so a password-guessing burst cannot take all the CPU.
- The public source includes the tests, and GitHub Actions runs gofmt, vet, the tests and govulncheck on every push.

### Upgrade notes

- Backups made with 1.1.x are not signed, so they can no longer be downloaded from off-site storage. Backups stored on the machine restore as before.
- A site whose hand-written vHost uses `include` or logs elsewhere reports an error at its next apply until that part is fixed.

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
- The README is now also in English and Chinese ([README.zh.md](README.zh.md)), with a table of the companion repositories (mpn-mail, mpn-nginx-modules).

## [1.0.0] - 2026-11-12

First release: PHP, Node.js, Python, static and proxy websites; Let's Encrypt SSL; MariaDB/MySQL and phpMyAdmin; email and the mpn-mail 1.0.0 web mail; backups; firewall, ModSecurity, fail2ban, 2FA; monitoring; Vietnamese and English interface.

[1.2.0]: https://github.com/marixdev/mini-panel/compare/v1.1.1...v1.2.0
[1.1.1]: https://github.com/marixdev/mini-panel/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/marixdev/mini-panel/compare/v1.0.3...v1.1.0
[1.0.3]: https://github.com/marixdev/mini-panel/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/marixdev/mini-panel/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/marixdev/mini-panel/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/marixdev/mini-panel/releases/tag/v1.0.0
