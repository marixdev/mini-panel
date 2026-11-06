package monitor

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Sample is one measurement.
type Sample struct {
	T      int64   `json:"t"`    // unix seconds
	CPU    float64 `json:"cpu"`  // percent of the processor that was busy
	Load   float64 `json:"load"` // load average over one minute
	MemMB  float64 `json:"mem"`  // memory in use, without what the system can take back (cache)
	SwapMB float64 `json:"swap"`
	Rx     float64 `json:"rx"` // bytes per second received
	Tx     float64 `json:"tx"` // bytes per second sent
}

// Machine is what does not change from one sample to the next.
type Machine struct {
	Cores       int     `json:"cores"`
	MemTotalMB  float64 `json:"mem_total"`
	SwapTotalMB float64 `json:"swap_total"`
}

// Disk is a filesystem.
type Disk struct {
	Mount           string  `json:"mount"`
	UsedGB, TotalGB float64 `json:"-"`
	Pct             float64 `json:"pct"`
}

// Source reads a file of /proc by name ("stat", "meminfo"...).
type Source func(name string) ([]byte, error)

// ProcSource reads the real /proc.
func ProcSource(name string) ([]byte, error) { return os.ReadFile("/proc/" + name) }

const (
	// Interval is the time between samples; Keep is how long they are kept.
	Interval = 30 * time.Second
	Keep     = 24 * time.Hour
	// saveEvery is how often the samples are written to the file.
	saveEvery = 5 * time.Minute
)

// Collector takes samples and keeps the last day of them.
type Collector struct {
	src  Source
	now  func() time.Time
	file string // where the samples are saved; "" for nowhere

	mu      sync.Mutex
	ring    []Sample
	machine Machine
	prevCPU cpuTimes
	prevNet netCounters
	prevAt  time.Time
	primed  bool
	dirty   bool
}

// New makes a collector that reads src and saves to file (which may be empty). Samples saved earlier are loaded.
func New(src Source, file string) *Collector { return newWithClock(src, file, time.Now) }

// newWithClock is New with the clock chosen, so a test can say what time it is when the saved samples are loaded.
func newWithClock(src Source, file string, now func() time.Time) *Collector {
	c := &Collector{src: src, now: now, file: file}
	c.load()
	return c
}

// Tick takes one sample. The first call only records the counters, because a rate needs two readings.
func (c *Collector) Tick() error {
	stat, err := c.src("stat")
	if err != nil {
		return err
	}
	cpu, cores, err := parseStat(stat)
	if err != nil {
		return err
	}
	memRaw, err := c.src("meminfo")
	if err != nil {
		return err
	}
	mem, err := parseMeminfo(memRaw)
	if err != nil {
		return err
	}
	load := 0.0
	if b, err := c.src("loadavg"); err == nil {
		load, _ = parseLoad(b)
	}
	var net netCounters
	if b, err := c.src("net/dev"); err == nil {
		net, _ = parseNetDev(b)
	}
	at := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.machine = Machine{Cores: cores, MemTotalMB: mem.total, SwapTotalMB: mem.swapTotal}
	prevCPU, prevNet, prevAt, primed := c.prevCPU, c.prevNet, c.prevAt, c.primed
	c.prevCPU, c.prevNet, c.prevAt, c.primed = cpu, net, at, true
	if !primed {
		return nil
	}
	s := Sample{T: at.Unix(), Load: load, MemMB: mem.used, SwapMB: mem.swapUsed}
	if dt := cpu.total - prevCPU.total; dt > 0 && cpu.total >= prevCPU.total {
		s.CPU = 100 * float64(cpu.busy-prevCPU.busy) / float64(dt)
		if s.CPU < 0 {
			s.CPU = 0
		} else if s.CPU > 100 {
			s.CPU = 100
		}
	}
	if secs := at.Sub(prevAt).Seconds(); secs > 0 {
		if net.rx >= prevNet.rx {
			s.Rx = float64(net.rx-prevNet.rx) / secs
		}
		if net.tx >= prevNet.tx {
			s.Tx = float64(net.tx-prevNet.tx) / secs
		}
	}
	c.add(s)
	return nil
}

// add appends a sample and forgets the ones older than Keep. The caller holds the lock.
func (c *Collector) add(s Sample) {
	c.ring = append(c.ring, s)
	cut := s.T - int64(Keep.Seconds())
	i := sort.Search(len(c.ring), func(i int) bool { return c.ring[i].T >= cut })
	if i > 0 {
		c.ring = append([]Sample(nil), c.ring[i:]...)
	}
	c.dirty = true
}

// Add stores a sample made elsewhere; a test uses it.
func (c *Collector) Add(s Sample) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.add(s)
}

