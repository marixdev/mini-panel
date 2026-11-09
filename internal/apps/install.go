package apps

import (
	"fmt"
	"path"
	"strings"

	"mpn/internal/installer"
	"mpn/internal/provision"
	"mpn/internal/site"
)

// Params is everything one installation needs. The values are checked (see Check) before a plan is made from them.
type Params struct {
	Site   site.Site
	Recipe Recipe
	// The database of the application: empty for an application that does not get one.
	DBName, DBUser, DBPass string
	// WordPress only.
	Title, AdminUser, AdminPass, AdminEmail, Locale string
}

// URL is the address the application is installed under. A site starts on plain HTTP; WordPress and the frameworks follow the scheme of each
// request once SSL is turned on, so the address keeps working.
func (p Params) URL() string { return "http://" + p.Site.Primary() }

// HtdocsOf is the folder the application goes into: the home of the site's web files, not a folder below it.
func HtdocsOf(p Params) string { return site.HtdocsOf(p.Site.User) }

// NeedsDB reports whether the recipe gets a database.
func (r Recipe) NeedsDB() bool {
	return r.ID == WordPressID || r.ID == "laravel" || r.ID == "codeigniter"
}

// Check refuses what cannot be installed, with a sentence that says why.
func (p Params) Check() error {
	switch {
	case p.Site.Kind != site.PHP:
		return fmt.Errorf("app-kind: applications are installed in PHP sites only")
	case p.Site.App != "":
		return fmt.Errorf("app-installed: this site has %s already", p.Site.App)
	case !PHPAtLeast(p.Site.PHPVersion, p.Recipe.MinPHP):
		return fmt.Errorf("app-php: %s needs PHP %s or newer; this site uses PHP %s", p.Recipe.Label, p.Recipe.MinPHP, p.Site.PHPVersion)
	}
	if p.Recipe.NeedsDB() {
		for _, v := range []string{p.DBName, p.DBUser, p.DBPass} {
			if !safeValueRe.MatchString(v) {
				return fmt.Errorf("the database details are missing or have characters that cannot be used")
			}
		}
	}
	if p.Recipe.ID == WordPressID {
		switch {
		case !ValidTitle(p.Title):
			return fmt.Errorf("the site title must be one line of at most %d characters", titleMax)
		case !ValidAdminUser(p.AdminUser):
			return fmt.Errorf("the administrator name must have 3 to 60 letters, digits or . _ @ -")
		case !ValidEmail(p.AdminEmail):
			return fmt.Errorf("the administrator email is not valid")
		case !ValidLocale(p.Locale):
			return fmt.Errorf("the language %q is not offered", p.Locale)
		case len(p.AdminPass) < 12 || len(p.AdminPass) > 64 || strings.ContainsAny(p.AdminPass, "\r\n\x00"):
			return fmt.Errorf("the administrator password must have 12 to 64 characters")
		}
	}
	return nil
}

// shq quotes a value for a shell script, whatever it holds.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func phpBin(p Params) string { return "/usr/bin/php" + p.Site.PHPVersion }

// asUser runs a PHP program as the site's own account, with a home directory for the caches of Composer and WP-CLI. It is an argument list,
// not a shell line, so no value is ever read as shell syntax.
func asUser(p Params, mem string, args ...string) installer.Action {
	home := "/home/" + p.Site.User
	// --chdir: the account of the site cannot enter the directory the helper was started in (/root), and WP-CLI and Composer start
	// processes of their own, which fails with "Permission denied" when the working directory is not readable.
	a := []string{"runuser", "-u", p.Site.User, "--", "env", "--chdir=" + HtdocsOf(p), "HOME=" + home, "COMPOSER_HOME=" + home + "/.composer", "COMPOSER_NO_INTERACTION=1",
		"COMPOSER_MEMORY_LIMIT=-1", phpBin(p), "-d", "memory_limit=" + mem}
	return installer.Run(append(a, args...)...)
}

func wp(p Params, args ...string) installer.Action {
	return asUser(p, wpMemoryLimit, append([]string{WPCLIPath, "--path=" + HtdocsOf(p), "--no-color"}, args...)...)
}

