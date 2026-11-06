package site

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// The settings of a site that a person may change after it is made. They are kept apart from the identity of the site (its
// name, account and kind), which never changes.
//
// PHP settings are a fixed list. A pool takes any "php_admin_value[...]" line, including open_basedir and
// disable_functions, which are what keep one site out of another's files; a free text box would let one line undo that.
// So only the settings below can be set, each with the values it accepts.

// phpKey describes one setting a site may set.
type phpKey struct {
	Name    string
	Default string                   // used when the site does not set it; "" means "the machine's own value" (memory_limit)
	Admin   bool                     // written as php_admin_value, which the site's own code cannot change with ini_set
	Common  bool                     // one of the six every form shows
	ok      func(string) bool        // whether a value is acceptable
	Choices []string                 // the values a form offers, if the setting is a choice
	Hint    string                   // what the value looks like, for a message
	parse   func(string) (int, bool) // for ranges: the number in the value, used to compare sizes
}

var (
	sizeRe = regexp.MustCompile(`^([1-9][0-9]{0,3})([MG])$`)
	tzRe   = regexp.MustCompile(`^(UTC|[A-Z][A-Za-z]+(/[A-Za-z0-9_+-]+){1,2})$`)
)

// sizeMB reads "256M" or "2G" as megabytes, up to 4096.
func sizeMB(v string) (int, bool) {
	m := sizeRe.FindStringSubmatch(v)
	if m == nil {
		return 0, false
	}
	n, _ := strconv.Atoi(m[1])
	if m[2] == "G" {
		n *= 1024
	}
	return n, n >= 1 && n <= 4096
}

func intIn(lo, hi int) func(string) bool {
	return func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && strconv.Itoa(n) == v && n >= lo && n <= hi
	}
}

func oneOf(vals ...string) func(string) bool {
	return func(v string) bool { return slices.Contains(vals, v) }
}

var phpKeys = []phpKey{
	{Name: "memory_limit", Admin: true, Common: true, ok: func(v string) bool { _, ok := sizeMB(v); return ok }, Hint: "64M, 256M, 1G (1M to 4G)", parse: sizeMB},
	{Name: "max_execution_time", Default: "60", Admin: true, Common: true, ok: intIn(0, 3600), Hint: "seconds, 0 to 3600 (0 is no limit)"},
	{Name: "max_input_time", Default: "60", Admin: true, Common: true, ok: intIn(-1, 3600), Hint: "seconds, -1 to 3600"},
	{Name: "max_input_vars", Default: "5000", Admin: true, Common: true, ok: intIn(100, 100000), Hint: "100 to 100000"},
	{Name: "post_max_size", Default: "64M", Admin: true, Common: true, ok: func(v string) bool { _, ok := sizeMB(v); return ok }, Hint: "8M, 64M, 1G (1M to 4G)", parse: sizeMB},
	{Name: "upload_max_filesize", Default: "64M", Admin: true, Common: true, ok: func(v string) bool { _, ok := sizeMB(v); return ok }, Hint: "2M, 64M, 1G (1M to 4G)", parse: sizeMB},
	// The rest are things a site may reasonably want different. They are php_value, so the site's code can still change
	// them with ini_set, as it could with a php.ini of its own.
	{Name: "display_errors", Default: "Off", ok: oneOf("On", "Off"), Choices: []string{"Off", "On"}, Hint: "On or Off"},
	// A number, not "E_ALL": the pool's ini parser does not know the names.
	{Name: "error_reporting", Default: "24575", ok: intIn(0, 32767), Choices: []string{"24575", "24567", "32767", "7"}, Hint: "a number, 0 to 32767"},
	{Name: "date.timezone", Default: "UTC", ok: func(v string) bool { return tzRe.MatchString(v) }, Hint: "UTC or Area/City, for example Asia/Ho_Chi_Minh"},
	{Name: "session.gc_maxlifetime", Default: "1440", ok: intIn(60, 864000), Hint: "seconds, 60 to 864000"},
	{Name: "max_file_uploads", Default: "20", ok: intIn(1, 1000), Hint: "1 to 1000"},
	{Name: "short_open_tag", Default: "Off", ok: oneOf("On", "Off"), Choices: []string{"Off", "On"}, Hint: "On or Off"},
}

func findKey(name string) (phpKey, bool) {
	for _, k := range phpKeys {
		if k.Name == name {
			return k, true
		}
	}
	return phpKey{}, false
}

// PHPSetting is a setting as a form shows it.
type PHPSetting struct {
	Name, Default, Hint string
	Common              bool
	Choices             []string
}

// PHPSettings lists what can be set, in the order forms show it. memoryDefault is the machine's value for memory_limit.
func PHPSettings(memoryDefault string) []PHPSetting {
	out := make([]PHPSetting, 0, len(phpKeys))
	for _, k := range phpKeys {
		d := k.Default
		if k.Name == "memory_limit" {
			d = memoryDefault
		}
		out = append(out, PHPSetting{Name: k.Name, Default: d, Hint: k.Hint, Common: k.Common, Choices: k.Choices})
	}
	return out
}

