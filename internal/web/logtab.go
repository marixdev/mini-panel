package web

import (
	"fmt"
	"net/http"
	"strconv"

	"mpn/internal/ctl"
	"mpn/internal/site"
	"mpn/internal/sitelog"
)

// logLineChoices are the counts the page offers. The helper accepts any count up to sitelog.MaxLines.
var logLineChoices = []int{100, 200, 500, 1000}

type logKindLink struct {
	Key    string
	Active bool
}

// logView is what the Logs tab draws.
type logView struct {
	Kinds   []logKindLink
	Kind    string
	Lines   int
	Choices []int
	Query   string
	Res     sitelog.Result
	Size    string // the size of the whole file, in words
	Error   string
}

// loadLogs reads the end of one log of the site. The kind, the count and the search come from the address; a value that is not
// valid is replaced by the default instead of being passed on.
func (s *Server) loadLogs(r *http.Request, row siteRow) *logView {
	T := translatorForRequest(r)
	hasApp := row.Kind == string(site.Node) || row.Kind == string(site.Python)
	q := r.URL.Query()
	kind := q.Get("kind")
	if kind != sitelog.Error && !(kind == sitelog.App && hasApp) {
		kind = sitelog.Access
	}
	lines := sitelog.DefaultLines
	if n, err := strconv.Atoi(q.Get("lines")); err == nil {
		if n2, err := sitelog.ValidLines(n); err == nil {
			lines = n2
		}
	}
	v := &logView{Kind: kind, Lines: lines, Choices: logLineChoices, Query: q.Get("q"), Res: sitelog.Result{Lines: []string{}}}
	if err := sitelog.ValidQuery(v.Query); err != nil {
		v.Query = ""
		v.Error = T("log.err.query")
	}
	for _, k := range []string{sitelog.Access, sitelog.Error} {
		v.Kinds = append(v.Kinds, logKindLink{k, k == kind})
	}
	if hasApp {
		v.Kinds = append(v.Kinds, logKindLink{sitelog.App, kind == sitelog.App})
	}
	if e := s.listText(T, ctl.Request{Op: "site.log", Site: row.Name, LogKind: kind, Lines: lines, Query: v.Query}, &v.Res); e != "" {
		v.Error = e
		return v
	}
	v.Size = byteSize(v.Res.Size)
	return v
}

// byteSize says a size the way a person reads it.
func byteSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
