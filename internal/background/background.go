// Package background checks and renders the background services of a website: long-running programs (queue workers,
// schedulers, bots) that systemd keeps running. A service runs as the website's own account, with the runtime of the site's
// kind (PHP CLI, Node.js or the python of the site's virtual environment). Its arguments come from a person, so they are
// parsed into argv here and never pass through a shell: a unit file is written as root, and one stray character could run
// another command.
package background

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"mpn/internal/distro"
	"mpn/internal/site"
	"mpn/internal/unit"
)

// Limits of a site's services and of their arguments.
const (
	MaxServices  = 10
	MaxArgs      = 64
	MaxArgLen    = 512
	MaxArgsText  = 2048
	MinMemoryMB  = 32
	MaxMemoryMB  = 4096
	nameMaxRunes = 48
)

// Service is one background service of a site. Args is the text the person wrote; it becomes argv only in ParseArgs.
// State is what systemd says now (active, failed, inactive or unknown); the store does not keep it.
type Service struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Args     string `json:"args"`
	Enabled  bool   `json:"enabled"`
	MemoryMB int    `json:"memory_mb"`
	State    string `json:"state,omitempty"`
}

// Input is what the CLI reads on standard input for add and set. The arguments can hold secrets, so they never go in argv.
type Input struct {
	Name     string `json:"name"`
	Args     string `json:"args"`
	Enabled  bool   `json:"enabled"`
	MemoryMB int    `json:"memory_mb"`
}

// nameRe allows letters, digits, a space, underscore, dot and dash, and the name starts with a letter or digit.
var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]*$`)

// forbidden are the characters refused outright, wherever they are, even inside quotes. None of them is needed by a
// command that runs without a shell, and each one is a way to start another command in a shell.
const forbidden = ";&|<>$`(){}"

// ValidName checks the name of a service: 1 to 48 characters.
func ValidName(name string) error {
	switch {
	case utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > nameMaxRunes:
		return fmt.Errorf("the name must be 1 to %d characters", nameMaxRunes)
	case !nameRe.MatchString(name):
		return fmt.Errorf("the name may use letters, digits, a space, _, . and -, and must start with a letter or digit")
	}
	return nil
}

// ValidMemory checks a memory limit in MB. 0 means no limit.
func ValidMemory(mb int) error {
	if mb != 0 && (mb < MinMemoryMB || mb > MaxMemoryMB) {
		return fmt.Errorf("the memory limit must be 0 (none) or between %d and %d MB", MinMemoryMB, MaxMemoryMB)
	}
	return nil
}

// ParseArgs turns the text of the arguments into argv. Spaces and tabs separate arguments. Single quotes keep everything
// between them, double quotes keep everything except a backslash, and a backslash outside single quotes takes the next
// character as it is. The result is checked by ValidArgs; this function only splits.
func ParseArgs(text string) ([]string, error) {
	if len(text) > MaxArgsText {
		return nil, fmt.Errorf("the arguments are longer than %d characters", MaxArgsText)
	}
	if !utf8.ValidString(text) {
		return nil, fmt.Errorf("the arguments must be valid text")
	}
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == 0 || c == '\r' || c == '\n' || (c < 0x20 && c != '\t') {
			return nil, fmt.Errorf("the arguments must be one line of text without control characters")
		}
		if strings.IndexByte(forbidden, c) >= 0 {
			return nil, fmt.Errorf("the arguments must not contain the character %q", c)
		}
	}
	var args []string
	var cur strings.Builder
	inArg := false
	var quote byte // the quote mark that is open, or 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
		case c == '\\':
			if i+1 >= len(text) {
				return nil, fmt.Errorf("a backslash must be followed by a character")
			}
			i++
			cur.WriteByte(text[i])
			inArg = true
		case quote == '"':
			if c == '"' {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote = c
			inArg = true
		case c == ' ' || c == '\t':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteByte(c)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("a quote is not closed")
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args, nil
}

// ValidArgs checks argv for a site of the given kind. The first argument is the script or module the runtime starts. It is
// relative to the working directory: an absolute path or a ".." segment would point outside the site. A dash in front of it
// is refused, because it would be read as an option of the runtime; the one exception is "-m module" for python.
func ValidArgs(kind site.Kind, args []string) error {
	if len(args) < 1 || len(args) > MaxArgs {
		return fmt.Errorf("a service has 1 to %d arguments", MaxArgs)
	}
	for _, a := range args {
		if n := utf8.RuneCountInString(a); n < 1 || n > MaxArgLen {
			return fmt.Errorf("each argument is 1 to %d characters", MaxArgLen)
		}
	}
	first := args[0]
	if strings.HasPrefix(first, "-") {
		if kind != site.Python || first != "-m" || len(args) < 2 || strings.HasPrefix(args[1], "-") {
			return fmt.Errorf("the first argument must be the script to run, not an option (only python accepts -m module)")
		}
	}
	if strings.HasPrefix(first, "/") {
		return fmt.Errorf("the first argument must be relative to the website's folder, not an absolute path")
	}
	for _, seg := range strings.Split(first, "/") {
		if seg == ".." {
			return fmt.Errorf("the first argument must not go up with ..")
		}
	}
	return nil
}

// ValidService checks a service of a site of the given kind: name, arguments and memory limit.
func ValidService(kind site.Kind, in Input) error {
	if err := ValidName(in.Name); err != nil {
		return err
	}
	args, err := ParseArgs(in.Args)
	if err != nil {
		return err
	}
	if err := ValidArgs(kind, args); err != nil {
		return err
	}
	return ValidMemory(in.MemoryMB)
}