// WPArgv is the command that runs WP-CLI in the WordPress of a site, as the site's own user and with the PHP version of the site. It is an
// argument list for exec, never a shell line.
func WPArgv(s site.Site, args ...string) []string {
	return wp(Params{Site: s}, args...).Args
}

func composer(p Params, args ...string) installer.Action {
	return asUser(p, fmt.Sprintf("%dM", composerMemoryMB), append([]string{ComposerPath}, args...)...)
}

// toolScript puts a tool at dest unless the pinned release is there already. The download is compared with the checksum that is fixed in
// this package; a file that differs is thrown away.
func toolScript(url, sum, algo, dest string) string {
	return fmt.Sprintf(`url=%[1]s; want=%[2]s; dest=%[4]s
have() { [ -f "$dest" ] && [ "$(%[3]ssum "$dest" | cut -d' ' -f1)" = "$want" ]; }
have && exit 0
t=$(mktemp); trap 'rm -f "$t"' EXIT
curl -fsSL --retry 3 -o "$t" "$url"
[ "$(%[3]ssum "$t" | cut -d' ' -f1)" = "$want" ] || { echo "the file from $url does not match its checksum; not installed" >&2; exit 1; }
install -d -m 0755 "$(dirname "$dest")"; install -m 0644 -o root -g root "$t" "$dest"`, shq(url), shq(sum), algo, shq(dest))
}

// ToolsStep installs the tool the recipe works with: WP-CLI for WordPress, Composer for the others.
func ToolsStep(r Recipe) installer.Step {
	if r.ID == WordPressID {
		return installer.Step{Name: "WP-CLI", Actions: []installer.Action{installer.Run("sh", "-c", toolScript(WPCLIURL, WPCLISHA512, "sha512", WPCLIPath))}}
	}
	return installer.Step{Name: "Composer", Actions: []installer.Action{installer.Run("sh", "-c", toolScript(ComposerURL, ComposerSHA256, "sha256", ComposerPath))}}
}

// ExtensionsStep installs the PHP extensions a recipe needs and has the site's PHP-FPM load them. It returns false when none is needed.
func ExtensionsStep(p Params) (installer.Step, bool) {
	if len(p.Recipe.Needs) == 0 {
		return installer.Step{}, false
	}
	var pkgs []string
	for _, n := range p.Recipe.Needs {
		pkgs = append(pkgs, "php"+p.Site.PHPVersion+"-"+n)
	}
	return installer.Step{Name: "php extensions", Actions: []installer.Action{
		installer.AptInstall(pkgs...),
		installer.Run("systemctl", "reload-or-restart", "php"+p.Site.PHPVersion+"-fpm"),
	}}, true
}

// cleanRootScript removes the welcome page mpn made and stops when anything else is in the web root: an application is never put over a
// person's files. It runs as the site's user, who owns the directory.
func cleanRootScript(p Params) string {
	return fmt.Sprintf(`cd %[2]s || exit 1
runuser -u %[1]s -- sh -c 'for f in index.php index.html; do if [ -f "$f" ] && grep -q %[3]s "$f"; then rm -f "$f"; fi; done'
[ -z "$(ls -A)" ] || { echo "app-notempty: the web root has files in it; an application can only be installed in an empty web root" >&2; exit 1; }`,
		p.Site.User, shq(HtdocsOf(p)), provision.PageMarker)
}

// secretFile writes content to a file only root can read, so that a program run by the site's user can take it from standard input. The
// content never appears on a command line, where other accounts of the machine could read it.
func secretFile(name, content string) installer.Action {
	return installer.WriteFile(path.Join(StageDir, name), content, 0o600)
}

// putAs copies a stage file to dest as the site's user, in a file only that user can read. With appendTo it adds to the end instead.
func putAs(p Params, stage, dest string, appendTo bool) installer.Action {
	redir := ">"
	if appendTo {
		redir = ">>"
	}
	return installer.Run("sh", "-c", fmt.Sprintf(`runuser -u %[1]s -- sh -c 'umask 077; cat %[4]s "$0"' %[3]s < %[2]s`, p.Site.User, shq(path.Join(StageDir, stage)), shq(dest), redir))
}

