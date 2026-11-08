//go:build linux

package main

import (
	"path/filepath"
	"syscall"
)

// freeBytes is the space an ordinary program can still write on the disk that holds path, or -1 when it cannot be found. A folder that
// does not exist yet is measured through the nearest one that does.
func freeBytes(path string) int64 {
	for p := path; ; p = filepath.Dir(p) {
		var s syscall.Statfs_t
		if err := syscall.Statfs(p, &s); err == nil {
			return int64(s.Bavail) * int64(s.Bsize)
		}
		if p == filepath.Dir(p) {
			return -1
		}
	}
}
