package ctl

import (
	"fmt"
	"strconv"
	"strings"

	"mpn/internal/mail"
)

// maxAliasTargets bounds where one alias may send its mail.
const maxAliasTargets = 20

// mailArgv checks a request of the mail server with the same rules the program applies again, and attaches each value to its flag with
// "=" so one that starts with "-" cannot be read as a flag. The password of a mailbox is not here: it reaches mpn on standard input.
func (r Request) mailArgv() ([]string, error) {
	switch r.Op {
	case "mail.status":
		return []string{"mail", "status", "-json"}, nil
	case "mail.records":
		if err := mail.ValidDomain(r.MailDomain); err != nil {
			return nil, err
		}
		return []string{"mail", "records", "-domain=" + r.MailDomain, "-json"}, nil
	case "mail.enable":
		if err := mail.ValidDomain(r.MailHost); err != nil {
			return nil, fmt.Errorf("mail host: %w", err)
		}
		return []string{"mail", "enable", "-host=" + r.MailHost, "-apply"}, nil
	case "mail.disable":
		return []string{"mail", "disable", "-apply"}, nil
	case "mail.ssl":
		return []string{"mail", "ssl", "-apply"}, nil
	case "mail.webmail":
		// No -binary here: a request from the web side can only ask for the program of the release, never name a file.
		return []string{"mail", "webmail", "-on=" + strconv.FormatBool(r.Enable), "-apply"}, nil
	case "mail.domainadd", "mail.domaindel":
		if err := mail.ValidDomain(r.MailDomain); err != nil {
			return nil, err
		}
		if r.Op == "mail.domainadd" {
			return []string{"mail", "domain-add", "-domain=" + r.MailDomain, "-apply"}, nil
		}
		return []string{"mail", "domain-del", "-domain=" + r.MailDomain, "-purge=" + strconv.FormatBool(r.Purge), "-apply"}, nil
	}

	// Everything below is about one address.
	if _, _, err := mail.SplitAddress(r.MailAddress); err != nil {
		return nil, err
	}
	addr := "-address=" + r.MailAddress
	switch r.Op {
	case "mail.boxadd":
		if err := mail.ValidPassword(r.MailPassword); err != nil {
			return nil, err
		}
		if err := mail.ValidQuota(r.MailQuota); err != nil {
			return nil, err
		}
		return []string{"mail", "box-add", addr, "-quota=" + strconv.Itoa(r.MailQuota), "-apply"}, nil
	case "mail.boxpasswd":
		if err := mail.ValidPassword(r.MailPassword); err != nil {
			return nil, err
		}
		return []string{"mail", "box-passwd", addr, "-apply"}, nil
	case "mail.boxquota":
		if err := mail.ValidQuota(r.MailQuota); err != nil {
			return nil, err
		}
		return []string{"mail", "box-quota", addr, "-quota=" + strconv.Itoa(r.MailQuota), "-apply"}, nil
	case "mail.boxdel":
		return []string{"mail", "box-del", addr, "-purge=" + strconv.FormatBool(r.Purge), "-apply"}, nil
	case "mail.aliasset":
		if len(r.MailTargets) == 0 || len(r.MailTargets) > maxAliasTargets {
			return nil, fmt.Errorf("an alias needs 1 to %d target addresses", maxAliasTargets)
		}
		for _, t := range r.MailTargets {
			if _, _, err := mail.SplitAddress(t); err != nil {
				return nil, err
			}
		}
		return []string{"mail", "alias-set", addr, "-targets=" + strings.Join(r.MailTargets, ","), "-apply"}, nil
	case "mail.aliasdel":
		return []string{"mail", "alias-del", addr, "-apply"}, nil
	}
	return nil, fmt.Errorf("unknown operation %q", r.Op)
}
