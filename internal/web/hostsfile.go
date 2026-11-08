package web

import (
	"io"
	"os"

	"mpn/internal/panelssl"
)

// maxHostsFile bounds the file; a longer one is ignored, because the panel only ever has a few names.
const maxHostsFile = 4 << 10

// fileHosts returns the names in HostsFile. It reads the file each time: it is a few dozen bytes, and a copy kept between requests
// could miss a change that keeps the size and falls in the same tick of the clock. A missing, unreadable or oversized file gives
// none, which only means that no extra name is accepted: IP addresses and the names the panel was started with still are.
func (s *Server) fileHosts() []string {
	if s.HostsFile == "" {
		return nil
	}
	if fi, err := os.Stat(s.HostsFile); err != nil || !fi.Mode().IsRegular() {
		return nil
	}
	f, err := os.Open(s.HostsFile)
	if err != nil {
		return nil
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxHostsFile+1))
	if err != nil || len(b) > maxHostsFile {
		return nil
	}
	return panelssl.ParseHosts(string(b))
}
