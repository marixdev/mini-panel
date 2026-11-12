package web

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// Version is the program's version, shown in the footer. The main package sets it at start.
var Version = "dev"

// footerView is what the footer of every page shows: the machine on the left, the copyright on the right.
type footerView struct {
	OS, CPU, RAM string
	Year         int
	Version      string
}

func osName(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "PRETTY_NAME="); ok {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

func (s *Server) footer() *footerView {
	f := &footerView{OS: s.HostOS, Year: time.Now().Year(), Version: Version}
	if f.OS == "" {
		f.OS = osName("/etc/os-release")
	}
	if s.Mon != nil {
		m := s.Mon.Machine()
		if m.Cores > 0 {
			f.CPU = fmt.Sprintf("%d", m.Cores)
		}
		if m.MemTotalMB > 0 {
			f.RAM = fmtMB(m.MemTotalMB)
		}
	}
	return f
}
