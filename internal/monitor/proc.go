// Package monitor measures the machine: processor, memory, load, network and disks. It reads /proc, keeps the last day of
// measurements in memory, and writes them to a file now and then so a restart of the panel does not empty the charts.
package monitor

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// cpuTimes are the counters of /proc/stat, in clock ticks since boot.
type cpuTimes struct{ busy, total uint64 }

// parseStat reads the first line of /proc/stat and counts the cores. Guest time is already part of user time, so it is not added.
func parseStat(b []byte) (cpuTimes, int, error) {
	var t cpuTimes
	cores := 0
	found := false
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		if f[0] != "cpu" {
			cores++
			continue
		}
		if len(f) < 8 {
			return t, 0, fmt.Errorf("/proc/stat: the cpu line is too short")
		}
		var v [8]uint64
		for i := range v {
			n, err := strconv.ParseUint(f[i+1], 10, 64)
			if err != nil {
				return t, 0, fmt.Errorf("/proc/stat: %w", err)
			}
			v[i] = n
			t.total += n
		}
		t.busy = t.total - v[3] - v[4] // not idle, not waiting for the disk
		found = true
	}
	if !found {
		return t, 0, fmt.Errorf("/proc/stat: no cpu line")
	}
	if cores == 0 {
		cores = 1
	}
	return t, cores, nil
}

// meminfo is what the charts need of /proc/meminfo, in megabytes.
type meminfo struct{ total, used, swapTotal, swapUsed float64 }

func parseMeminfo(b []byte) (meminfo, error) {
	kb := map[string]float64{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			continue
		}
		if n, err := strconv.ParseFloat(f[0], 64); err == nil {
			kb[name] = n
		}
	}
	total, ok := kb["MemTotal"]
	if !ok || total <= 0 {
		return meminfo{}, fmt.Errorf("/proc/meminfo: no MemTotal")
	}
	avail, ok := kb["MemAvailable"]
	if !ok { // kernels before 3.14
		avail = kb["MemFree"] + kb["Buffers"] + kb["Cached"]
	}
	m := meminfo{total: total / 1024, used: (total - avail) / 1024, swapTotal: kb["SwapTotal"] / 1024}
	m.swapUsed = (kb["SwapTotal"] - kb["SwapFree"]) / 1024
	if m.used < 0 {
		m.used = 0
	}
	return m, nil
}

func parseLoad(b []byte) (float64, error) {
	f := strings.Fields(string(b))
	if len(f) < 1 {
		return 0, fmt.Errorf("/proc/loadavg is empty")
	}
	return strconv.ParseFloat(f[0], 64)
}

func parseUptime(b []byte) (float64, error) {
	f := strings.Fields(string(b))
	if len(f) < 1 {
		return 0, fmt.Errorf("/proc/uptime is empty")
	}
	return strconv.ParseFloat(f[0], 64)
}

// netCounters are the bytes received and sent by every real network interface since boot.
type netCounters struct{ rx, tx uint64 }

// virtualInterface says whether an interface only carries traffic between programs on this machine.
func virtualInterface(name string) bool {
	return name == "lo" || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "virbr")
}

func parseNetDev(b []byte) (netCounters, error) {
	var c netCounters
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		f := strings.Fields(rest)
		if virtualInterface(name) || len(f) < 9 {
			continue
		}
		rx, err1 := strconv.ParseUint(f[0], 10, 64)
		tx, err2 := strconv.ParseUint(f[8], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		c.rx += rx
		c.tx += tx
	}
	return c, nil
}

// realFilesystems are the kinds of filesystem worth showing; the rest (tmpfs, overlay, squashfs of snaps...) are not storage.
var realFilesystems = map[string]bool{"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true, "zfs": true, "f2fs": true, "vfat": true}

// mountPoints reads /proc/mounts and returns one mount point for each real device, the shortest path first (so "/" wins over a
// bind mount of the same device).
func mountPoints(b []byte) []string {
	best := map[string]string{}
	var order []string
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 || !realFilesystems[f[2]] || !strings.HasPrefix(f[0], "/dev/") {
			continue
		}
		mp := strings.ReplaceAll(f[1], `\040`, " ")
		if cur, ok := best[f[0]]; !ok {
			best[f[0]] = mp
			order = append(order, f[0])
		} else if len(mp) < len(cur) {
			best[f[0]] = mp
		}
	}
	out := make([]string, 0, len(order))
	for _, dev := range order {
		out = append(out, best[dev])
	}
	return out
}