// Supported refuses the kinds that cannot run a background service: a static site has no runtime, and a proxy only forwards.
func Supported(kind site.Kind) error {
	switch kind {
	case site.PHP, site.Node, site.Python:
		return nil
	}
	return fmt.Errorf("a %s site cannot have background services; only php, node and python sites can", kind)
}

// UnitName is the name of the unit file of a service.
func UnitName(siteName string, id int64) string {
	return fmt.Sprintf("app-%s-bg-%d.service", siteName, id)
}

// UnitDir is where systemd reads the unit files of the services.
const UnitDir = "/etc/systemd/system"

// UnitFile is the path of the unit file of a service.
func UnitFile(siteName string, id int64) string {
	return UnitDir + "/" + UnitName(siteName, id)
}

// StrayUnits lists the unit files of a site in dir whose service number is not in keep. Such a file belongs to a service
// that is no longer in the database, so systemd must not start it again. A file whose name does not end in a service number
// is not a service of this site and is left alone.
func StrayUnits(dir, siteName string, keep map[int64]bool) ([]string, error) {
	prefix := "app-" + siteName + "-bg-"
	matches, err := filepath.Glob(filepath.Join(dir, prefix+"*.service"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, path := range matches {
		num := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), prefix), ".service")
		if num == "" || strings.Trim(num, "0123456789") != "" {
			continue
		}
		id, err := strconv.ParseInt(num, 10, 64)
		if err != nil || id < 1 {
			continue
		}
		if !keep[id] {
			out = append(out, path)
		}
	}
	return out, nil
}

// StateOf turns the answer of `systemctl is-active` into the state the tab shows. Anything else, including a missing
// systemctl, is "unknown".
func StateOf(answer string) string {
	switch strings.TrimSpace(answer) {
	case "active", "activating", "reloading":
		return "active"
	case "failed":
		return "failed"
	case "inactive", "deactivating":
		return "inactive"
	}
	return "unknown"
}

// RemoveScript stops, disables and deletes every background unit of a site. It is a shell line for the removal of a site,
// so the name must already have passed site validation. A missing match is skipped.
func RemoveScript(siteName string) string {
	return `for f in /etc/systemd/system/app-` + siteName + `-bg-*.service; do [ -e "$f" ] || continue; systemctl disable --now "$(basename "$f")" || true; rm -f "$f"; done; systemctl daemon-reload`
}

// interpreter is the program that runs the service: the runtime of the site's kind.
func interpreter(s site.Site) (string, error) {
	switch s.Kind {
	case site.PHP:
		if !site.ValidPHPVersion(s.PHPVersion) {
			return "", fmt.Errorf("site %q has no valid PHP version", s.Name)
		}
		return distro.PHPCLIBinary(s.PHPVersion), nil
	case site.Node:
		if !site.ValidNodeVersion(s.NodeVersion) {
			return "", fmt.Errorf("site %q has no valid Node.js version", s.Name)
		}
		return unit.NodeRoot + "/" + s.NodeVersion + "/bin/node", nil
	case site.Python:
		return site.VenvDir(s.User) + "/bin/python", nil
	}
	return "", Supported(s.Kind)
}

// quote makes one argument safe for ExecStart: systemd reads a double-quoted word with \ and " escaped, and expands %
// specifiers unless each % is doubled.
func quote(a string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`).Replace(a) + `"`
}

// Unit renders the unit file of one service of site s. The service must be valid for the site's kind.
func Unit(s site.Site, svc Service) (string, error) {
	if err := Supported(s.Kind); err != nil {
		return "", err
	}
	if err := s.Validate(); err != nil {
		return "", err
	}
	if err := ValidService(s.Kind, Input{Name: svc.Name, Args: svc.Args, Enabled: svc.Enabled, MemoryMB: svc.MemoryMB}); err != nil {
		return "", err
	}
	args, err := ParseArgs(svc.Args)
	if err != nil {
		return "", err
	}
	bin, err := interpreter(s)
	if err != nil {
		return "", err
	}
	argv := []string{quote(bin)}
	for _, a := range args {
		argv = append(argv, quote(a))
	}
	dir := site.HtdocsOf(s.User)
	lines := []string{
		"[Unit]",
		"Description=Background service " + svc.Name + " of website " + s.Name,
		"After=network.target",
	}
	if s.Kind != site.PHP {
		lines = append(lines, "After="+unit.Name(s), "Wants="+unit.Name(s))
		dir = s.Root
	}
	lines = append(lines, "", "[Service]", "User="+s.User, "WorkingDirectory="+dir)
	switch s.Kind {
	case site.Node:
		lines = append(lines, "Environment=NODE_ENV=production")
	case site.Python:
		lines = append(lines, "Environment=PYTHONUNBUFFERED=1")
	}
	lines = append(lines,
		"ExecStart="+strings.Join(argv, " "),
		"Restart=always",
		"RestartSec=3",
		"NoNewPrivileges=yes",
		"PrivateTmp=yes",
		"ProtectSystem=full",
	)
	if svc.MemoryMB > 0 {
		lines = append(lines, fmt.Sprintf("MemoryMax=%dM", svc.MemoryMB))
	}
	lines = append(lines, "", "[Install]", "WantedBy=multi-user.target", "")
	return strings.Join(lines, "\n"), nil
}
