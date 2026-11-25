package installer

import (
	"fmt"
	"runtime"
	"strings"

	"mpn/internal/distro"
	"mpn/internal/osinfo"
)

// One nginx version on every system. A dynamic module only loads into the exact nginx it was built for, and each distribution ships a
// different nginx (1.18 to 1.28), so the panel uses nginx 1.30.7 from nginx.org and loads two modules built for that version by
// github.com/marixdev/mpn-packages. Ubuntu 26.04 kept the nginx and the packaged modules of the distribution until 1.2.2, when
// nginx.org had no package for it (see osinfo.Target.NginxOrg and NginxUpgradeActions).
const (
	NginxVersion = "1.30.7"
	// nginxSigningFPR is a fingerprint in the signing key nginx.org publishes; the key is refused when it is not there.
	nginxSigningFPR = "573BFD6B3D8FBC641079A6ABABF5BD827BD9BF62"

	modulesRelease = "nginx-1.30.7-r2"
	// The repository was called mpn-nginx-modules until 2026-11-13; GitHub sends the old address to this one.
	modulesBase = "https://github.com/marixdev/mpn-packages/releases/download/" + modulesRelease

	nginxModulesEnabled = "/etc/nginx/modules-enabled"
	nginxKeyring        = "/usr/share/keyrings/nginx-archive-keyring.gpg"
)

// moduleRelease is the release that holds the PageSpeed module of a system: nginx-1.30.7-r2, for Debian, Ubuntu and the RHEL family
// (r1 has the same files without Ubuntu 26.04). The ModSecurity module of EL is in el-r3 (see elWAFActions).
func moduleRelease(t osinfo.Target) string {
	return modulesBase
}

// Module kinds.
const (
	ModPageSpeed   = "pagespeed"
	ModModSecurity = "modsecurity"
)

// moduleSums are the SHA-256 of the files of the release above, by system and CPU (see moduleKey). A file that does not match is never
// installed.
var moduleSums = map[string]map[string]string{
	ModPageSpeed: {
		"jammy":          "040d7613fdfd15ab8a28d8e5c34ac45b0794bdbbcf704f9bc2f8b7061f6812a0",
		"noble":          "1da61c9b24d9b924fdfddbb869396ed7481412ec4021ef097039ff45cc47962a",
		"bookworm":       "59a02c472f593c7d1d65c7c051a925367a8a37c84033fff7935be7eba26c6d79",
		"trixie":         "5190a76ae093e99a95417d6ce4fbb3639264721274bfd173e36f54b95d099ba1",
		"jammy-arm64":    "98080bd2585fe1766ab8fa3ce9fdf44c925084399fe38f9071f9b004515dde9e",
		"noble-arm64":    "0047ca17f4121967102b80d94a179f784a63a2e4a3353e9032083dbad3099348",
		"bookworm-arm64": "9b3fe9d9bfd4c8240cd86c888724d16f71ffa94c530e858c1169127cee8757e5",
		"trixie-arm64":   "aca6de15b67c60d1faf988ecfb98fc7e471f197568243e3b03835fdc6ffd20cc",
		"resolute":       "c0ecc16300523db52c2a38525a064c6b06e2c54af4a47a38b233097db6f2b897",
		"resolute-arm64": "16629ba1d982c989f9ee501381b33c06b1996e390bde32f260dd1d6b981954d9",
		// EL 9 and 10 (AlmaLinux, Rocky Linux, CentOS Stream).
		"el9":        "57d7a1fef06296639e172f1467b6bf83d70fcaaec63506b923728458bef72279",
		"el9-arm64":  "33e8912b276f8884177270cb8d39aa431842fb545b8f2e58f33509932890ef60",
		"el10":       "4766892fa51246f2b49858b19fd3980d4ca70541dfe728c1a7460789330b3ddc",
		"el10-arm64": "d1cc67b6eb4b427eedf42c6fe4c7e61aecb988e6327571b450fbd7423450baac",
	},
	ModModSecurity: {
		"jammy":          "0fdcfc8ce3bedb5ed57d8d755df0c6c454f3f508c25f5167ca34a7d8f403134e",
		"noble":          "bbd20dfbebc0fb3293e48fa3aa2e7f889dbb418f26f1c2d5ca124b12489ea4b4",
		"bookworm":       "8bdc64c4cbb51d8d6a7cfad3abadefa0af1b9fa645d04f8fc8e9f90838140d70",
		"trixie":         "3e64fb1a0f4ea0521201f6cabeca87893ad04e8947060c44421c954ebaec6f92",
		"jammy-arm64":    "1a7c9a62e91a2259aec26dd3bd226db13ea4702cc56890b78ed35340fc2ee9c5",
		"noble-arm64":    "3c6ccd7cabf5346430341eb1c66c002abbb5e335cc7253410a6664800481184d",
		"bookworm-arm64": "fd25027fca704581455437d682ecd446fe58024e11e593e9cb30d355eec87940",
		"trixie-arm64":   "8136051db3558796bfd69cb87fd2d250a8be64ab5e5651a8fcbba8d387dfa15d",
		"resolute":       "b21ed356c1c4faabcad3d866731c795fd1ffe043fb6e867e29d7a73954f56858",
		"resolute-arm64": "fa9ae5a80498bed519110919c49b813205046b10edb7e5879f36245d97ec8f4a",
		// The ModSecurity module of EL comes from el-r3 with the rules (see elWAFActions and elSums).
	},
}

