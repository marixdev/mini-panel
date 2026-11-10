// Package front decides which nginx server blocks give the panel, the database browser and the web mail an address without a port.
// It holds no state and touches no file: the caller says what is true on the machine and gets back the files that should exist.
package front

import (
	"mpn/internal/installer"
	"mpn/internal/panelssl"
)

// Input is what decides the blocks.
type Input struct {
	// Domain is the name of the panel, or "" when it has none or its certificate does not cover it: only a certificate that covers the
	// name can be served on port 443 without a browser warning.
	Domain string
	// PMA says the database browser is installed.
	PMA bool
	// Webmail says the web mail is on and its program is a version that knows the /webmail/ prefix; the panel's name then serves it
	// under /webmail/ next to /pma/.
	Webmail bool
	// PanelACME says the block that answers Let's Encrypt on port 80 for the panel's name exists. While the name has a block of its own
	// on port 443, that block sends visitors of port 80 there; otherwise it is left as it is (it sends them to the panel's port).
	PanelACME bool
}

// Managed is every file Files can return, except the block of port 80, which belongs to the certificate. A caller removes the ones
// that Files does not return.
var Managed = []string{installer.PanelDomainFile}

// Files returns the content of each file that should exist, by path.
func Files(in Input) (map[string]string, error) {
	out := map[string]string{}
	if in.Domain == "" {
		return out, nil
	}
	if err := panelssl.ValidDomain(in.Domain); err != nil {
		return nil, err
	}
	out[installer.PanelDomainFile] = installer.PanelDomainNginx(in.Domain, in.PMA, in.Webmail)
	if in.PanelACME {
		conf, err := panelssl.ACMEConfig(in.Domain, 0) // the name has its own block on 443, so port 80 sends visitors there
		if err != nil {
			return nil, err
		}
		out[panelssl.ACMEConf] = conf
	}
	return out, nil
}
