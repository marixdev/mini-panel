package installer

import (
	"fmt"
	"strings"

	"mpn/internal/osinfo"
)

// One nginx version on every system. A dynamic module only loads into the exact nginx it was built for, and each distribution ships a
// different nginx (1.18 to 1.28), so the panel uses nginx 1.28.3 from nginx.org and loads two modules built for that version by
// github.com/marixdev/mpn-nginx-modules. Ubuntu 26.04 already ships 1.28.3, so it keeps the nginx and the packaged modules of the
// distribution (see osinfo.Target.NginxOrg).
const (
	NginxVersion = "1.28.3"
	// nginxSigningFPR is a fingerprint in the signing key nginx.org publishes; the key is refused when it is not there.
	nginxSigningFPR = "573BFD6B3D8FBC641079A6ABABF5BD827BD9BF62"

	modulesRelease = "nginx-1.28.3-r1"
	modulesBase    = "https://github.com/marixdev/mpn-nginx-modules/releases/download/" + modulesRelease

	nginxModuleDir      = "/usr/lib/nginx/modules"
	nginxModulesEnabled = "/etc/nginx/modules-enabled"
	nginxKeyring        = "/usr/share/keyrings/nginx-archive-keyring.gpg"
)

// Module kinds.
const (
	ModPageSpeed   = "pagespeed"
	ModModSecurity = "modsecurity"
)

// moduleSums are the SHA-256 of the files of the release above. A file that does not match is never installed.
var moduleSums = map[string]map[string]string{
	ModPageSpeed: {
		"jammy":    "76dd04c59dbe853d199a2c0d8e977811a0ff99fa832e7f900373d65b00e1c431",
		"noble":    "1a54e8c592294a835b1adda9a972c216ba62ad4996df08f996174debf4528da5",
		"bookworm": "7b7732689562a0264e7c59de23c378f2bd30d1880c45238e1122710718e21721",
		"trixie":   "2a16561a51188766326749aa9e87d119e33a5e8446375e1a26e660ef65587d5b",
	},
	ModModSecurity: {
		"jammy":    "97c36e71bf8d896461ee37c74b7fc9496c758dfa247c0d6f640c22a59a94256d",
		"noble":    "3cd94b8e89db382d0dd4de7b6cfdf08a98b1b3137115d51cb5cf473f6511d2f6",
		"bookworm": "6ccb5f5b45bff0cdfe4e96e7940fecdc3e629b112891adea123dda9547fcf391",
		"trixie":   "e33ee1a9b4c2d5c4d18bd37f7ee38d530d5a573715827430840469eac5883e25",
	},
}

var moduleNames = map[string]struct{ file, so, snippet string }{
	ModPageSpeed:   {"ngx_pagespeed_module", "ngx_pagespeed_module.so", "50-mod-pagespeed.conf"},
	ModModSecurity: {"ngx_http_modsecurity_module", "ngx_http_modsecurity_module.so", "50-mod-http-modsecurity.conf"},
}

// ModuleFile is the name of the release file of a module for a system.
func ModuleFile(kind, codename string) string {
	return fmt.Sprintf("%s-%s-nginx%s.so", moduleNames[kind].file, codename, NginxVersion)
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

// moduleActions puts a module for the pinned nginx in place and makes nginx load it. The caller runs nginx -t and the reload.
func moduleActions(t osinfo.Target, kind string) ([]Action, error) {
	sum, ok := moduleSums[kind][t.Codename]
	if !ok {
		return nil, fmt.Errorf("there is no %s module for nginx %s on %s %s", kind, NginxVersion, t.ID, t.Version)
	}
	n := moduleNames[kind]
	dest := nginxModuleDir + "/" + n.so
	return []Action{
		Run("sh", "-c", fetchScript(modulesBase+"/"+ModuleFile(kind, t.Codename), sum, dest)),
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

// nginxOrgSources are the apt files that make apt take nginx 1.28.3 from nginx.org and no other version. The pin has a priority above
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
		// The panel's nginx.conf includes both folders, which this package does not make.
		Run("install", "-d", "-m", "0755", nginxModulesEnabled, "/etc/nginx/sites-enabled"),
		Run("rm", "-f", "/etc/nginx/conf.d/default.conf"),
	}
}

// pageSpeedStep makes the PageSpeed module available to nginx: from the build of this project on the systems with nginx.org, and
// from the repository of We-Amp (built for the distribution's own nginx) on Ubuntu 26.04.
func pageSpeedStep(t osinfo.Target) (Step, error) {
	if !t.NginxOrg() {
		return Step{"nginx pagespeed", []Action{
			Run("sh", "-c", pagespdAptScript),
			aptUpdate(),
			aptInstall("nginx-module-pagespeed"),
			Run("nginx", "-t"),
			ReloadNginx(),
			Run("install", "-d", "-o", "www-data", "-g", "www-data", "-m", "0755", "/var/cache/ngx_pagespeed"),
		}}, nil
	}
	acts, err := moduleActions(t, ModPageSpeed)
	if err != nil {
		return Step{}, err
	}
	acts = append(acts,
		Run("nginx", "-t"),
		ReloadNginx(),
		Run("install", "-d", "-o", "www-data", "-g", "www-data", "-m", "0755", "/var/cache/ngx_pagespeed"),
	)
	return Step{"nginx pagespeed", acts}, nil
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
func WAFPackagesPlan(t osinfo.Target, nginxVersion string) (Plan, error) {
	update := Run("sh", "-c", "apt-get -o DPkg::Lock::Timeout=300 update || true")
	if !t.NginxOrg() || nginxVersion != NginxVersion {
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

// NginxVersionOf reads the version out of the output of "nginx -v" ("nginx version: nginx/1.28.3"), or "".
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