var moduleNames = map[string]struct{ file, so, snippet string }{
	ModPageSpeed:   {"ngx_pagespeed_module", "ngx_pagespeed_module.so", "50-mod-pagespeed.conf"},
	ModModSecurity: {"ngx_http_modsecurity_module", "ngx_http_modsecurity_module.so", "50-mod-http-modsecurity.conf"},
}

// moduleKey is the key of moduleSums for a system and a CPU: the codename on x86-64, and the codename with -arm64 on arm64.
func moduleKey(codename, arch string) string {
	if arch == "arm64" {
		return codename + "-arm64"
	}
	return codename
}

// ModuleFile is the name of the release file of a module for a system and a CPU ("amd64" or "arm64", as runtime.GOARCH says):
// ngx_pagespeed_module-trixie-nginx1.30.7.so on x86-64, ngx_pagespeed_module-trixie-nginx1.30.7-arm64.so on arm64.
func ModuleFile(kind, codename, arch string) string {
	suffix := ""
	if arch == "arm64" {
		suffix = "-arm64"
	}
	return fmt.Sprintf("%s-%s-nginx%s%s.so", moduleNames[kind].file, codename, NginxVersion, suffix)
}

// fetchScript downloads a file, compares it with a checksum fixed in this program and only then puts it in place. A file that is
// already there with that checksum is left alone, so running the plan again changes nothing.
func fetchScript(url, sum, dest string) string {
	return `set -e
url='` + url + `'; want='` + sum + `'; dest='` + dest + `'
if [ -f "$dest" ] && [ "$(sha256sum "$dest" | cut -d' ' -f1)" = "$want" ]; then exit 0; fi
t=$(mktemp); trap 'rm -f "$t"' EXIT
curl -fsSL --retry 3 --proto '=https' -o "$t" "$url" || { echo "nginx-module-download: $url could not be downloaded" >&2; exit 1; }
[ "$(sha256sum "$t" | cut -d' ' -f1)" = "$want" ] || { echo "nginx-module-checksum: the file from $url does not match its checksum; not installed" >&2; exit 1; }
install -d -m 0755 "$(dirname "$dest")"
install -m 0644 -o root -g root "$t" "$dest"`
}