// StageFiles are the files a plan puts in StageDir; a failed installation removes them.
var StageFiles = []string{"wp-config.php", "wp-admin-pass", "env-block", "home.php"}

func dropStage(names ...string) installer.Action {
	var l []string
	for _, n := range names {
		l = append(l, shq(path.Join(StageDir, n)))
	}
	return installer.Run("sh", "-c", "rm -f "+strings.Join(l, " "))
}

// CleanupPlan removes what a failed installation left: the secrets in StageDir and everything that was put in the web root. It must only be
// run after the preparation (Plan's first plan) succeeded, because from then on the web root held nothing of the person's.
func CleanupPlan(p Params) installer.Plan {
	return installer.Plan{{Name: "clean up", Actions: []installer.Action{
		dropStage(StageFiles...),
		installer.Run("sh", "-c", fmt.Sprintf(`cd %s && runuser -u %s -- find . -mindepth 1 -delete`, shq(HtdocsOf(p)), p.Site.User)),
	}}}
}

// Plan returns two plans. prep installs the tools and makes sure the web root is empty; it changes nothing that belongs to the person. install
// downloads and sets up the application, and when it fails CleanupPlan puts the web root back to empty.
func Plan(p Params) (prep, install installer.Plan, err error) {
	if err := p.Check(); err != nil {
		return nil, nil, err
	}
	prep = installer.Plan{{Name: "stage", Actions: []installer.Action{installer.Run("install", "-d", "-m", "0700", "-o", "root", "-g", "root", StageDir)}}}
	prep = append(prep, ToolsStep(p.Recipe))
	if ext, ok := ExtensionsStep(p); ok {
		prep = append(prep, ext)
	}
	prep = append(prep, installer.Step{Name: "web root", Actions: []installer.Action{installer.Run("sh", "-c", cleanRootScript(p))}})
	if p.Recipe.ID == WordPressID {
		install, err = wordpressSteps(p)
	} else {
		install, err = composerSteps(p)
	}
	if err != nil {
		return nil, nil, err
	}
	return prep, install, nil
}

func wordpressSteps(p Params) (installer.Plan, error) {
	cfg, err := WPConfig(p.DBName, p.DBUser, p.DBPass)
	if err != nil {
		return nil, err
	}
	root := HtdocsOf(p)
	return installer.Plan{
		{Name: "download WordPress", Actions: []installer.Action{wp(p, "core", "download", "--locale="+p.Locale)}},
		{Name: "wp-config.php", Actions: []installer.Action{secretFile("wp-config.php", cfg), putAs(p, "wp-config.php", path.Join(root, "wp-config.php"), false), dropStage("wp-config.php")}},
		{Name: "install WordPress", Actions: []installer.Action{
			secretFile("wp-admin-pass", p.AdminPass+"\n"),
			// WP-CLI reads the password from standard input, so it is not an argument of a process other accounts can list.
			installer.Run("sh", "-c", fmt.Sprintf(`runuser -u %[1]s -- env --chdir=%[5]s HOME=/home/%[1]s %[2]s -d memory_limit=%[3]s %[4]s --path=%[5]s --no-color core install --url=%[6]s --title=%[7]s --admin_user=%[8]s --admin_email=%[9]s --skip-email --prompt=admin_password < %[10]s`,
				p.Site.User, phpBin(p), wpMemoryLimit, WPCLIPath, shq(root), shq(p.URL()), shq(p.Title), shq(p.AdminUser), shq(p.AdminEmail), shq(path.Join(StageDir, "wp-admin-pass")))),
			dropStage("wp-admin-pass"),
		}},
		{Name: "WordPress settings", Actions: []installer.Action{
			wp(p, "rewrite", "structure", "/%postname%/", "--hard"),
			// The sample plugin and post are no use on a real site; failing to remove them is no reason to stop.
			installer.Run("sh", "-c", fmt.Sprintf(`runuser -u %[1]s -- env --chdir=%[4]s HOME=/home/%[1]s %[2]s %[3]s --path=%[4]s --no-color plugin delete hello akismet >/dev/null 2>&1 || true`,
				p.Site.User, phpBin(p), WPCLIPath, shq(root))),
		}},
	}, nil
}

