// Package provision builds the plan that puts one site on a server: account, directories, runtime and nginx.
package provision

import (
	"fmt"
	"strings"

	"mpn/internal/installer"
	"mpn/internal/site"
	"mpn/internal/tune"
	"mpn/internal/unit"
	"mpn/internal/vhost"
)

type Options struct {
	Profile   tune.Profile
	TLS       bool
	CertDir   string // default /etc/mpn/certs/<name>
	NodeMemMB int    // MemoryMax for node apps, 0 for no cap
}

// LogDir holds a site's nginx logs. It belongs to root and not to the site user: nginx's master process (root) opens
// the files, so in a directory the user controls he could swap in a symlink and make root change the owner of, or
// append to, any file (reproduced on all four supported systems: nginx's reopen changed a root-owned file's owner).
// The user's group may read the directory but cannot create, rename or delete entries in it.
func LogDir(name string) string { return "/var/log/mpn/sites/" + name }

func logDirAction(s site.Site) installer.Action {
	return installer.Run("install", "-d", "-o", "root", "-g", s.User, "-m", "0750", LogDir(s.Name))
}

// createRootScript creates the web root only when nothing is there yet. Running chmod or install on an existing path
// as root would follow a symlink the site user had put in its place (the user owns the parent directory), so a
// later apply must never touch it. A dangling symlink counts as present.
func createRootScript(user, root string) string {
	one := func(p string) string {
		return fmt.Sprintf("if [ ! -e %[2]s ] && [ ! -L %[2]s ]; then install -d -o %[1]s -g www-data %[2]s && chmod 2750 %[2]s; fi", user, p)
	}
	// A root below htdocs (public, for Laravel) needs every folder on the way, and htdocs itself first.
	base := site.HtdocsOf(user)
	cmds := []string{one(base)}
	if sub, ok := (site.Site{User: user, Root: root}).RootSub(); ok && sub != "" {
		p := base
		for _, el := range strings.Split(sub, "/") {
			p += "/" + el
			cmds = append(cmds, one(p))
		}
	} else if root != base {
		cmds = append(cmds, one(root))
	}
	return strings.Join(cmds, "; ")
}

// accountMark goes into the comment field of every account mpn creates. mpn changes or deletes only an account that
// carries the mark of that very site, so it can never take over or remove an account it did not make (a system
// account, an administrator, another site's user).
func accountMark(siteName string) string { return "mpn site " + siteName }

// AccountMark is the text mpn puts in the comment field of the account it creates for a site.
func AccountMark(siteName string) string { return accountMark(siteName) }

// createAccountScript makes the site's own account. If the account exists already it must be this site's own (a repeated
// apply); anything else stops the plan before a directory is touched. user and siteName have passed site.Validate.
func createAccountScript(user, siteName string) string {
	return fmt.Sprintf(`if id -u %[1]s >/dev/null 2>&1; then [ "$(getent passwd %[1]s | cut -d: -f5)" = '%[2]s' ] || { echo 'account %[1]s already exists and was not created by mpn for this site; choose another -user' >&2; exit 1; }; else useradd --create-home --home-dir /home/%[1]s --shell /usr/sbin/nologin --comment '%[2]s' %[1]s; fi`,
		user, accountMark(siteName))
}

// removeAccountScript deletes the account and its home directory, but only if it carries this site's mark.
func removeAccountScript(user, siteName string) string {
	return fmt.Sprintf(`if [ "$(getent passwd %[1]s | cut -d: -f5)" = '%[2]s' ]; then loginctl terminate-user %[1]s >/dev/null 2>&1 || true; pkill -u %[1]s || true; for i in 1 2 3 4 5 6 7 8 9 10; do pgrep -u %[1]s >/dev/null || break; [ "$i" -lt 4 ] || pkill -KILL -u %[1]s || true; sleep 0.5; done; userdel -r %[1]s; else echo 'account %[1]s was not created by mpn for this site, left alone'; fi`,
		user, accountMark(siteName))
}

// CertDir is where a site's certificate files live.
func CertDir(name string) string { return "/etc/mpn/certs/" + name }

// VhostPlan writes the nginx configuration of a site again, with HTTPS if the site has it, and reloads nginx. A change of the
// domain or of its redirects touches nothing else: not the account, the PHP pool or the service.
func VhostPlan(s site.Site, certDir string) (installer.Plan, error) {
	if certDir == "" {
		certDir = CertDir(s.Name)
	}
	conf, err := vhost.Render(s, vhost.Options{TLS: s.TLS, CertDir: certDir})
	if err != nil {
		return nil, err
	}
	return installer.Plan{{Name: "nginx vhost", Actions: []installer.Action{
		logDirAction(s),
		installer.WriteFile("/etc/nginx/sites-enabled/"+s.Name+".conf", conf, 0o644),
		installer.Run("nginx", "-t"),
		installer.ReloadNginx(),
	}}}, nil
}