// moduleActions puts a module for the pinned nginx in place and makes nginx load it. The caller runs nginx -t and the reload. The file
// is the one built for the CPU of this program, which is the CPU of the machine it runs on.
func moduleActions(t osinfo.Target, kind string) ([]Action, error) {
	sum, ok := moduleSums[kind][moduleKey(t.Codename, runtime.GOARCH)]
	if !ok {
		return nil, fmt.Errorf("there is no %s module for nginx %s on %s %s (%s)", kind, NginxVersion, t.ID, t.Version, runtime.GOARCH)
	}
	n := moduleNames[kind]
	dest := distro.NginxModuleDir() + "/" + n.so
	return []Action{
		Run("sh", "-c", fetchScript(moduleRelease(t)+"/"+ModuleFile(kind, t.Codename, runtime.GOARCH), sum, dest)),
		Run("install", "-d", "-m", "0755", nginxModulesEnabled),
		WriteFile(nginxModulesEnabled+"/"+n.snippet, "# Written by mpn.\nload_module "+dest+";\n", 0o644),
	}, nil
}

// nginxOrgKeyScript fetches the signing key of nginx.org and keeps it only when it holds the fingerprint above.
const nginxOrgKeyScript = `set -eu
t=$(mktemp); trap 'rm -f "$t" ` + nginxKeyring + `.tmp' EXIT
curl -fsSL --retry 3 --proto '=https' https://nginx.org/keys/nginx_signing.key -o "$t"
gpg --batch --with-colons --show-keys "$t" | awk -F: '$1=="fpr"{print $10}' | grep -qx ` + nginxSigningFPR + ` \
  || { echo "the nginx.org signing key is not the one this installer expects" >&2; exit 1; }
gpg --batch --dearmor < "$t" > ` + nginxKeyring + `.tmp
mv ` + nginxKeyring + `.tmp ` + nginxKeyring

// nginxOrgSources are the apt files that make apt take nginx 1.30.7 from nginx.org and no other version. The pin has a priority above
// 1000, so apt installs that version even over a newer one, and the hold keeps it from moving at all.
func nginxOrgSources(t osinfo.Target) (list, prefs string) {
	list = "# Written by mpn.\ndeb [signed-by=" + nginxKeyring + "] https://nginx.org/packages/" + t.ID + " " + t.Codename + " nginx\n"
	prefs = "# Written by mpn.\nPackage: *\nPin: origin nginx.org\nPin-Priority: 900\n\n" +
		"Package: nginx\nPin: version " + NginxVersion + "-1~" + t.Codename + "\nPin-Priority: 1001\n"
	return list, prefs
}

// nginxInstallActions installs nginx, from nginx.org at the fixed version or from the distribution, and holds it there.
func nginxInstallActions(t osinfo.Target) []Action {
	if !t.NginxOrg() {
		return []Action{aptInstall("nginx"), Run("apt-mark", "hold", "nginx")}
	}
	list, prefs := nginxOrgSources(t)
	return []Action{
		aptInstall("gnupg"),
		Run("sh", "-c", nginxOrgKeyScript),
		WriteFile("/etc/apt/sources.list.d/nginx.list", list, 0o644),
		WriteFile("/etc/apt/preferences.d/99-mpn-nginx", prefs, 0o644),
		aptUpdate(),
		aptInstall("nginx=" + NginxVersion + "-1~" + t.Codename),
		Run("apt-mark", "hold", "nginx"),
		// The modules of the panel load only into this exact build, so any other nginx stops the install here, not later when a module
		// fails to load.
		Run("sh", "-c", `v=$(nginx -v 2>&1); case "$v" in *nginx/`+NginxVersion+`*) ;; *) echo "nginx `+NginxVersion+` was expected, this machine has: $v" >&2; exit 1;; esac`),
		// The panel's nginx.conf includes both folders, which this package does not make.
		Run("install", "-d", "-m", "0755", nginxModulesEnabled, "/etc/nginx/sites-enabled"),
		Run("rm", "-f", "/etc/nginx/conf.d/default.conf"),
	}
}

