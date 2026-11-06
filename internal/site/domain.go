package site

import (
	"slices"
	"strings"
)

// A site has one domain. Its first host name is the address the site is known by. With the www option the same name with or
// without "www." is also served, only to send visitors on to that first name, so the certificate covers both.
const (
	WWWOff  = ""     // only the domain itself
	WWWBare = "bare" // example.com is the address; www.example.com redirects to it
	WWWWith = "www"  // www.example.com is the address; example.com redirects to it
)

// Where a site's SSL certificate comes from. A site made new has none; a person chooses on the SSL tab.
const (
	SSLNone        = ""            // no certificate: the site is served over HTTP
	SSLLetsEncrypt = "letsencrypt" // issued by Let's Encrypt and renewed by mpn
	SSLCustom      = "custom"      // brought by the person (bought, or an origin certificate); they replace it before it ends
)

// ValidSSLMode reports whether m is one of the three.
func ValidSSLMode(m string) bool { return m == SSLNone || m == SSLLetsEncrypt || m == SSLCustom }

// ValidWWW reports whether w is one of the three choices.
func ValidWWW(w string) bool { return w == WWWOff || w == WWWBare || w == WWWWith }

// BareDomain is a host name without a leading "www.".
func BareDomain(h string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(h)), "www.")
}

// NamesFor returns the host names of a site for a domain and a www choice: the address first, then the name that redirects to it.
func NamesFor(domain, www string) []string {
	bare := BareDomain(domain)
	switch www {
	case WWWBare:
		return []string{bare, "www." + bare}
	case WWWWith:
		return []string{"www." + bare, bare}
	}
	return []string{strings.ToLower(strings.TrimSpace(domain))}
}

// ValidDomain reports whether d can be a site's domain: a host name, and for the www choices one that still is a host name
// once "www." is put in front or taken off.
func ValidDomain(d string) bool {
	return ValidHostname(d) && ValidHostname(BareDomain(d))
}

// Primary is the address the site is known by.
func (s Site) Primary() string {
	if len(s.Hostnames) == 0 {
		return ""
	}
	return s.Hostnames[0]
}

// Aliases are the names that only redirect to the primary one. They exist only with the www choice; without it every host
// name serves the site, as it did before a site had one domain.
func (s Site) Aliases() []string {
	if s.WWW == WWWOff || len(s.Hostnames) < 2 {
		return nil
	}
	return s.Hostnames[1:]
}

// validDomainFields checks the www choice against the host names, for Validate.
func (s Site) validDomainFields() []string {
	var problems []string
	if !ValidWWW(s.WWW) {
		return []string{"the www choice " + s.WWW + " is not one of: none, bare, www"}
	}
	if s.WWW != WWWOff && len(s.Hostnames) > 0 && !slices.Equal(s.Hostnames, NamesFor(s.Hostnames[0], s.WWW)) {
		problems = append(problems, "with the www choice a site has exactly its domain and the same name with or without www, the address first")
	}
	return problems
}
