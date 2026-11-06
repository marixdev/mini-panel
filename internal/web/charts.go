package web

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"math"
	"strings"
	"time"

	"mpn/internal/monitor"
)

// chartSeries is one line of a chart. Class is "1" or "2": the first and second colour of the palette.
type chartSeries struct {
	Name   string
	Class  string
	Values []float64
}

// chartSpec describes a line chart over time. The axis always starts at zero.
type chartSpec struct {
	ID       string
	Label    string // what the chart shows, for screen readers
	From, To time.Time
	Times    []int64
	Step     int64 // seconds between two points when none is missing: the width of a bucket of averaged samples
	Series   []chartSeries
	MinMax   float64              // the axis is at least this tall, so a small value does not fill the chart
	Cap      float64              // the axis is never taller than this when it is not 0 (a percentage is at most 100)
	Fixed    float64              // when not 0 the axis is exactly this tall (memory: the size of the machine)
	Binary   bool                 // the values are sizes written in units of 1024
	Format   func(float64) string // how a value is written, in the axis and in the tooltip
}

const (
	chartW, chartH = 600.0, 200.0
	padL, padR     = 72.0, 16.0 // wide enough for the longest axis label, "1023 KB/s"
	padT, padB     = 12.0, 28.0
)

// niceMax rounds up to a number an axis can end on: 1, 2, 2.5, 5 or 10 times a power of ten.
func niceMax(v float64) float64 {
	if v <= 0 {
		return 1
	}
	exp := math.Pow(10, math.Floor(math.Log10(v)))
	f := v / exp
	for _, n := range []float64{1, 2, 5, 10} { // the line half way up is then 0.5, 1, 2.5 or 5: never 1.25
		if f <= n+1e-9 {
			return n * exp
		}
	}
	return 10 * exp
}

func (s chartSpec) axisMax() float64 {
	if s.Fixed > 0 {
		return s.Fixed
	}
	m := s.MinMax
	for _, se := range s.Series {
		for _, v := range se.Values {
			m = math.Max(m, v)
		}
	}
	if s.Binary {
		// Sizes are written in units of 1024, so the axis rounds in those units: 2 KB/s and 4 KB/s, not 98 and 49.
		unit := 1.0
		for m >= 1024*unit {
			unit *= 1024
		}
		m = niceMax(m/unit) * unit
	} else {
		m = niceMax(m)
	}
	if s.Cap > 0 && m > s.Cap {
		m = s.Cap
	}
	return m
}

// chartJSON is what the script that answers the pointer needs; every text is already written.
type chartJSON struct {
	W float64    `json:"w"`
	H float64    `json:"h"`
	T []string   `json:"t"` // the time of each point
	X []float64  `json:"x"` // where each point is, in the units of the picture
	S []chartRow `json:"s"`
}

type chartRow struct {
	N string   `json:"n"` // name of the series
	C string   `json:"c"` // its colour
	V []string `json:"v"` // its values, written
}

