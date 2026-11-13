package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"mpn/internal/backup"
	"mpn/internal/distro"
	"mpn/internal/provision"
	"mpn/internal/site"
	"mpn/internal/tune"
	"mpn/internal/unit"
	"mpn/internal/vhost"
)

// siteInfo is what the overview of a site shows. It is read by the root helper, because the size of a home directory and the
// state of a service are not for the unprivileged web process to look at.
type siteInfo struct {
	Name          string            `json:"name"`
	Kind          string            `json:"kind"`
	User          string            `json:"user"`
	Hostnames     []string          `json:"hostnames"`
	Root          string            `json:"root"`
	RootSub       string            `json:"root_sub"`
	RootOutside   bool              `json:"root_outside"` // the root is not below htdocs (made by hand); it cannot be changed here
	TLS           bool              `json:"tls"`
	CertExpiry    int64             `json:"cert_expiry,omitempty"`
	Running       bool              `json:"running"`
	RunDetail     string            `json:"run_detail,omitempty"`
	DiskBytes     int64             `json:"disk_bytes"`
	DiskCut       bool              `json:"disk_cut,omitempty"`
	LastBackup    string            `json:"last_backup,omitempty"`
	PHPVersion    string            `json:"php_version,omitempty"`
	PHPValues     map[string]string `json:"php_values,omitempty"` // every setting with the value that runs
	PHPSet        map[string]string `json:"php_set,omitempty"`    // the settings the site changed
	PHPMemory     int               `json:"php_memory_default,omitempty"`
	NodeVer       string            `json:"node_version,omitempty"`
	PyVer         string            `json:"python_version,omitempty"`
	Entry         string            `json:"entry,omitempty"`
	Port          int               `json:"port,omitempty"`
	MemMB         int               `json:"mem_mb,omitempty"`
	Upstream      string            `json:"upstream,omitempty"`
	InstalledPHP  []string          `json:"installed_php"`
	InstalledNode []string          `json:"installed_node"`
	InstalledPy   []string          `json:"installed_python"`
	Nginx         string            `json:"nginx"`
}

func sh(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return strings.TrimSpace(string(out)), err
}

func certExpiry(name string) int64 {
	b, err := os.ReadFile(provision.CertDir(name) + "/fullchain.pem")
	if err != nil {
		return 0
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return 0
	}
	c, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return 0
	}
	return c.NotAfter.Unix()
}

func lastBackup(name string) string {
	ents, err := os.ReadDir(backup.SiteDir(name))
	if err != nil {
		return ""
	}
	var stamps []string
	for _, e := range ents {
		if e.IsDir() && backup.ValidStamp(e.Name()) {
			stamps = append(stamps, e.Name())
		}
	}
	sort.Strings(stamps)
	if len(stamps) == 0 {
		return ""
	}
	return stamps[len(stamps)-1]
}

// running says whether the site is being served: the PHP pool exists and its master runs, the node unit is active, or (for
// static and proxy sites) its vhost is in place.
func running(s site.Site) (bool, string) {
	switch s.Kind {
	case site.PHP:
		svc := distro.PHPService(s.PHPVersion)
		if _, err := os.Stat(provision.PHPSocket(s.Name)); err != nil {
			return false, svc
		}
		out, _ := sh(3*time.Second, "systemctl", "is-active", svc)
		return out == "active", svc
	case site.Node, site.Python:
		out, _ := sh(3*time.Second, "systemctl", "is-active", unit.Name(s))
		return out == "active", unit.Name(s)
	}
	_, err := os.Stat("/etc/nginx/sites-enabled/" + s.Name + ".conf")
	return err == nil, "nginx"
}

func buildInfo(s site.Site, ramMB int) siteInfo {
	p := tune.New(tune.Input{RAMMB: ramMB, CPUs: 1})
	in := siteInfo{Name: s.Name, Kind: string(s.Kind), User: s.User, Hostnames: s.Hostnames, Root: s.Root, TLS: s.TLS,
		InstalledPHP: site.InstalledPHP(), InstalledNode: site.InstalledNode(unit.NodeRoot), InstalledPy: site.InstalledPython("/usr/bin")}
	if sub, ok := s.RootSub(); ok {
		in.RootSub = sub
	} else {
		in.RootOutside = true
	}
	if s.TLS {
		in.CertExpiry = certExpiry(s.Name)
	}
	in.Running, in.RunDetail = running(s)
	in.LastBackup = lastBackup(s.Name)
	if out, err := sh(8*time.Second, "du", "-sb", "--apparent-size", "/home/"+s.User); err == nil {
		if f := strings.Fields(out); len(f) > 0 {
			in.DiskBytes, _ = strconv.ParseInt(f[0], 10, 64)
		}
	} else {
		in.DiskCut = true // too large to measure quickly: the page says so rather than showing a wrong number
	}
	switch s.Kind {
	case site.PHP:
		in.PHPVersion, in.PHPSet, in.PHPMemory = s.PHPVersion, s.PHPIni, p.PHPMemoryLimitMB
		in.PHPValues = map[string]string{}
		for _, k := range site.PHPSettings("") {
			in.PHPValues[k.Name] = s.PHPValue(k.Name, p.PHPMemoryLimitMB)
		}
	case site.Node:
		in.NodeVer, in.Entry, in.Port, in.MemMB = s.NodeVersion, s.Entry, s.NodePort, s.MemMB
	case site.Python:
		in.PyVer, in.Entry, in.Port, in.MemMB = s.PyVersion, s.Entry, s.NodePort, s.MemMB
	case site.Proxy:
		in.Upstream = s.Upstream
	}
	if conf, err := vhost.Render(s, vhost.Options{TLS: s.TLS, CertDir: provision.CertDir(s.Name)}); err == nil {
		in.Nginx = conf
	}
	return in
}

func siteInfoCmd(args []string) {
	fs := flag.NewFlagSet("site info", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	ram := fs.Int("ram", 0, "host RAM in MB for the default PHP memory; detected when 0")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)
	if *ram == 0 {
		if m, _ := readMeminfo(); m > 0 {
			*ram = m
		} else {
			*ram = 1024
		}
	}
	printJSON(buildInfo(s, *ram))
	_ = fmt.Sprint
}
