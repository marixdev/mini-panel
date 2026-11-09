// Package apps knows the PHP applications mpn can download and set up in a new site: what each needs, where its web root is, and the
// files and commands that make it work. Like provision it only describes the work (see install.go); nothing here touches the machine.
package apps

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Pinned releases of the two tools that do the work. They are downloaded once, from the project that publishes them, and compared with
// the checksum the project publishes; a file that does not match is never installed.
const (
	WPCLIVersion     = "2.12.0"
	WPCLIURL         = "https://github.com/wp-cli/wp-cli/releases/download/v" + WPCLIVersion + "/wp-cli-" + WPCLIVersion + ".phar"
	WPCLISHA512      = "be928f6b8ca1e8dfb9d2f4b75a13aa4aee0896f8a9a0a1c45cd5d2c98605e6172e6d014dda2e27f88c98befc16c040cbb2bd1bfa121510ea5cdf5f6a30fe8832"
	WPCLIPath        = "/usr/local/bin/wp-cli.phar"
	ComposerVersion  = "2.11.1"
	ComposerURL      = "https://getcomposer.org/download/" + ComposerVersion + "/composer.phar"
	ComposerSHA256   = "6f1b891e1d4927d62740276c540fb490e47689e19e102c49e8da849794882f8c"
	ComposerPath     = "/usr/local/bin/composer.phar"
	StageDir         = "/usr/local/share/mpn/stage"
	WordPressID      = "wordpress"
	wpMemoryLimit    = "512M"
	composerMemoryMB = 768
)

// Recipe is one application.
type Recipe struct {
	ID, Label string
	MinPHP    string            // the oldest PHP version it runs on
	Package   string            // the Composer package it is made from; empty for WordPress, which WP-CLI downloads
	DocRoot   string            // the folder below htdocs that nginx serves; empty for htdocs itself
	PHPIni    map[string]string // settings that suit it, applied to the site
	Needs     []string          // PHP extensions beyond the ones every site has (package suffixes such as "intl")
}

var recipes = []Recipe{
	{ID: WordPressID, Label: "WordPress", MinPHP: "7.4", PHPIni: map[string]string{"memory_limit": "256M"}},
	{ID: "laravel", Label: "Laravel", MinPHP: "8.2", Package: "laravel/laravel", DocRoot: "public", PHPIni: map[string]string{"memory_limit": "256M"}},
	{ID: "codeigniter", Label: "CodeIgniter 4", MinPHP: "8.1", Package: "codeigniter4/appstarter", DocRoot: "public", Needs: []string{"intl"}},
	{ID: "cakephp", Label: "CakePHP", MinPHP: "8.1", Package: "cakephp/app", DocRoot: "webroot", Needs: []string{"intl"}},
	{ID: "yii", Label: "Yii 2", MinPHP: "7.4", Package: "yiisoft/yii2-app-basic", DocRoot: "web"},
}

// All lists the applications in the order a form shows them.
func All() []Recipe { return slices.Clone(recipes) }

// Get returns the recipe of an application.
func Get(id string) (Recipe, bool) {
	for _, r := range recipes {
		if r.ID == id {
			return r, true
		}
	}
	return Recipe{}, false
}

// IDs are the identifiers of All, in order.
func IDs() []string {
	out := make([]string, len(recipes))
	for i, r := range recipes {
		out[i] = r.ID
	}
	return out
}

func majorMinor(v string) (int, int, bool) {
	a, b, ok := strings.Cut(v, ".")
	if !ok {
		return 0, 0, false
	}
	x, err1 := strconv.Atoi(a)
	y, err2 := strconv.Atoi(b)
	return x, y, err1 == nil && err2 == nil
}

// PHPAtLeast reports whether version have (such as "8.4") is min or newer. Versions that do not look like x.y count as too old.
func PHPAtLeast(have, min string) bool {
	hx, hy, ok1 := majorMinor(have)
	mx, my, ok2 := majorMinor(min)
	return ok1 && ok2 && (hx > mx || hx == mx && hy >= my)
}

// What a person types for WordPress. Each value ends up in a command or a file, so each is checked here as well as by the program that
// uses it.
var (
	adminRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{2,59}$`)
	emailRe  = regexp.MustCompile(`^[A-Za-z0-9._%+-]{1,64}@[A-Za-z0-9.-]{1,190}\.[A-Za-z]{2,24}$`)
	Locales  = []string{"en_US", "vi", "fr_FR", "de_DE", "es_ES", "ja", "zh_CN"}
	titleMax = 100
)

func ValidAdminUser(s string) bool { return adminRe.MatchString(s) }
func ValidEmail(s string) bool     { return emailRe.MatchString(s) && !strings.Contains(s, "..") }
func ValidLocale(s string) bool    { return slices.Contains(Locales, s) }

// ValidTitle accepts the title of a WordPress site: some text on one line, no control characters.
func ValidTitle(s string) bool {
	if s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > titleMax || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

const saltChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$%&()*+,-./:;<=>?@[]^_{|}~"

// Salt returns 64 random characters for a key of wp-config.php. None of them is a quote or a backslash, so it sits safely in a PHP string.
func Salt() (string, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, 64)
	for i, x := range b {
		out[i] = saltChars[int(x)%len(saltChars)]
	}
	return string(out), nil
}

var wpKeys = []string{"AUTH_KEY", "SECURE_AUTH_KEY", "LOGGED_IN_KEY", "NONCE_KEY", "AUTH_SALT", "SECURE_AUTH_SALT", "LOGGED_IN_SALT", "NONCE_SALT"}

var safeValueRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

// WPConfig is wp-config.php for a database on this machine. The values are letters, digits and underscores only, which is all that a
// database name, a user and a password made by mpn contain, so nothing in them can end a PHP string.
func WPConfig(dbName, dbUser, dbPass string) (string, error) {
	for _, v := range []string{dbName, dbUser, dbPass} {
		if !safeValueRe.MatchString(v) {
			return "", fmt.Errorf("a database value has characters that cannot go into wp-config.php")
		}
	}
	var b strings.Builder
	b.WriteString("<?php\n// Written by Mini Panel.\n")
	fmt.Fprintf(&b, "define( 'DB_NAME', '%s' );\ndefine( 'DB_USER', '%s' );\ndefine( 'DB_PASSWORD', '%s' );\n", dbName, dbUser, dbPass)
	b.WriteString("define( 'DB_HOST', 'localhost' );\ndefine( 'DB_CHARSET', 'utf8mb4' );\ndefine( 'DB_COLLATE', '' );\n\n")
	for _, k := range wpKeys {
		s, err := Salt()
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "define( '%s', '%s' );\n", k, s)
	}
	b.WriteString("\n$table_prefix = 'wp_';\n\ndefine( 'WP_DEBUG', false );\ndefine( 'FS_METHOD', 'direct' );\ndefine( 'DISALLOW_FILE_EDIT', true );\n\n")
	b.WriteString("if ( ! defined( 'ABSPATH' ) ) {\n\tdefine( 'ABSPATH', __DIR__ . '/' );\n}\nrequire_once ABSPATH . 'wp-settings.php';\n")
	return b.String(), nil
}
