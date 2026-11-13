package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"slices"

	"mpn/internal/certs"
	"mpn/internal/front"
	"mpn/internal/installer"
	"mpn/internal/mail"
	"mpn/internal/panelssl"
	"mpn/internal/store"
)

// frontInput reads the machine to find out which names are served on port 443. A name is served there only when its certificate
// covers it: a certificate for something else would give every visitor a warning.
func frontInput(st *store.Store) front.Input {
	var in front.Input
	mode, _ := st.Setting(panelssl.SettingMode, panelssl.ModeSelfSigned)
	domain, _ := st.Setting(panelssl.SettingDomain, "")
	if mode != panelssl.ModeSelfSigned && panelssl.ValidDomain(domain) == nil {
		if b, err := os.ReadFile(panelssl.Dir + "/fullchain.pem"); err == nil && certs.Covers(b, []string{domain}) {
			in.Domain = domain
		}
	}
	in.PMA = fileExists(installer.PMAVersionFile)
	in.PanelACME = fileExists(panelssl.ACMEConf)
	if on, _ := st.Setting(settingWebmail, "off"); on == "on" {
		in.Webmail = webmailPrefixReady()
	}
	return in
}

// webmailPrefixReady says the web mail program on this machine is the pinned version, which knows the /webmail/ prefix. An older one
// would write links that lead out of /webmail/, so nothing is served under that path until the program has been updated (turning the
// web mail off and on again installs the pinned one).
func webmailPrefixReady() bool {
	if !mail.WebmailKnowsPrefix {
		return false
	}
	b, err := os.ReadFile(mail.WebmailBinary)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]) == mail.WebmailSHA256
}

// syncFront makes the blocks of nginx that give the panel, the database browser and the web mail an address without a port match
// what is true now: it writes the ones that should exist, removes the ones that should not, and reloads nginx once, after nginx -t.
// It does nothing when the files are already right. Every command that changes one of the things the blocks depend on ends with it.
func syncFront(st *store.Store) error {
	want, err := front.Files(frontInput(st))
	if err != nil {
		return err
	}
	var acts []installer.Action
	for path, content := range want {
		if b, err := os.ReadFile(path); err != nil || string(b) != content {
			acts = append(acts, installer.WriteFile(path, content, 0o644))
		}
	}
	// The pages of the database browser decide where its cookies belong and where signing out leads, which differs under /pma/. A browser
	// that was installed before that existed has the old pages, so they are written again when they differ.
	phpReload := false
	if in := frontInput(st); in.PMA {
		if pp := panelPortOf(); pp > 0 {
			for path, content := range installer.PMAFiles(pp) {
				if b, err := os.ReadFile(path); err != nil || string(b) != content {
					acts = append(acts, installer.WriteFile(path, content, 0o644))
					phpReload = true
				}
			}
		}
	}
	for _, path := range front.Managed {
		if _, ok := want[path]; !ok && fileExists(path) {
			acts = append(acts, installer.Run("rm", "-f", path))
		}
	}
	if len(acts) == 0 {
		return nil
	}
	// A sorted order keeps the plan the same from one run to the next.
	slices.SortStableFunc(acts, func(a, b installer.Action) int {
		if a.Path < b.Path {
			return -1
		}
		if a.Path > b.Path {
			return 1
		}
		return 0
	})
	acts = append(acts, installer.Run("nginx", "-t"), installer.ReloadNginx())
	if phpReload {
		acts = append(acts, installer.Run("systemctl", "reload", installer.PMAPHPService()))
	}
	return runPlanQuiet(installer.Plan{{Name: "addresses of the panel without a port", Actions: acts}})
}

// printFront says where the panel, the database browser and the web mail are reached now.
func printFront(st *store.Store) {
	in := frontInput(st)
	if in.Domain == "" {
		return
	}
	fmt.Printf("panel:       https://%s\n", in.Domain)
	if in.PMA {
		fmt.Printf("phpMyAdmin:  https://%s%s\n", in.Domain, installer.PMAPrefix)
	}
	if in.Webmail {
		fmt.Printf("web mail:    https://%s/webmail/\n", in.Domain)
	}
}
