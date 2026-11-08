package cloudsync

import (
	"os"
	"strings"
	"time"
)

// FetchState is how the last download of a backup from the drive went; the panel shows it.
type FetchState struct {
	Stamp   string `json:"stamp"`
	At      int64  `json:"at"`
	Running bool   `json:"running"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

func FetchKey(siteName string) string { return "gdrive_fetch_" + siteName }

// startGrace is how long a download may be marked running before its process has taken the lock.
const startGrace = 30 * time.Second

// LockHeld reports whether a sync or a download is running now.
func LockHeld(path string, alive func(pid int) bool) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var pid int
	for _, f := range strings.Fields(string(b)) {
		for _, c := range f {
			if c < '0' || c > '9' {
				return false
			}
			pid = pid*10 + int(c-'0')
		}
		break
	}
	return pid > 0 && alive(pid)
}

// Effective is the state to show. A download marked as running whose process is gone (the server was restarted, the process was killed)
// would otherwise show "running" for ever, so it is shown as interrupted.
func (s FetchState) Effective(now time.Time, lockHeld bool) FetchState {
	if s.Running && !lockHeld && now.Sub(time.Unix(s.At, 0)) > startGrace {
		s.Running, s.OK, s.Message = false, false, "cloud-interrupted: the download stopped before it finished"
	}
	return s
}
