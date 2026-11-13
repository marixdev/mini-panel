package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mpn/internal/distro"
	"mpn/internal/installer"
	"mpn/internal/osinfo"
	"mpn/internal/security"
	"mpn/internal/store"
)

func securityUsage() {
	fmt.Fprintln(os.Stderr, "usage: mpn security <status|waf|f2b|unban> [flags]   (add -h for the flags)")
	os.Exit(2)
}

const (
	settingWAF       = "waf"
	settingF2B       = "f2b"
	settingF2BIgnore = "f2b_ignore"
)

func modsecModule() string { return distro.NginxModuleDir() + "/ngx_http_modsecurity_module.so" }

// crsVersionScript prints the version of the installed package of the OWASP rules.
// EL: the rules come from mpn-packages there, not from a package; their version is read in the RHEL work.
func crsVersionScript() string {
	if distro.IsEL() {
		// The panel puts the rules there itself on EL and writes their version next to them (see installer.elWAFActions).
		return "cat /usr/share/modsecurity-crs/.mpn-version 2>/dev/null || true"
	}
	return "dpkg-query -W -f='${Version}' modsecurity-crs 2>/dev/null || true"
}

type jailInfo struct {
	Name        string   `json:"name"`
	Failed      int      `json:"failed"`
	TotalFailed int      `json:"total_failed"`
	Banned      int      `json:"banned"`
	TotalBanned int      `json:"total_banned"`
	IPs         []string `json:"ips"`
}

