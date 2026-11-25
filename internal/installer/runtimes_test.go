package installer

import (
	"strings"
	"testing"

	"mpn/internal/distro"
	"mpn/internal/osinfo"
)

func TestPHPAndNodeSteps(t *testing.T) {
	o := opts("mariadb", 1024)
	o.PHPVersions, o.NodeVersions = []string{"7.4", "8.4"}, []string{"22", "24"}
	p, err := Build(o)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(p), "base packages,journald limit,nginx,nginx pagespeed,php 7.4,8.4,node 22,node 24,database mariadb"; got != want {
		t.Errorf("steps=%s want %s", got, want)
	}
	s := p.String()
	for _, want := range []string{
		"php7.4-opcache", "php7.4-fpm", "php8.4-fpm",
		"write /etc/php/8.4/fpm/conf.d/99-mpn.ini", "systemctl disable --now php8.4-fpm",
		"sh -c set -eu ... (multi-line script)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "php8.4-opcache") {
		t.Error("opcache is built in from 8.0 and must not be requested as a package")
	}
}

func TestNodeScriptNamesTheMajor(t *testing.T) {
	st, err := nodeStep("22")
	if err != nil {
		t.Fatal(err)
	}
	script := st.Actions[len(st.Actions)-1].Args[2]
	if !strings.Contains(script, "major=22\n") || strings.Contains(script, "@MAJOR@") {
		t.Errorf("script:\n%s", script)
	}
}

func TestUnknownRuntimeVersionsFail(t *testing.T) {
	o := opts("", 1024)
	o.PHPVersions = []string{"5.4"}
	if _, err := Build(o); err == nil {
		t.Error("php 5.4 must be rejected")
	}
	o = opts("", 1024)
	o.NodeVersions = []string{"16"}
	if _, err := Build(o); err == nil {
		t.Error("node 16 must be rejected")
	}
}

// PHP 5.6 comes from Sury on Debian and Ubuntu, with json as a package of its own and the opcache file without the JIT key.
func TestPHP56OnDebianComesFromSuryWithJSON(t *testing.T) {
	o := opts("", 1024)
	o.PHPVersions = []string{"5.6"}
	p, err := Build(o)
	if err != nil {
		t.Fatal(err)
	}
	all := names(p)
	var cmds []string
	for _, st := range p {
		for _, a := range st.Actions {
			cmds = append(cmds, strings.Join(a.Args, " ")+"\n"+a.Path+"\n"+a.Content)
		}
	}
	text := strings.Join(cmds, "\n")
	for _, want := range []string{"php5.6-fpm", "php5.6-json", "php5.6-mysql", "php5.6-opcache", "/etc/php/5.6/fpm/conf.d/99-mpn.ini"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in the php step (%s)", want, all)
		}
	}
	if strings.Contains(text, "opcache.jit") {
		t.Error("PHP 5.6 has no JIT; the opcache file must not name it")
	}
}

func TestPHP56OnELIsTheBuildOfMpnPackages(t *testing.T) {
	distro.SetForTest(distro.EL) // OwnPHP asks the family of the running system
	t.Cleanup(func() { distro.SetForTest(distro.Debian) })
	for _, el := range []osinfo.Target{{ID: "almalinux", Version: "9", Codename: "el9"}, {ID: "rocky", Version: "10", Codename: "el10"}} {
		acts, err := elPHPActions(el, "5.6", nil)
		if err != nil {
			t.Fatalf("%s: %v", el.Codename, err)
		}
		var text string
		for _, a := range acts {
			text += strings.Join(a.Args, " ") + "\n"
		}
		name := "php-5.6.40-" + el.Codename + archSuffix() + ".tar.xz"
		if !strings.Contains(text, name) || elSums[name] == "" {
			t.Errorf("%s: %s is not fetched or has no checksum", el.Codename, name)
		}
	}
}

func TestSwapReportedSlightlySmallIsEnough(t *testing.T) {
	p, _ := Build(opts("", 1023)) // the kernel shows a 1024MB swap file as 1023MB
	if strings.Contains(names(p), "swap") {
		t.Errorf("no swap step expected: %s", names(p))
	}
}
