package web

import (
	"fmt"
	"html/template"
	"math"
	"net/http"
	"time"

	"mpn/internal/monitor"
	"mpn/internal/webauth"
)

type monRange struct {
	Key, Label string
	D          time.Duration
	Active     bool
}

var monRanges = []monRange{{Key: "1h", Label: "mon.r.1h", D: time.Hour}, {Key: "6h", Label: "mon.r.6h", D: 6 * time.Hour}, {Key: "24h", Label: "mon.r.24h", D: 24 * time.Hour}}

type monDisk struct {
	Mount           string
	UsedGB, TotalGB float64
	Pct             float64
	Level           string // ok, warn or crit
}

type monLegend struct{ Name, Class string }

type monChart struct {
	Title, Sub string
	SVG        template.HTML
	Legend     []monLegend
}

// monView is what the monitoring page draws.
type monView struct {
	Ranges    []monRange
	Empty     bool
	Now       monitor.Sample
	Machine   monitor.Machine
	Uptime    string
	CPULevel  string
	MemPct    float64
	MemLevel  string
	SwapPct   float64
	LoadLevel string
	Disks     []monDisk
	Charts    []monChart
	Services  []monitor.Service
	TableHead []string
	Table     [][]string
}

func (s *Server) monitorPages(mux *http.ServeMux) {
	mux.HandleFunc("GET /monitor", s.auth(s.monitorPage))
}

// level says how worried to be about a percentage: the colour of a meter follows it, and so does a word beside it.
func level(pct float64) string {
	switch {
	case pct >= 90:
		return "crit"
	case pct >= 75:
		return "warn"
	}
	return "ok"
}
func humanDuration(d time.
	Duration) string {
	return humanDurationText(T, d)
}

func humanDurationText(T translator, d time.Duration) string {
	days, hours, mins := int(d.Hours())/24, int(d.Hours())%24, int(d.Minutes())%60
	switch {
	case days > 0:
		return T("mon.up.dh", days, hours)
	case hours > 0:
		return T("mon.up.hm", hours, mins)
	}
	return T("mon.up.m", mins)
}

func (s *Server) monitorPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	v := &monView{}
	chosen := monRanges[0]
	for _, mr := range monRanges {
		if mr.Key == r.URL.Query().Get("range") {
			chosen = mr
		}
	}
	for _, mr := range monRanges {
		mr.Active = mr.Key == chosen.Key
		v.Ranges = append(v.Ranges, mr)
	}
	if s.Mon == nil {
		v.Empty = true
		s.show(w, 200, ss, view{Page: "monitor", Mon: v})
		return
	}

	to := time.Now()
	from := to.Add(-chosen.D)
	raw := s.Mon.Since(from)
	v.Machine = s.Mon.Machine()
	v.Uptime = humanDurationText(T, s.Mon.Uptime())
	if last, ok := s.Mon.Latest(); ok {
		v.Now = last
		v.CPULevel = level(last.CPU)
		if v.Machine.MemTotalMB > 0 {
			v.MemPct = 100 * last.MemMB / v.Machine.MemTotalMB
		}
		if v.Machine.SwapTotalMB > 0 {
			v.SwapPct = 100 * last.SwapMB / v.Machine.SwapTotalMB
		}
		v.MemLevel = level(v.MemPct)
		if v.Machine.Cores > 0 {
			v.LoadLevel = level(100 * last.Load / float64(v.Machine.Cores))
		}
	} else {
		v.Empty = true
	}
	for _, d := range s.Mon.Disks() {
		v.Disks = append(v.Disks, monDisk{Mount: d.Mount, UsedGB: d.UsedGB, TotalGB: d.TotalGB, Pct: d.Pct, Level: level(d.Pct)})
	}
	state := s.UnitState
	if state == nil {
		state = monitor.SystemdState
	}
	v.Services = monitor.Services(state, s.installedPHP())

	pts := monitor.Downsample(raw, from, to, 240)
	if len(pts) > 0 {
		times := make([]int64, len(pts))
		col := func(f func(monitor.Sample) float64) []float64 {
			out := make([]float64, len(pts))
			for i, p := range pts {
				out[i] = f(p)
			}
			return out
		}
		for i, p := range pts {
			times[i] = p.T
		}
		spec := func(id, label string, fmtf func(float64) string, series ...chartSeries) chartSpec {
			return chartSpec{ID: id, Label: label, From: from, To: to, Times: times, Step: (to.Unix() - from.Unix()) / 240, Series: series, Format: fmtf}
		}
		cpu := spec("cpu", T("mon.cpu"), fmtPercent, chartSeries{T("mon.cpu"), "1", col(func(p monitor.Sample) float64 { return p.CPU })})
		cpu.MinMax, cpu.Cap = 10, 100
		mem := spec("mem", T("mon.mem"), fmtMB, chartSeries{T("mon.mem.used"), "1", col(func(p monitor.Sample) float64 { return p.MemMB })})
		mem.Fixed = v.Machine.MemTotalMB
		if mem.Fixed <= 0 {
			mem.MinMax = 256
		}
		load := spec("load", T("mon.load"), fmtLoad, chartSeries{T("mon.load"), "1", col(func(p monitor.Sample) float64 { return p.Load })})
		load.MinMax = math.Max(1, float64(v.Machine.Cores))
		net := spec("net", T("mon.net"), fmtRate, chartSeries{T("mon.net.rx"), "1", col(func(p monitor.Sample) float64 { return p.Rx })},
			chartSeries{T("mon.net.tx"), "2", col(func(p monitor.Sample) float64 { return p.Tx })})
		net.MinMax, net.Binary = 1024, true
		v.Charts = []monChart{
			{Title: T("mon.cpu"), Sub: T("mon.cpu.sub", v.Machine.Cores), SVG: renderChart(cpu)},
			{Title: T("mon.mem"), Sub: T("mon.mem.sub", fmtMB(v.Machine.MemTotalMB)), SVG: renderChart(mem)},
			{Title: T("mon.load"), Sub: T("mon.load.sub", v.Machine.Cores), SVG: renderChart(load)},
			{Title: T("mon.net"), Sub: T("mon.net.sub"), SVG: renderChart(net), Legend: []monLegend{{T("mon.net.rx"), "1"}, {T("mon.net.tx"), "2"}}},
		}
		// The same numbers as a table, for whoever cannot or does not want to use the pointer.
		v.TableHead = []string{T("mon.t.time"), T("mon.cpu"), T("mon.mem"), T("mon.load"), T("mon.net.rx"), T("mon.net.tx")}
		for _, p := range monitor.Downsample(raw, from, to, 24) {
			v.Table = append(v.Table, []string{time.Unix(p.T, 0).Local().Format("02/01 15:04"), fmtPercent(p.CPU), fmtMB(p.MemMB), fmtLoad(p.Load), fmtRate(p.Rx), fmtRate(p.Tx)})
		}
	}
	s.show(w, 200, ss, view{Page: "monitor", Mon: v})
}

// dict gives a template a small map to pass to a sub-template: dict "Pct" 12.5 "Level" "ok".
func dict(kv ...any) (map[string]any, error) {
	if len(kv)%2 != 0 {
		return nil, fmt.Errorf("dict needs pairs")
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict keys are strings")
		}
		m[k] = kv[i+1]
	}
	return m, nil
}