// TLSPlan writes a site's certificate files and, when enable is set, switches its vhost to HTTPS.
// The private key is readable by root only. Without enable nothing serves the certificate yet.
func TLSPlan(s site.Site, chainPEM, keyPEM []byte, enable bool) (installer.Plan, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	dir := CertDir(s.Name)
	plan := installer.Plan{{Name: "certificate files", Actions: []installer.Action{
		logDirAction(s), // a site made before logs moved out of its home still needs the new directory for nginx -t
		installer.WriteFile(dir+"/fullchain.pem", string(chainPEM), 0o644),
		installer.WriteFile(dir+"/privkey.pem", string(keyPEM), 0o600),
	}}}
	if !enable {
		return plan, nil
	}
	s.TLS = true
	conf, err := vhost.Render(s, vhost.Options{TLS: true, CertDir: dir})
	if err != nil {
		return nil, err
	}
	return append(plan, installer.Step{Name: "nginx vhost (https)", Actions: []installer.Action{
		installer.WriteFile("/etc/nginx/sites-enabled/"+s.Name+".conf", conf, 0o644),
		installer.Run("nginx", "-t"),
		installer.ReloadNginx(),
	}}), nil
}

// RemovePlan returns the steps that take a site off the server. The account and its files are kept unless
// purge is set, because deleting them cannot be undone. A web root outside the user's home is never deleted.
func RemovePlan(s site.Site, purge bool) (installer.Plan, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	plan := installer.Plan{{Name: "nginx vhost", Actions: []installer.Action{
		installer.Run("rm", "-f", "/etc/nginx/sites-enabled/"+s.Name+".conf"),
		installer.Run("nginx", "-t"),
		installer.ReloadNginx(),
	}}}
	switch s.Kind {
	case site.PHP:
		v, dir := s.PHPVersion, "/etc/php/"+s.PHPVersion+"/fpm/pool.d/"
		plan = append(plan, installer.Step{Name: "php-fpm pool", Actions: []installer.Action{
			installer.Run("rm", "-f", dir+s.Name+".conf"),
			// The master stays only while another pool still uses this PHP version.
			installer.Run("sh", "-c", fmt.Sprintf("if ls %[1]s*.conf >/dev/null 2>&1; then systemctl reload-or-restart php%[2]s-fpm; else systemctl disable --now php%[2]s-fpm; fi", dir, v)),
		}})
	case site.Node, site.Python:
		plan = append(plan, installer.Step{Name: string(s.Kind) + " service", Actions: []installer.Action{
			installer.Run("systemctl", "disable", "--now", unit.Name(s)),
			installer.Run("rm", "-f", "/etc/systemd/system/"+unit.Name(s)),
			installer.Run("systemctl", "daemon-reload"),
		}})
	}
	if purge {
		plan = append(plan, installer.Step{Name: "account and files", Actions: []installer.Action{
			installer.Run("sh", "-c", removeAccountScript(s.User, s.Name)),
			installer.Run("rm", "-rf", LogDir(s.Name)),
		}})
	}
	return plan, nil
}

