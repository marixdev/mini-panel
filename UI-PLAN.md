# Web panel redesign plan

Written on 2026-11-04. The feature list below was read from the page and menu names of an installed CloudPanel 2.5.4
(read-only, on a server of the owner's). No code, markup, style, text or image of it was copied or kept. The panel
described here is designed from the feature list and is meant to look and be organised differently.

## What that panel offers (feature groups, in my own words)

Per website:
- Create: PHP, WordPress (with installer), Node.js, Python, static, reverse proxy.
- Pages for one site: overview, domain and PHP/Node settings, raw web-server config, databases and database users,
  SSH and FTP users, file manager, cron jobs, certificates (Let's Encrypt, import), logs (access, error, PHP),
  security (blocked IPs, blocked bots, basic auth), page cache (Varnish).
Server level:
- Dashboard with server information. Users of the panel, roles, two-factor login, account settings.
- Firewall rules. Services list and restart. Instance settings.
- Notifications and an event log.
- Database servers list. Remote backup targets (S3 and compatible, Dropbox, Google Drive, SFTP, rclone).
- Cloud snapshots for five hosting providers.

## Where mpn stands

| Area | mpn today |
|---|---|
| Create static, PHP, Node, proxy sites | done (WordPress installer, Python: no) |
| Databases per site, password reset | done |
| Backups of files and databases, restore, prune | done (local disk only) |
| Let's Encrypt | done on the command line, not in the panel |
| Site and database lists, remove with confirmation | done |
| Cron jobs, firewall, monitoring | not yet (the owner's order: cron, firewall, monitoring) |
| Logs viewer, blocked IPs, basic auth | not yet |
| Panel users, two-factor login | one admin account only |
| File manager, FTP/SSH users, Varnish, cloud snapshots | not planned |

## Design rules so that it does not look like theirs

- Own name for things: "Websites", "Databases", "Backups", "Schedule", "Firewall", "Server".
- One page per website with section tabs, and a server overview built from status cards and small charts, not tables of
  links. Own colour palette, own type scale, own icons (inline SVG drawn for this project).
- Light and dark theme, readable on a phone, keyboard friendly, no colour-only meaning.
- Text in Vietnamese and English from one string table (the language to start with is the owner's choice).

## Technical approach

- Tailwind CSS v4, standalone CLI binary (no Node). Input `internal/web/assets/input.css`, output `app.css` committed
  and embedded in the Go binary. Templates move from Go strings to `internal/web/templates/*.html`, embedded too, so
  the CLI can scan them for class names.
- Content-Security-Policy stays strict: `style-src 'self'`; one small `app.js` served from the same origin
  (`script-src 'self'`) for the theme switch, copy button and confirm dialogs. No inline script, no external CDN.
- Existing behaviour and tests stay: sessions, CSRF, one-time results, typed confirmations.

## Order of work

1. Design system and page shell (navigation, header, cards, tables, forms, alerts, buttons) and restyle the current
   pages: login, websites, databases, backups, result and confirmation pages.
2. One page per website, with tabs.
3. Server overview with live status (CPU, memory, disk, services). Needs a read-only "status" operation in the helper.
4. New features in the owner's order: cron (Schedule), firewall, monitoring; then logs, blocked IPs, basic auth.
5. Real certificate for the panel, panel users with two-factor login.
