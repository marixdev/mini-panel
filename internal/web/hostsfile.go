package web

import (
	"os"
	"sync"
	"time"

	"mpn/internal/panelssl"
)

// hostsCache keeps the names read from Server.HostsFile until the file changes, so a request does not read it each time.
type hostsCache struct {
	mu    sync.Mutex
	mod   time.Time
	size  int64
	names []string
}

// maxHostsFile bounds the file; a longer one is ignored, because the panel only ever has a few names.
const maxHostsFile = 4 << 10

// fileHosts returns the names in HostsFile. A missing, unreadable or oversized file gives none, which only means that no extra
// name is accepted: IP addresses and the names the panel was started with are still accepted.
func (s *Server) fileHosts() []string {
	if s.HostsFile == "" {
		return nil
	}
	fi, err := os.Stat(s.HostsFile)
	s.hosts.mu.Lock()
	defer s.hosts.mu.Unlock()
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxHostsFile {
		s.hosts.names, s.hosts.mod, s.hosts.size = nil, time.Time{}, 0
		return nil
	}
	if !fi.ModTime().Equal(s.hosts.mod) || fi.Size() != s.hosts.size {
		b, err := os.ReadFile(s.HostsFile)
		if err != nil {
			s.hosts.names = nil
			return nil
		}
		s.hosts.names, s.hosts.mod, s.hosts.size = panelssl.ParseHosts(string(b)), fi.ModTime(), fi.Size()
	}
	return s.hosts.names
}
