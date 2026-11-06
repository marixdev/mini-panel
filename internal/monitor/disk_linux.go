//go:build linux

package monitor

import "syscall"

// fsUsage returns the size of a filesystem and the space free to ordinary programs, in bytes.
func fsUsage(mount string) (total, free uint64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(mount, &st); err != nil {
		return 0, 0, false
	}
	return st.Blocks * uint64(st.Bsize), st.Bavail * uint64(st.Bsize), true
}