// renderChart draws the chart as an inline SVG inside a figure. Nothing here comes from a person: names are fixed strings,
// the rest are numbers and times.
func renderChart(s chartSpec) template.HTML {
	max := s.axisMax()
	// Over half a day the same clock time comes round twice, so the date goes with it.
	layout := "15:04"
	if s.To.Sub(s.From) >= 12*time.Hour {
		layout = "02/01 15:04"
	}
	plotW, plotH := chartW-padL-padR, chartH-padT-padB
	span := float64(s.To.Unix() - s.From.Unix())
	if span <= 0 {
		span = 1
	}
	xOf := func(t int64) float64 { return padL + plotW*float64(t-s.From.Unix())/span }
	yOf := func(v float64) float64 { return padT + plotH*(1-math.Min(math.Max(v/max, 0), 1)) }

	var b strings.Builder
	fmt.Fprintf(&b, `<figure class="viz mt-0" tabindex="0" data-chart="%s" aria-label="%s">`, "\x00JSON\x00", html.EscapeString(s.Label))
	fmt.Fprintf(&b, `<svg viewBox="0 0 %g %g" class="viz-svg" role="img" aria-label="%s">`, chartW, chartH, html.EscapeString(s.Label))

	// grid and the numbers on the axis: the top, the middle and zero
	for _, f := range []float64{0, 0.5, 1} {
		y := yOf(max * f)
		fmt.Fprintf(&b, `<line class="viz-grid" x1="%g" x2="%g" y1="%.1f" y2="%.1f"/>`, padL, chartW-padR, y, y)
		fmt.Fprintf(&b, `<text class="viz-tick" x="%g" y="%.1f" text-anchor="end" dominant-baseline="middle">%s</text>`, padL-8, y, html.EscapeString(s.Format(max*f)))
	}
	// the time along the bottom
	for i := 0; i <= 3; i++ {
		t := s.From.Add(time.Duration(float64(s.To.Sub(s.From)) * float64(i) / 3))
		anchor := "middle"
		if i == 0 {
			anchor = "start"
		} else if i == 3 {
			anchor = "end"
		}
		fmt.Fprintf(&b, `<text class="viz-tick" x="%.1f" y="%g" text-anchor="%s">%s</text>`, xOf(t.Unix()), chartH-7, anchor, t.Local().Format(layout))
	}

	// the lines: a gap in the samples is a gap in the line, not a slope across it
	// Two samples further apart than a few intervals, or a few buckets once they were averaged, have nothing between them.
	gap := int64(monitor.Interval.Seconds()) * 4
	if g := 3 * s.Step; g > gap {
		gap = g
	}
	for _, se := range s.Series {
		var d strings.Builder
		var areas []string
		var seg strings.Builder
		var segFirst, segLast float64
		start := true
		flush := func() {
			if seg.Len() > 0 {
				areas = append(areas, fmt.Sprintf("%sL%.1f %g L%.1f %gZ", seg.String(), segLast, padT+plotH, segFirst, padT+plotH))
			}
			seg.Reset()
		}
		for i, v := range se.Values {
			x, y := xOf(s.Times[i]), yOf(v)
			if i > 0 && s.Times[i]-s.Times[i-1] > gap {
				start = true
				flush()
			}
			cmd := "L"
			if start {
				cmd, segFirst = "M", x
				start = false
			}
			fmt.Fprintf(&d, "%s%.1f %.1f", cmd, x, y)
			fmt.Fprintf(&seg, "%s%.1f %.1f", cmd, x, y)
			segLast = x
		}
		flush()
		if len(s.Series) == 1 {
			for _, a := range areas {
				fmt.Fprintf(&b, `<path class="viz-area viz-area-%s" d="%s"/>`, se.Class, a)
			}
		}
		if d.Len() > 0 {
			fmt.Fprintf(&b, `<path class="viz-line viz-line-%s" d="%s"/>`, se.Class, d.String())
		}
	}
	// the last value gets a dot with a ring in the colour of the card, so it stays clear where lines cross
	for _, se := range s.Series {
		if n := len(se.Values); n > 0 {
			fmt.Fprintf(&b, `<circle class="viz-dot viz-dot-%s" cx="%.1f" cy="%.1f" r="4"/>`, se.Class, xOf(s.Times[n-1]), yOf(se.Values[n-1]))
		}
	}
	fmt.Fprintf(&b, `<line class="viz-cross" x1="0" x2="0" y1="%g" y2="%g" hidden/>`, padT, padT+plotH)
	b.WriteString(`</svg><div class="viz-tip" hidden></div></figure>`)

	j := chartJSON{W: chartW, H: chartH}
	for _, t := range s.Times {
		j.T = append(j.T, time.Unix(t, 0).Local().Format(layout))
		j.X = append(j.X, math.Round(xOf(t)*10)/10)
	}
	for _, se := range s.Series {
		row := chartRow{N: se.Name, C: se.Class}
		for _, v := range se.Values {
			row.V = append(row.V, s.Format(v))
		}
		j.S = append(j.S, row)
	}
	raw, _ := json.Marshal(j)
	return template.HTML(strings.Replace(b.String(), "\x00JSON\x00", html.EscapeString(string(raw)), 1))
}

// value formats

func fmtPercent(v float64) string {
	if v >= 10 || v == 0 {
		return fmt.Sprintf("%.0f%%", v)
	}
	return fmt.Sprintf("%.1f%%", v)
}

func fmtMB(v float64) string {
	if v >= 1024 {
		return fmt.Sprintf("%.1f GB", v/1024)
	}
	return fmt.Sprintf("%.0f MB", v)
}

func fmtLoad(v float64) string {
	if v >= 10 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.2f", v)
}

func fmtRate(v float64) string {
	switch {
	case v >= 1<<30:
		return fmt.Sprintf("%.1f GB/s", v/(1<<30))
	case v >= 1<<20:
		return fmt.Sprintf("%.1f MB/s", v/(1<<20))
	case v >= 1<<10:
		return fmt.Sprintf("%.0f KB/s", v/(1<<10))
	}
	return fmt.Sprintf("%.0f B/s", v)
}