// SetMachine sets what a test wants Machine to report.
func (c *Collector) SetMachine(m Machine) {
	c.mu.Lock()
	c.machine = m
	c.mu.Unlock()
}

// Machine returns the cores and the memory of the machine, as of the last tick.
func (c *Collector) Machine() Machine {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.machine
}

// Since returns the samples taken at or after the given time, oldest first.
func (c *Collector) Since(t time.Time) []Sample {
	c.mu.Lock()
	defer c.mu.Unlock()
	i := sort.Search(len(c.ring), func(i int) bool { return c.ring[i].T >= t.Unix() })
	return append([]Sample(nil), c.ring[i:]...)
}

// Latest returns the newest sample.
func (c *Collector) Latest() (Sample, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.ring) == 0 {
		return Sample{}, false
	}
	return c.ring[len(c.ring)-1], true
}

// Uptime is how long the machine has been running.
func (c *Collector) Uptime() time.Duration {
	b, err := c.src("uptime")
	if err != nil {
		return 0
	}
	s, _ := parseUptime(b)
	return time.Duration(s * float64(time.Second))
}

// Disks lists the filesystems with how full they are.
func (c *Collector) Disks() []Disk {
	b, err := c.src("mounts")
	if err != nil {
		return nil
	}
	var out []Disk
	for _, mp := range mountPoints(b) {
		if total, free, ok := fsUsage(mp); ok && total > 0 {
			used := total - free
			out = append(out, Disk{Mount: mp, UsedGB: float64(used) / (1 << 30), TotalGB: float64(total) / (1 << 30), Pct: 100 * float64(used) / float64(total)})
		}
	}
	return out
}

// Run takes a sample every Interval until ctx ends, and saves them now and then and at the end.
func (c *Collector) Run(ctx context.Context) {
	if err := c.Tick(); err != nil {
		log.Printf("monitor: %v", err)
	}
	tick, save := time.NewTicker(Interval), time.NewTicker(saveEvery)
	defer tick.Stop()
	defer save.Stop()
	for {
		select {
		case <-ctx.Done():
			c.save()
			return
		case <-tick.C:
			if err := c.Tick(); err != nil {
				log.Printf("monitor: %v", err)
			}
		case <-save.C:
			c.save()
		}
	}
}

type snapshot struct {
	V       int      `json:"v"`
	Samples []Sample `json:"samples"`
}

func (c *Collector) save() {
	if c.file == "" {
		return
	}
	c.mu.Lock()
	if !c.dirty {
		c.mu.Unlock()
		return
	}
	data, err := json.Marshal(snapshot{V: 1, Samples: c.ring})
	c.dirty = false
	c.mu.Unlock()
	if err != nil {
		return
	}
	tmp := c.file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err == nil {
		err = os.Rename(tmp, c.file)
		if err == nil {
			return
		}
	}
	log.Printf("monitor: the samples could not be saved to %s; the charts will start empty after a restart", filepath.Dir(c.file))
}

func (c *Collector) load() {
	if c.file == "" {
		return
	}
	b, err := os.ReadFile(c.file)
	if err != nil {
		return
	}
	var s snapshot
	if json.Unmarshal(b, &s) != nil || s.V != 1 {
		return
	}
	cut := c.now().Add(-Keep).Unix()
	for _, x := range s.Samples {
		if x.T >= cut && (len(c.ring) == 0 || x.T > c.ring[len(c.ring)-1].T) {
			c.ring = append(c.ring, x)
		}
	}
}

// Downsample reduces the samples to at most n points by averaging buckets of equal time, so a day of samples fits a chart.
// A bucket with no sample gives no point: a gap stays a gap.
func Downsample(s []Sample, from, to time.Time, n int) []Sample {
	if n < 1 || len(s) == 0 || !to.After(from) {
		return nil
	}
	if len(s) <= n {
		return s
	}
	span := to.Unix() - from.Unix()
	type acc struct {
		Sample
		n float64
	}
	buckets := make([]acc, n)
	for _, x := range s {
		i := int((x.T - from.Unix()) * int64(n) / span)
		if i < 0 || i >= n {
			continue
		}
		b := &buckets[i]
		b.T += x.T
		b.CPU += x.CPU
		b.Load += x.Load
		b.MemMB += x.MemMB
		b.SwapMB += x.SwapMB
		b.Rx += x.Rx
		b.Tx += x.Tx
		b.n++
	}
	var out []Sample
	for _, b := range buckets {
		if b.n == 0 {
			continue
		}
		out = append(out, Sample{T: b.T / int64(b.n), CPU: b.CPU / b.n, Load: b.Load / b.n, MemMB: b.MemMB / b.n, SwapMB: b.SwapMB / b.n, Rx: b.Rx / b.n, Tx: b.Tx / b.n})
	}
	return out
}