// envBlock is what goes at the end of the .env file of a framework: the address, production mode and the database.
func envBlock(p Params) string {
	switch p.Recipe.ID {
	case "laravel":
		return fmt.Sprintf("\nAPP_ENV=production\nAPP_DEBUG=false\nAPP_URL=%s\nDB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3306\nDB_DATABASE=%s\nDB_USERNAME=%s\nDB_PASSWORD=%s\n",
			p.URL(), p.DBName, p.DBUser, p.DBPass)
	case "codeigniter":
		return fmt.Sprintf("\nCI_ENVIRONMENT = production\napp.baseURL = '%s/'\ndatabase.default.hostname = localhost\ndatabase.default.database = %s\ndatabase.default.username = %s\ndatabase.default.password = %s\ndatabase.default.DBDriver = MySQLi\n",
			p.URL(), p.DBName, p.DBUser, p.DBPass)
	}
	return ""
}

// inRoot runs a shell line in the web root as the site's user.
func inRoot(p Params, line string) installer.Action {
	return installer.Run("sh", "-c", fmt.Sprintf(`cd %s && runuser -u %s -- sh -c %s`, shq(HtdocsOf(p)), p.Site.User, shq(line)))
}

func composerSteps(p Params) (installer.Plan, error) {
	root := HtdocsOf(p)
	plan := installer.Plan{{Name: "download " + p.Recipe.Label, Actions: []installer.Action{
		composer(p, "create-project", "--no-interaction", "--prefer-dist", "--no-progress", "--no-dev", p.Recipe.Package, root),
	}}}
	switch p.Recipe.ID {
	case "laravel", "codeigniter":
		prep := `[ -f .env ] || cp env .env; chmod 600 .env`
		if p.Recipe.ID == "laravel" {
			// The lines of the example file for the address and the database, commented or not, make way for ours.
			prep = `[ -f .env ] || cp .env.example .env; chmod 600 .env; sed -i -E '/^#? ?(APP_ENV|APP_DEBUG|APP_URL|DB_[A-Z_]+)=/d' .env`
		}
		plan = append(plan, installer.Step{Name: "configuration", Actions: []installer.Action{
			inRoot(p, prep),
			secretFile("env-block", envBlock(p)), putAs(p, "env-block", path.Join(root, ".env"), true), dropStage("env-block"),
		}})
		if p.Recipe.ID == "laravel" {
			plan = append(plan, installer.Step{Name: "database tables", Actions: []installer.Action{
				asUser(p, "256M", path.Join(root, "artisan"), "migrate", "--force", "--no-interaction"),
			}})
		}
	case "cakephp":
		// The skeleton starts in debug mode, which loads the DebugKit plugin; it is a development package and is not installed here.
		// Without debug mode its home page refuses to show ("replace templates/Pages/home.php"), so it gets the welcome page of mpn.
		home := "<?php $this->disableAutoLayout(); ?>\n" + provision.PHPWelcome("templates/Pages/home.php")
		plan = append(plan, installer.Step{Name: "production mode", Actions: []installer.Action{
			inRoot(p, `sed -i "s/env('DEBUG', true)/env('DEBUG', false)/" config/app_local.php`),
			secretFile("home.php", home), putAs(p, "home.php", path.Join(root, "templates/Pages/home.php"), false), dropStage("home.php"),
		}})
	case "yii":
		// The sample front controller starts in debug mode; a real site must not.
		plan = append(plan, installer.Step{Name: "production mode", Actions: []installer.Action{
			inRoot(p, `sed -i -e "s/define('YII_DEBUG', true)/define('YII_DEBUG', false)/" -e "s/define('YII_ENV', 'dev')/define('YII_ENV', 'prod')/" web/index.php`),
		}})
	}
	return plan, nil
}
