package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"html/template"
	"io"
	"log"
	"net/http"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed assets/app.css assets/app.js assets/theme.js assets/terminal.js assets/fileeditor.js assets/filemanager.js assets/monitor.js assets/editor assets/vendor/xterm.js assets/vendor/xterm.css assets/vendor/addon-fit.js
var assetFS embed.FS

// page is a page drawn from the files in templates/: a shell (the frame around the page) and the page's own content.
type page struct {
	t     *template.Template
	shell string
}

// pages holds the pages already moved to the new design. Any other page is still drawn by the old inline template.
var pages = map[string]page{}

func init() {
	for name, shell := range map[string]string{"login": "auth", "sites": "base", "result": "base", "confirm": "base", "sitenew": "base", "site": "base", "fileedit": "base", "filerename": "base", "pmaopen": "base", "firewall": "base", "security": "base", "cloudlist": "base", "monitor": "base"} {
		t := template.Must(template.New(name).Funcs(template.FuncMap{"T": T, "stamp": stampText, "asset": assetURL, "siteMenu": siteMenu, "dict": dict}).
			ParseFS(templateFS, append([]string{"templates/" + shell + ".html", "templates/" + name + ".html"}, extraTemplates[name]...)...))
		pages[name] = page{t, shell}
	}
}

func (s *Server) execute(w io.Writer, v view) {
	if p, ok := pages[v.Page]; ok {
		v.Title = T("page." + v.Page)
		v.Nav = navOf[v.Page]
		// The status and the first part of the page are already sent when a template fails, so the person sees a page cut
		// short. The reason goes to the log instead of being thrown away.
		if err := p.t.ExecuteTemplate(w, shellName[p.shell], v); err != nil {
			log.Printf("page %s could not be drawn completely: %v", v.Page, err)
		}
		return
	}
	_, _ = io.WriteString(w, "unknown page")
}

// assetFiles maps the name a page uses to the embedded file and its type.
var assetFiles = map[string]struct{ path, ctype string }{
	"app.css":        {"assets/app.css", "text/css; charset=utf-8"},
	"app.js":         {"assets/app.js", "text/javascript; charset=utf-8"},
	"theme.js":       {"assets/theme.js", "text/javascript; charset=utf-8"},
	"terminal.js":    {"assets/terminal.js", "text/javascript; charset=utf-8"},
	"fileeditor.js":  {"assets/fileeditor.js", "text/javascript; charset=utf-8"},
	"filemanager.js": {"assets/filemanager.js", "text/javascript; charset=utf-8"},
	"monitor.js":     {"assets/monitor.js", "text/javascript; charset=utf-8"},
	"xterm.js":       {"assets/vendor/xterm.js", "text/javascript; charset=utf-8"},
	"xterm.css":      {"assets/vendor/xterm.css", "text/css; charset=utf-8"},
	"addon-fit.js":   {"assets/vendor/addon-fit.js", "text/javascript; charset=utf-8"},
}

// assetVersion is a short hash of each static file's content. It goes into the file's URL, so a browser that holds an
// older copy asks for the new one as soon as the file changes, instead of running a stale script for an hour.
var assetVersion = func() map[string]string {
	m := map[string]string{}
	for name, f := range assetFiles {
		b, err := assetFS.ReadFile(f.path)
		if err != nil {
			panic(err) // the files are embedded, so this cannot happen once the program builds
		}
		sum := sha256.Sum256(b)
		m[name] = hex.EncodeToString(sum[:4])
	}
	return m
}()

func assetURL(name string) string { return "/static/" + name + "?v=" + assetVersion[name] }

func (s *Server) assetRoutes(mux *http.ServeMux) {
	for name, f := range assetFiles {
		body, _ := assetFS.ReadFile(f.path)
		version := assetVersion[name]
		etag := `"` + version + `"`
		ctype := f.ctype
		mux.HandleFunc("GET /static/"+name, func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Type", ctype)
			h.Set("ETag", etag)
			if r.URL.Query().Get("v") == version {
				h.Set("Cache-Control", "public, max-age=31536000, immutable") // this URL names exactly this content
			} else {
				h.Set("Cache-Control", "no-cache") // an address without the version is always checked first
			}
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			_, _ = w.Write(body)
		})
	}
}

// extraTemplates are files parsed together with a page, for parts of it that are too long to keep in one file.
var extraTemplates = map[string][]string{"site": {"templates/files.html", "templates/filedialogs.html", "templates/fileicons.html", "templates/overview.html", "templates/siteheader.html", "templates/ssl.html", "templates/vhost.html", "templates/cron.html", "templates/cloud.html"}, "fileedit": {"templates/fileicons.html"}}
