<p align="center"><img src="logo.svg" alt="Mini Panel" width="72"></p>

<h1 align="center">Mini Panel</h1>

<p align="center">
A lightweight hosting control panel for small VPS, written in Go. One binary, runs well on 1 vCPU / 1 GB RAM.<br>
<a href="https://mpanel.biz">mpanel.biz</a> · <a href="https://mpanel.biz/install/">Install</a> · <a href="https://github.com/marixdev/mini-panel/releases">Releases</a>
</p>

<p align="center"><a href="README.md">Tiếng Việt</a> · <b>English</b> · <a href="README.zh.md">简体中文</a></p>

---

## Install

Log in to the VPS as root and run:

```sh
curl -fsSL https://mpanel.biz/install.sh | sh
```

The installer checks the operating system, downloads `mpn` from the latest GitHub release and compares its SHA-256 with `SHA256SUMS`. It then installs nginx 1.28.3 with the PageSpeed module, and the panel on port 2610 with a self-signed certificate. The ModSecurity module is installed only when you turn on the web application firewall in the Security tab. The installer does **not** install PHP, Node.js or a database: you choose those later on the panel's Services page.

When it finishes, open the address it prints (`https://<IP>:2610/setup`) to create the administrator account. Do this right away: until an account exists, whoever opens `/setup` first creates it.

Options: `MPN_VERSION=1.0.2` installs a fixed version, `MPN_PANEL_PORT=2610` changes the panel port.

**Operating systems:** Ubuntu 22.04, 24.04, 26.04, Debian 12, 13, and AlmaLinux, Rocky Linux, CentOS Stream 9, 10, on x86_64 or arm64 (aarch64). On the RHEL family SELinux stays on and firewalld is turned off (the panel manages the firewall with nftables).

## Features

- **Websites:** PHP 7.0 to 8.5 (one PHP-FPM pool per site), Node.js 18 to 24, Python, static sites and reverse proxies. Each site has its own system account, domain, SSH/SFTP, file manager, web terminal, cron, logs and an editable vHost (checked with `nginx -t` before it is loaded).
- **App installer:** WordPress, Laravel, CodeIgniter 4, CakePHP, Yii 2, plus a WordPress manager (updates, plugins, themes).
- **SSL:** Let's Encrypt with a DNS check first and automatic renewal, custom certificates, CSR generation. The panel has its own certificate as well.
- **Databases:** MariaDB or MySQL, one user per database, phpMyAdmin that signs in with a temporary account.
- **Email** (off by default): Postfix, Dovecot, OpenDKIM. Domains, mailboxes and aliases are managed inside each website, with ready MX/SPF/DKIM/DMARC records that can be added straight to Cloudflare. The [mpn-mail](https://github.com/marixdev/mpn-mail) web mail exits when nobody uses it.
- **Backups:** nightly, 7 kept, restore from the interface, sync to Google Drive.
- **Security:** an nftables firewall that cannot lock out SSH or the panel port, ModSecurity with OWASP CRS, fail2ban, two-factor sign-in (TOTP).
- **Monitoring:** CPU, memory, load, network, disk and service status over 24 hours.
- **Interface** in Vietnamese, English and Simplified Chinese, light and dark.

## How it works

- `mpn web` serves the interface as a user without root rights, listens on `127.0.0.1` only and sits behind nginx with TLS.
- `mpn root` is a helper running as root. It listens on a Unix socket and accepts only a fixed list of operations. Every value is checked again before it becomes a command-line argument, and no shell is involved.
- Passwords, private keys and certificates travel over standard input, never in arguments, logs or the database.
- Every change to the machine is a plan of steps. By default it is only shown, and it runs with `-apply`. If `nginx -t` or a step fails, the old files are restored.
- nginx is pinned at 1.28.3 on every operating system (from nginx.org, held with `apt-mark hold` or `dnf versionlock`), so `apt upgrade` or `dnf upgrade` does not break the modules built for that version ([mpn-packages](https://github.com/marixdev/mpn-packages)). Every download is compared with a SHA-256 written in the source.
- The panel's data is in SQLite, `/var/lib/mpn/mpn.db`, mode 0600.

## Companion repositories

The panel downloads these components from their releases and checks the SHA-256 pinned in the source:

| Repository | Used by the panel for | Pinned in |
|---|---|---|
| [marixdev/mpn-mail](https://github.com/marixdev/mpn-mail) | Web mail at `/webmail/` (a customised lilmail) | `internal/mail/webmail.go` |
| [marixdev/mpn-packages](https://github.com/marixdev/mpn-packages) | PageSpeed and ModSecurity modules built for nginx 1.28.3; PHP 7.0 to 7.3 and the OWASP CRS for AlmaLinux, Rocky Linux, CentOS Stream | `internal/installer/nginx.go`, `internal/installer/el.go` |

## Command line

```
mpn version | tune | plan | site | db | backup | cron | firewall | security | panel | cloud | app | wp | mail | services | root | web
```

Add `-h` after a subcommand to see its flags. Commands that change the machine only show what they would do unless given `-apply`. Forgot the password: `mpn web passwd`. Lost the authenticator app: `mpn web 2fa-off`.

| What | Where |
|---|---|
| Binary | `/usr/local/bin/mpn` |
| Panel database | `/var/lib/mpn/mpn.db` |
| Certificates of each site | `/etc/mpn/certs/<site>/` |
| nginx logs of each site | `/var/log/mpn/sites/<site>/` |
| Backups | `/var/backups/mpn/<site>/<time>/` |
| Services | `mpn-root`, `mpn-web`, `mpn-renew.timer`, `mpn-backup.timer` |

## Building from source

Requires Go 1.27 or newer:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o mpn ./cmd/mpn
```

Releases are built by GitHub Actions when a `v*` tag is pushed ([release.yml](.github/workflows/release.yml)); the release notes come from that version's section of [CHANGELOG.md](CHANGELOG.md). After editing templates in `internal/web`, rebuild the CSS with Tailwind v4: `tailwindcss -i assets/input.css -o assets/app.css --minify`. The CodeMirror editor is built in `tools/editor`.

## Licence

Mini Panel is released under the [GNU General Public License v3.0](LICENSE). Third-party libraries and their licences are listed in [NOTICE](NOTICE).