// secStatus is the answer of "mpn security status -json".
type secStatus struct {
	WAFMode      string     `json:"waf_mode"`      // what the person chose
	WAFInstalled bool       `json:"waf_installed"` // the nginx module and the rule set are on the machine
	CRSVersion   string     `json:"crs_version"`
	F2BOn        bool       `json:"f2b_on"`
	F2BInstalled bool       `json:"f2b_installed"`
	F2BActive    bool       `json:"f2b_active"`
	Ignore       []string   `json:"ignore"`
	Jails        []jailInfo `json:"jails"`
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func ignoreList(st *store.Store) []string {
	v, _ := st.Setting(settingF2BIgnore, "")
	if v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

var (
	reFailed      = regexp.MustCompile(`Currently failed:\s*(\d+)`)
	reTotalFailed = regexp.MustCompile(`Total failed:\s*(\d+)`)
	reBanned      = regexp.MustCompile(`Currently banned:\s*(\d+)`)
	reTotalBanned = regexp.MustCompile(`Total banned:\s*(\d+)`)
	reIPs         = regexp.MustCompile(`Banned IP list:\s*(.*)`)
)

func num(re *regexp.Regexp, s string) int {
	if m := re.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

func readJail(name string) jailInfo {
	s := out(5*time.Second, "fail2ban-client", "status", name)
	j := jailInfo{Name: name, Failed: num(reFailed, s), TotalFailed: num(reTotalFailed, s), Banned: num(reBanned, s), TotalBanned: num(reTotalBanned, s), IPs: []string{}}
	if m := reIPs.FindStringSubmatch(s); m != nil {
		j.IPs = strings.Fields(m[1])
	}
	return j
}

func currentSecurity(st *store.Store) secStatus {
	mode, _ := st.Setting(settingWAF, security.ModeOff)
	f2b, _ := st.Setting(settingF2B, "off")
	s := secStatus{WAFMode: mode, F2BOn: f2b == "on", Ignore: ignoreList(st), Jails: []jailInfo{}}
	s.WAFInstalled = fileExists(modsecModule()) && fileExists(security.CRSRules)
	if s.WAFInstalled {
		s.CRSVersion = strings.TrimSpace(out(5*time.Second, "sh", "-c", crsVersionScript()))
	}
	if _, err := os.Stat("/usr/bin/fail2ban-client"); err == nil {
		s.F2BInstalled = true
		s.F2BActive = out(3*time.Second, "systemctl", "is-active", "fail2ban") == "active"
		if s.F2BActive {
			for _, name := range security.Jails {
				s.Jails = append(s.Jails, readJail(name))
			}
		}
	}
	return s
}

// wafPackagesPlan installs ModSecurity for the nginx this machine runs: the module built for nginx 1.28.3 from nginx.org on a system
// that uses it, and the package of the distribution on the others (see installer.WAFPackagesPlan).
func wafPackagesPlan() (installer.Plan, error) {
	raw, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return nil, err
	}
	t, err := osinfo.Parse(string(raw))
	if err != nil {
		return nil, err
	}
	// "nginx -v" prints its version on standard error.
	v, _ := exec.Command("nginx", "-v").CombinedOutput()
	return installer.WAFPackagesPlan(t, installer.NginxVersionOf(string(v)))
}

// wafLoadPlan writes the rules and turns them on in nginx. It runs after the packages are installed, because which files the rule
// set has decides what the rule file names. nginx -t runs before the reload, and a failure restores the files written here.
func wafLoadPlan(mode string) (installer.Plan, error) {
	conf, err := security.MainConfig(mode, fileExists)
	if err != nil {
		return nil, err
	}
	return installer.Plan{{Name: "web application firewall", Actions: []installer.Action{
		installer.Run("install", "-d", "-m", "0755", security.Dir+"/off.d"),
		installer.WriteFile(security.OffFile, security.OffContent, 0o644),
		// A panel written by an older mpn has no exemption yet; without it the rules would block saving code in the editor.
		installer.Run("sh", "-c", security.ExemptScript),
		installer.Run("sh", "-c", "[ -f "+security.AuditLog+" ] || install -m 0640 -o "+distro.WebUser()+" -g "+distro.LogGroup()+" /dev/null "+security.AuditLog),
		// On EL the log is var_log_t when made, which nginx may not open ("modsec_audit.log ... denied", seen on Rocky Linux 10).
		installer.Run("sh", "-c", `command -v semanage >/dev/null && [ "$(getenforce 2>/dev/null)" != Disabled ] || exit 0
semanage fcontext -a -t httpd_log_t '`+security.AuditLog+`.*' 2>/dev/null || semanage fcontext -m -t httpd_log_t '`+security.AuditLog+`.*'
restorecon `+security.AuditLog),
		installer.WriteFile(security.LogrotateFile, security.LogrotateText, 0o644),
		installer.Run("logrotate", "-d", security.LogrotateFile),
		installer.WriteFile(security.MainConf, conf, 0o644),
		installer.WriteFile(security.NginxHook, security.HookContent, 0o644),
		installer.Run("nginx", "-t"),
		installer.ReloadNginx(),
	}}}, nil
}

func wafOffPlan() installer.Plan {
	return installer.Plan{{Name: "turn the web application firewall off", Actions: []installer.Action{
		installer.Run("rm", "-f", security.NginxHook, security.MainConf, security.OffFile),
		installer.Run("nginx", "-t"),
		installer.ReloadNginx(),
	}}}
}

// f2bPlan installs fail2ban, writes the jails of mpn and starts it. fail2ban-client -t checks the whole configuration first.
func f2bPlan(ignore []string) (installer.Plan, error) {
	l := lockedPorts()
	jail, err := security.JailConfig(l.SSH, l.Panel, ignore)
	if err != nil {
		return nil, err
	}
	return installer.Plan{
		{Name: "fail2ban packages", Actions: []installer.Action{
			installer.Run("sh", "-c", distro.RefreshScript()+" || true"),
			installer.Run(distro.InstallCommand(f2bPackages()...)...),
			// On EL, fail2ban-firewalld would ban through firewalld, which the panel turns off; jail.d/mpn.conf names nftables.
			installer.Run("sh", "-c", "rm -f /etc/fail2ban/jail.d/00-firewalld.conf"),
		}},
		{Name: "fail2ban jails", Actions: []installer.Action{
			installer.WriteFile(security.PanelFilterFile, security.PanelFilter, 0o644),
			installer.WriteFile(security.ModsecFilterFile, security.ModsecFilter, 0o644),
			installer.WriteFile(security.JailFile, jail, 0o644),
			installer.Run("fail2ban-client", "-t"),
			installer.Run("systemctl", "enable", "fail2ban"),
			installer.Run("systemctl", "restart", "fail2ban"),
		}},
	}, nil
}

// f2bPackages are fail2ban, its systemd backend and nftables. On EL fail2ban comes from EPEL, and "fail2ban" there pulls in
// fail2ban-firewalld, so only the parts the jails of mpn use are asked for.
func f2bPackages() []string {
	if distro.IsEL() {
		// fail2ban-selinux lets fail2ban read the journal and change nftables while SELinux is on.
		return []string{"epel-release", "fail2ban-server", "fail2ban-systemd", "fail2ban-selinux", "python3-systemd", "nftables"}
	}
	return []string{"fail2ban", "python3-systemd", "nftables"}
}

func f2bOffPlan() installer.Plan {
	return installer.Plan{{Name: "turn fail2ban off", Actions: []installer.Action{
		installer.Run("sh", "-c", "systemctl disable --now fail2ban 2>/dev/null || true"),
	}}}
}

// runSecurity manages the web application firewall (ModSecurity with the OWASP Core Rule Set) and fail2ban. Both are installed
// when a person turns them on; nothing here is part of a fresh installation.
func runSecurity(args []string) {
	if len(args) == 0 {
		securityUsage()
	}
	sub := args[0]
	fs := flag.NewFlagSet("security "+sub, flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	asJSON := fs.Bool("json", false, "status: print JSON")
	mode := fs.String("mode", "", "waf: off, detect or block")
	on := fs.Bool("on", true, "f2b: turn fail2ban on (false turns it off)")
	ignore := fs.String("ignore", "", "f2b: addresses and networks that are never banned, separated by commas; empty keeps the saved list")
	jail := fs.String("jail", "", "unban: the jail")
	ip := fs.String("ip", "", "unban: the address")
	apply := fs.Bool("apply", false, "change this machine (Linux, root); otherwise only check")
	_ = fs.Parse(args[1:])
	st := openStore(*db)
	defer st.Close()

	switch sub {
	case "status":
		s := currentSecurity(st)
		if *asJSON {
			printJSON(s)
			return
		}
		fmt.Printf("waf: %s (installed %v, CRS %s)\nfail2ban: on %v, installed %v, running %v\n", s.WAFMode, s.WAFInstalled, s.CRSVersion, s.F2BOn, s.F2BInstalled, s.F2BActive)
		for _, j := range s.Jails {
			fmt.Printf("%s\tfailed %d\tbanned %d (total %d)\t%v\n", j.Name, j.Failed, j.Banned, j.TotalBanned, j.IPs)
		}
	case "waf":
		fail(security.ValidMode(*mode))
		if !*apply {
			fmt.Printf("# checked only. Would set the web application firewall to %q.\n", *mode)
			return
		}
		requireLinuxRoot()
		if *mode == security.ModeOff {
			fail(runPlanQuiet(wafOffPlan()))
		} else {
			pk, err := wafPackagesPlan()
			fail(err)
			fail(runPlanQuiet(pk))
			plan, err := wafLoadPlan(*mode)
			fail(err)
			fail(runPlanQuiet(plan))
		}
		fail(st.SetSetting(settingWAF, *mode))
		fmt.Printf("the web application firewall is %s\n", *mode)
	case "f2b":
		list := ignoreList(st)
		if *ignore != "" {
			var err error
			list, err = security.ValidIgnore(strings.Split(*ignore, ","))
			fail(err)
		}
		if !*apply {
			fmt.Printf("# checked only. Would turn fail2ban %s, never banning %v.\n", map[bool]string{true: "on", false: "off"}[*on], list)
			return
		}
		requireLinuxRoot()
		if !*on {
			fail(runPlanQuiet(f2bOffPlan()))
			fail(st.SetSetting(settingF2B, "off"))
			fmt.Println("fail2ban is off")
			return
		}
		plan, err := f2bPlan(list)
		fail(err)
		fail(runPlanQuiet(plan))
		fail(st.SetSetting(settingF2BIgnore, strings.Join(list, ",")))
		fail(st.SetSetting(settingF2B, "on"))
		fmt.Println("fail2ban is on")
	case "unban":
		fail(security.ValidJail(*jail))
		addr, err := security.ValidAddr(*ip)
		fail(err)
		if !*apply {
			fmt.Printf("# checked only. Would unban %s in %s.\n", addr, *jail)
			return
		}
		requireLinuxRoot()
		fail(runPlanQuiet(installer.Plan{{Name: "unban", Actions: []installer.Action{installer.Run("fail2ban-client", "set", *jail, "unbanip", addr)}}}))
		fmt.Printf("%s is no longer banned in %s\n", addr, *jail)
	default:
		securityUsage()
	}
}
