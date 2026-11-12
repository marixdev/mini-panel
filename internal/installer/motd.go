package installer

// MotdScript is shown at every SSH login in place of the greeting of the distribution. pam_motd runs it through
// update-motd as root; it must stay quick, so it only reads files and runs small commands. The panel port is read from
// the panel's nginx file at each login, so a port changed later is still right. It prints ASCII only, so it looks the
// same whatever the locale of the SSH client.
const MotdScript = `#!/bin/sh
# Mini Panel login greeting, written by the installer. The greeting of the distribution is switched off;
# the scripts it used are kept (see /etc/mpn/motd-off and the execute bits in /etc/update-motd.d).
a='\033[38;5;69m'; b='\033[1m'; d='\033[2m'; r='\033[0m'
ver=$(/usr/local/bin/mpn version 2>/dev/null || echo "?")
os=$(. /etc/os-release 2>/dev/null; echo "${PRETTY_NAME:-Linux}")
up=$(uptime -p 2>/dev/null | sed 's/^up //')
ip=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{ for (i = 1; i < NF; i++) if ($i == "src") { print $(i + 1); exit } }')
port=$(sed -n 's/^ *listen \([0-9][0-9]*\) ssl.*/\1/p' /etc/nginx/conf.d/01-mpn-panel.conf 2>/dev/null | head -n 1)
load=$(cut -d' ' -f1-3 /proc/loadavg)
mem=$(free -m 2>/dev/null | awk '/^Mem:/ { print $3 " / " $2 " MB" }')
disk=$(df -h / 2>/dev/null | awk 'NR == 2 { print $3 " / " $2 }')
sites=$(find /etc/nginx/sites-enabled -maxdepth 1 -name '*.conf' 2>/dev/null | wc -l)
printf '\n'
printf "  ${a}%s${r}   ${b}Mini Panel${r} %s\n" ' /| /|' "$ver"
printf "  ${a}%s${r}   ${d}%s, up %s${r}\n\n" '/ |/ |' "$os" "${up:-?}"
printf "  ${d}%-9s${r} ${a}https://%s:%s${r}\n" 'Panel' "${ip:-<server-ip>}" "${port:-2610}"
printf "  ${d}%-9s${r} %s\n" 'Docs' 'https://mpanel.biz'
printf "  ${d}%-9s${r} %s\n\n" 'Command' 'mpn  (mpn web passwd resets the panel password)'
printf "  ${d}load${r} %s   ${d}memory${r} %s   ${d}disk${r} %s   ${d}websites${r} %s\n\n" "$load" "${mem:-?}" "${disk:-?}" "$sites"
`

// motdFile is the greeting. motdOffScript switches off the distribution's greeting (Ubuntu's news, help text and
// system information, Debian's kernel line and licence text) without deleting anything: scripts lose their execute bit,
// links (such as Ubuntu's landscape-sysinfo, whose target is outside /etc) move to /etc/mpn/motd-off, and /etc/motd is
// kept as /etc/motd.mpn-orig before it is emptied. Running it again changes nothing more.
const motdFile = "/etc/update-motd.d/00-mpn"

const motdOffScript = `set -e
install -d -m 0755 /etc/mpn/motd-off
for f in /etc/update-motd.d/*; do
  [ "$f" = ` + motdFile + ` ] && continue
  if [ -L "$f" ]; then mv "$f" /etc/mpn/motd-off/; elif [ -f "$f" ]; then chmod -x "$f"; fi
done
if [ -s /etc/motd ] && [ ! -e /etc/motd.mpn-orig ]; then cp -p /etc/motd /etc/motd.mpn-orig; fi
: > /etc/motd
rm -f /run/motd.dynamic`

func motdActions() []Action {
	return []Action{
		Run("install", "-d", "-m", "0755", "/etc/update-motd.d"),
		WriteFile(motdFile, MotdScript, 0o755),
		Run("sh", "-c", motdOffScript),
	}
}