// Plan validates the site and returns the steps to provision it. Every value placed on a command line or
// in a file has already passed site.Validate.
func Plan(s site.Site, o Options) (installer.Plan, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if o.CertDir == "" {
		o.CertDir = CertDir(s.Name)
	}
	home := "/home/" + s.User
	// The home directory can be traversed but not listed. The web root belongs to the site user and to group
	// www-data (setgid), so nginx can read it and other sites' users cannot.
	plan := installer.Plan{{Name: "account and directories", Actions: []installer.Action{
		installer.Run("sh", "-c", createAccountScript(s.User, s.Name)),
		installer.Run("chmod", "711", home),
		installer.Run("sh", "-c", createRootScript(s.User, s.Root)),
		logDirAction(s),
	}}}

	switch s.Kind {
	case site.PHP:
		v := s.PHPVersion
		pool := "/etc/php/" + v + "/fpm/pool.d/"
		plan = append(plan, installer.Step{Name: "php-fpm pool", Actions: []installer.Action{
			// The package's default pool would keep idle workers alive for no site.
			installer.Run("sh", "-c", fmt.Sprintf("[ ! -e %[1]swww.conf ] || mv %[1]swww.conf %[1]swww.conf.disabled", pool)),
			installer.WriteFile(pool+s.Name+".conf", o.Profile.PHPPoolWith(s.Name, s.User, "/run/php/"+s.Name+".sock", s.PHPPoolLines(o.Profile.PHPMemoryLimitMB), s.RequestSeconds()), 0o644),
			installer.Run("php-fpm"+v, "-t"),
			installer.Run("systemctl", "enable", "php"+v+"-fpm"),
			installer.Run("systemctl", "reload-or-restart", "php"+v+"-fpm"),
		}})
	case site.Node:
		mem := o.NodeMemMB
		if s.MemMB > 0 {
			mem = s.MemMB // the site's own limit wins over the default
		}
		content, err := unit.Node(s, mem)
		if err != nil {
			return nil, err
		}
		plan = append(plan, installer.Step{Name: "node service", Actions: []installer.Action{
			installer.WriteFile("/etc/systemd/system/"+unit.Name(s), content, 0o644),
			installer.Run("systemctl", "daemon-reload"),
			installer.Run("systemctl", "enable", unit.Name(s)),
			// Not "enable --now": a unit that is already running would keep its old version, port and memory limit.
			installer.Run("systemctl", "restart", unit.Name(s)),
		}})
	case site.Python:
		mem := o.NodeMemMB
		if s.MemMB > 0 {
			mem = s.MemMB
		}
		content, err := unit.Python(s, mem)
		if err != nil {
			return nil, err
		}
		plan = append(plan, installer.Step{Name: "python service", Actions: []installer.Action{
			installer.Run("sh", "-c", venvScript(s)),
			installer.WriteFile("/etc/systemd/system/"+unit.Name(s), content, 0o644),
			installer.Run("systemctl", "daemon-reload"),
			installer.Run("systemctl", "enable", unit.Name(s)),
			installer.Run("systemctl", "restart", unit.Name(s)),
		}})
	}

	conf, err := vhost.Render(s, vhost.Options{TLS: o.TLS || s.TLS, CertDir: o.CertDir})
	if err != nil {
		return nil, err
	}
	plan = append(plan, installer.Step{Name: "nginx vhost", Actions: []installer.Action{
		installer.WriteFile("/etc/nginx/sites-enabled/"+s.Name+".conf", conf, 0o644),
		installer.Run("nginx", "-t"),
		installer.ReloadNginx(),
	}})
	return plan, nil
}

// venvScript makes the site's virtual environment with the interpreter of the version chosen, as the site's own user. An
// environment made for another version is upgraded in place, so the packages stay. Every value in it has passed Validate.
func venvScript(s site.Site) string {
	py, venv := "/usr/bin/python"+s.PyVersion, site.VenvDir(s.User)
	return fmt.Sprintf(`py=%[1]s; venv=%[2]s
[ -x "$py" ] || { echo "Python %[3]s is not installed on this machine (install the package python%[3]s)" >&2; exit 2; }
if [ -x "$venv/bin/python" ]; then
  [ "$("$venv/bin/python" -c 'import sys; print("%%d.%%d" %% sys.version_info[:2])')" = %[3]s ] && exit 0
  action=--upgrade
else
  action=
fi
runuser -u %[4]s -- "$py" -m venv $action "$venv" || { echo "the virtual environment could not be made (install the package python%[3]s-venv)" >&2; exit 2; }`,
		py, venv, s.PyVersion, s.User)
}

// dropPHPPoolActions take a site's pool out of one PHP version. The master stays only while another pool still uses it.
func dropPHPPoolActions(s site.Site, version string) []installer.Action {
	dir := "/etc/php/" + version + "/fpm/pool.d/"
	return []installer.Action{
		installer.Run("rm", "-f", dir+s.Name+".conf"),
		installer.Run("sh", "-c", fmt.Sprintf("if ls %[1]s*.conf >/dev/null 2>&1; then systemctl reload-or-restart php%[2]s-fpm; else systemctl disable --now php%[2]s-fpm; fi", dir, version)),
	}
}

// ChangePlan puts a site that is already provisioned into its new state. Most changes (the web root, the PHP settings,
// the port or memory of a node app) are the same steps as making the site, repeated.
//
// Moving a PHP site to another version is different. The old and the new pool have the same socket path, and an old master
// that is told to drop its pool removes that path, so the old pool must go before the new master creates the socket. But it
// must not go before the new pool is known to work: if the new version cannot start the pool, the site would have none. So
// the new pool is written and tested first, and the old one is dropped just before the new master is started.
func ChangePlan(old, cur site.Site, o Options) (installer.Plan, error) {
	if err := old.Validate(); err != nil {
		return nil, err
	}
	next, err := Plan(cur, o)
	if err != nil {
		return nil, err
	}
	if old.Kind != site.PHP || cur.Kind != site.PHP || old.PHPVersion == cur.PHPVersion {
		return next, nil
	}
	for si, st := range next {
		if st.Name != "php-fpm pool" {
			continue
		}
		for ai, a := range st.Actions {
			if len(a.Args) >= 2 && a.Args[0] == "systemctl" && a.Args[1] == "enable" {
				acts := append([]installer.Action{}, st.Actions[:ai]...)
				acts = append(acts, dropPHPPoolActions(old, old.PHPVersion)...)
				next[si].Actions = append(acts, st.Actions[ai:]...)
				return next, nil
			}
		}
	}
	return nil, fmt.Errorf("the plan for a php site has no step that starts its pool")
}