// ValidatePHPIni checks the settings a site asks for: every name must be on the list and every value one it accepts. The
// upload limit may not exceed the size a request may have, or PHP would refuse the uploads it was told to allow.
func ValidatePHPIni(m map[string]string) error {
	var errs []string
	for _, name := range sortedKeys(m) {
		k, ok := findKey(name)
		if !ok {
			errs = append(errs, fmt.Sprintf("%q is not a setting that can be changed", name))
			continue
		}
		if !k.ok(m[name]) {
			errs = append(errs, fmt.Sprintf("%s: %q is not accepted (%s)", name, m[name], k.Hint))
		}
	}
	if len(errs) == 0 {
		up, post := effectiveMB(m, "upload_max_filesize"), effectiveMB(m, "post_max_size")
		if up > post {
			errs = append(errs, fmt.Sprintf("upload_max_filesize (%dM) may not be larger than post_max_size (%dM)", up, post))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func effectiveMB(m map[string]string, name string) int {
	k, _ := findKey(name)
	v := k.Default
	if x, ok := m[name]; ok {
		v = x
	}
	n, _ := sizeMB(v)
	return n
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// PHPValue returns the value a site uses for a setting: what it set, or the default (memoryMB for memory_limit).
func (s Site) PHPValue(name string, memoryMB int) string {
	if v, ok := s.PHPIni[name]; ok {
		return v
	}
	if name == "memory_limit" {
		return fmt.Sprintf("%dM", memoryMB)
	}
	k, _ := findKey(name)
	return k.Default
}

// PHPPoolLines returns the lines of a pool that carry the site's settings. The six common ones are always written, so what
// a form shows is what runs. memoryMB is the machine's default memory_limit.
func (s Site) PHPPoolLines(memoryMB int) []string {
	var lines []string
	for _, k := range phpKeys {
		_, set := s.PHPIni[k.Name]
		if !k.Common && !set {
			continue // the PHP default stands
		}
		kind := "php_value"
		if k.Admin {
			kind = "php_admin_value"
		}
		lines = append(lines, fmt.Sprintf("%s[%s] = %s", kind, k.Name, s.PHPValue(k.Name, memoryMB)))
	}
	return lines
}

// BodyLimit is the size nginx lets a request have: what PHP accepts, so that the two agree.
func (s Site) BodyLimit() string { return s.PHPValue("post_max_size", 0) }

// RequestSeconds is how long a PHP request may run before the pool ends it, and how long nginx waits for the answer. It is
// the script's own limit plus some room, and never less than five minutes, which the pool has always allowed.
func (s Site) RequestSeconds() int {
	n, _ := strconv.Atoi(s.PHPValue("max_execution_time", 0))
	switch {
	case n == 0:
		return 3600 // no limit for the script; the pool still ends it after an hour
	case n+30 > 300:
		return n + 30
	}
	return 300
}

// Web root inside the home. The web root of a site is the folder htdocs of its account, or a folder below it (public for
// Laravel, for example). The folder htdocs is the one mpn makes with the right owner and group.

var subdirRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,63}$`)

// HtdocsOf is the folder every web root of an account lies in.
func HtdocsOf(user string) string { return "/home/" + user + "/htdocs" }

// RootSub returns the part of the web root below htdocs ("" for htdocs itself), or false if the root lies elsewhere.
func (s Site) RootSub() (string, bool) {
	base := HtdocsOf(s.User)
	if s.Root == base {
		return "", true
	}
	if strings.HasPrefix(s.Root, base+"/") {
		return s.Root[len(base)+1:], true
	}
	return "", false
}

// ValidSubdir checks a folder below htdocs, written with "/" between names. It may be empty. No name may start with a dot
// or be "..", so it cannot reach the hidden folders of the account or leave htdocs.
func ValidSubdir(sub string) bool {
	if sub == "" {
		return true
	}
	if len(sub) > 200 {
		return false
	}
	for _, el := range strings.Split(sub, "/") {
		if !subdirRe.MatchString(el) {
			return false
		}
	}
	return true
}

// RootFor returns the web root for a folder below htdocs.
func RootFor(user, sub string) string {
	if sub == "" {
		return HtdocsOf(user)
	}
	return HtdocsOf(user) + "/" + sub
}

// CheckPHPSetting reports whether name is a setting that can be set and value is one it accepts. When it is not, hint says
// what a good value looks like (empty if the name itself is not on the list).
func CheckPHPSetting(name, value string) (hint string, ok bool) {
	k, found := findKey(name)
	if !found {
		return "", false
	}
	return k.Hint, k.ok(value)
}

// UploadLargerThanPost reports whether the settings ask for an upload size above the size of a request.
func UploadLargerThanPost(m map[string]string) bool {
	return effectiveMB(m, "upload_max_filesize") > effectiveMB(m, "post_max_size")
}