// pageSpeedStep makes the PageSpeed module available to nginx: from the build of this project on the systems with nginx.org, and
// from the repository of We-Amp (built for the distribution's own nginx) on a system that keeps its distribution's nginx.
func pageSpeedStep(t osinfo.Target) (Step, error) {
	if !t.NginxOrg() {
		return Step{"nginx pagespeed", []Action{
			Run("sh", "-c", pagespdAptScript),
			aptUpdate(),
			aptInstall("nginx-module-pagespeed"),
			Run("nginx", "-t"),
			ReloadNginx(),
			pageSpeedCacheDir(),
		}}, nil
	}
	acts, err := moduleActions(t, ModPageSpeed)
	if err != nil {
		return Step{}, err
	}
	acts = append(acts,
		Run("nginx", "-t"),
		ReloadNginx(),
		pageSpeedCacheDir(),
	)
	return Step{"nginx pagespeed", acts}, nil
}

// pageSpeedCacheDir is the cache of PageSpeed, which nginx's workers write.
func pageSpeedCacheDir() Action {
	u := distro.WebUser()
	return Run("install", "-d", "-o", u, "-g", u, "-m", "0755", "/var/cache/ngx_pagespeed")
}

// ModsecBaseline is /etc/nginx/modsecurity.conf for a system whose nginx comes from nginx.org: the settings the OWASP rules assume,
// which the Debian package of the connector ships as a file of its own and the library does not. Without SecRequestBodyAccess the
// body of a POST is never looked at, and the other limits are the ones ModSecurity recommends.
const ModsecBaseline = `# Written by mpn: the base settings ModSecurity recommends, which the rules of the OWASP Core Rule Set assume.
SecRequestBodyAccess On
SecRequestBodyLimit 13107200
SecRequestBodyNoFilesLimit 131072
SecRequestBodyLimitAction Reject
SecResponseBodyAccess Off
SecTmpDir /tmp/
SecDataDir /tmp/
SecPcreMatchLimit 1000
SecPcreMatchLimitRecursion 1000
SecArgumentSeparator "&"
SecCookieFormat 0
SecStatusEngine Off
`

// WAFPackagesPlan installs what the web application firewall needs. nginxVersion is the version of the nginx that is installed now
// ("" when unknown): the module built for nginx.org is used only on a machine that really runs that version, so a machine installed
// before the version was fixed keeps getting the module of its distribution.
func WAFPackagesPlan(t osinfo.Target, nginxV string) (Plan, error) {
	// nginxV is the whole output of "nginx -v": a distribution's nginx at the pinned version (Ubuntu 26.04 before 1.2.3 could have
	// had one) still needs the distribution's module.
	nginxVersion, distroNginx := NginxVersionOf(nginxV), DistroNginx(nginxV)
	if t.EL() {
		acts, err := elWAFActions(t)
		if err != nil {
			return nil, err
		}
		return Plan{{Name: "ModSecurity packages", Actions: acts}}, nil
	}
	update := Run("sh", "-c", "apt-get -o DPkg::Lock::Timeout=300 update || true")
	if !t.NginxOrg() || nginxVersion != NginxVersion || distroNginx {
		return Plan{{Name: "ModSecurity packages", Actions: []Action{update, AptInstall("libnginx-mod-http-modsecurity", "modsecurity-crs")}}}, nil
	}
	mod, err := moduleActions(t, ModModSecurity)
	if err != nil {
		return nil, err
	}
	// The library is called libmodsecurity3t64 on the newer systems and libmodsecurity3 on the older ones.
	apt := "DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=300 install -y "
	lib := Run("sh", "-c", apt+"libmodsecurity3t64 modsecurity-crs || "+apt+"libmodsecurity3 modsecurity-crs")
	acts := append([]Action{update, lib}, mod...)
	acts = append(acts, WriteFile("/etc/nginx/modsecurity.conf", ModsecBaseline, 0o644))
	return Plan{{Name: "ModSecurity packages", Actions: acts}}, nil
}

// NginxVersionOf reads the version out of the output of "nginx -v" ("nginx version: nginx/1.30.7"), or "".
func NginxVersionOf(out string) string {
	_, v, ok := strings.Cut(out, "nginx/")
	if !ok {
		return ""
	}
	if i := strings.IndexAny(v, " \r\n"); i >= 0 {
		v = v[:i]
	}
	return v
}
