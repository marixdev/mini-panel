//go:build !linux

package monitor

// fsUsage is only measured on Linux, where the panel runs; the development machine gets no disks.
func fsUsage(string) (total, free uint64, ok bool) { return 0, 0, false }
