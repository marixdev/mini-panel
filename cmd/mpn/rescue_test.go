package main

import (
	"bufio"
	"errors"
	"strings"
	"testing"
)

type rescueCall struct {
	stdin string
	args  string
}

// rescueRun runs the menu on the given input and returns what it printed and the commands it ran.
func rescueRun(t *testing.T, input string, passwords ...string) (string, []rescueCall) {
	t.Helper()
	var out strings.Builder
	var calls []rescueCall
	r := &rescueIO{
		in:  bufio.NewReader(strings.NewReader(input)),
		out: &out,
		run: func(stdin string, args ...string) error {
			calls = append(calls, rescueCall{stdin, strings.Join(args, " ")})
			return nil
		},
		password: func(string) (string, error) {
			if len(passwords) == 0 {
				return "", errors.New("no password left")
			}
			p := passwords[0]
			passwords = passwords[1:]
			return p, nil
		},
	}
	rescueLoop(r, "MPN test rescue menu")
	return out.String(), calls
}

func wantCalls(t *testing.T, got []rescueCall, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ran %d commands %v, want %v", len(got), got, want)
	}
	for i := range want {
		if got[i].args != want[i] {
			t.Errorf("command %d: %q, want %q", i, got[i].args, want[i])
		}
	}
}

func TestRescueMenuListsEveryEntryAndExits(t *testing.T) {
	out, calls := rescueRun(t, "0\n")
	for _, s := range []string{"Reset admin password", "Turn off admin 2FA", "Disable a delegated user", "New first-time setup link",
		"Turn off panel SSL", "Show ports", "Cancel an unfinished port change", "Unban an IP address", "Firewall status",
		"Status of panel services", "Restart the panel", "0) Exit"} {
		if !strings.Contains(out, s) {
			t.Errorf("menu lacks %q", s)
		}
	}
	wantCalls(t, calls)
}

func TestRescueMenuEndsWhenTheInputEnds(t *testing.T) {
	_, calls := rescueRun(t, "")
	wantCalls(t, calls)
}

func TestRescueMenuRefusesAChoiceNotInTheList(t *testing.T) {
	out, calls := rescueRun(t, "99\nabc\n0\n")
	if strings.Count(out, "Choose a number from the list.") != 2 {
		t.Errorf("bad choices not refused:\n%s", out)
	}
	wantCalls(t, calls)
}

func TestRescueChangesNothingUnlessConfirmed(t *testing.T) {
	// 2FA off, setup link, SSL off, firewall off and restart, each answered with Enter (no) or "n".
	_, calls := rescueRun(t, "2\n\n\n4\nn\n\n5\n\n\n9\n\n\n11\nno\n\n0\n")
	wantCalls(t, calls, "firewall status") // the firewall entry shows the status before it asks
}

func TestRescueTurnsOff2FAWhenConfirmed(t *testing.T) {
	out, calls := rescueRun(t, "2\ny\n\n0\n")
	wantCalls(t, calls, "web 2fa-off")
	if !strings.Contains(out, "$ mpn web 2fa-off") {
		t.Errorf("the command is not shown before it runs:\n%s", out)
	}
}

func TestRescueResetPasswordPassesTheTypedPasswordOnStdin(t *testing.T) {
	_, calls := rescueRun(t, "1\n\n0\n", "correct horse battery", "correct horse battery")
	wantCalls(t, calls, "web passwd -stdin")
	if calls[0].stdin != "correct horse battery\n" {
		t.Errorf("stdin %q", calls[0].stdin)
	}
}

func TestRescueResetPasswordRefusesAMismatchOrAShortPassword(t *testing.T) {
	out, calls := rescueRun(t, "1\n\n0\n", "correct horse battery", "something else entirely")
	wantCalls(t, calls)
	if !strings.Contains(out, "not the same") {
		t.Errorf("mismatch not reported:\n%s", out)
	}
	out, calls = rescueRun(t, "1\n\n0\n", "short")
	wantCalls(t, calls)
	if !strings.Contains(out, "must have 12 to 256 characters") {
		t.Errorf("short password not refused:\n%s", out)
	}
}

func TestRescueResetPasswordGeneratesOneWhenLeftEmpty(t *testing.T) {
	_, calls := rescueRun(t, "1\ny\n\n0\n", "")
	wantCalls(t, calls, "web passwd")
}

func TestRescueDisablesTheNamedUser(t *testing.T) {
	_, calls := rescueRun(t, "3\nann\ny\n\n0\n")
	wantCalls(t, calls, "web user-off ann")
	_, calls = rescueRun(t, "3\n\n\n0\n") // empty name goes back
	wantCalls(t, calls)
}

func TestRescuePortCancelOnlyTakesSSHOrPanel(t *testing.T) {
	_, calls := rescueRun(t, "7\nssh\ny\n\n0\n")
	wantCalls(t, calls, "port status", "port cancel -service ssh -apply")
	out, calls := rescueRun(t, "7\nwebmail; id\n\n0\n")
	wantCalls(t, calls, "port status")
	if !strings.Contains(out, "type ssh or panel") {
		t.Errorf("bad service not refused:\n%s", out)
	}
}

func TestRescueUnbanChecksTheAddressAndTriesEveryJail(t *testing.T) {
	_, calls := rescueRun(t, "8\n203.0.113.9\ny\n\n0\n")
	wantCalls(t, calls,
		"security unban -jail sshd -ip 203.0.113.9 -apply",
		"security unban -jail mpn-panel -ip 203.0.113.9 -apply",
		"security unban -jail mpn-modsec -ip 203.0.113.9 -apply")
	out, calls := rescueRun(t, "8\n1.2.3.4; rm -rf /\n\n0\n")
	wantCalls(t, calls)
	if !strings.Contains(out, "is not an IP address") {
		t.Errorf("bad address not refused:\n%s", out)
	}
}

func TestRescueShowsPortsAndServicesWithoutAsking(t *testing.T) {
	_, calls := rescueRun(t, "6\n\n10\n\n0\n")
	wantCalls(t, calls, "port status", "services status")
}
