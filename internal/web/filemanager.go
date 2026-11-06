package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/pkg/sftp"

	"mpn/internal/fm"
	"mpn/internal/webauth"
)

// fileRow is one line of a folder listing, with everything the page shows already worked out.
type fileRow struct {
	Name, Path, Size, Mode, Time  string
	Kind                          string // which icon to draw: dir, zip, image, code, text, config or file
	Bytes, Unix                   int64
	IsDir, IsZip, IsLink, CanEdit bool
}

// fileKind picks the icon of a file from its name.
func fileKind(name string, isDir bool) string {
	if isDir {
		return "dir"
	}
	n := strings.ToLower(name)
	ext := ""
	if i := strings.LastIndexByte(n, '.'); i >= 0 {
		ext = n[i+1:]
	}
	switch {
	case n == ".htaccess" || n == ".env" || strings.HasPrefix(n, ".env.") || n == "nginx.conf" || n == "wp-config.php":
		return "config"
	}
	switch ext {
	case "zip", "gz", "tgz", "tar", "bz2", "xz", "7z", "rar":
		return "zip"
	case "png", "jpg", "jpeg", "gif", "webp", "svg", "ico", "bmp", "avif":
		return "image"
	case "php", "phtml", "js", "mjs", "ts", "jsx", "tsx", "css", "scss", "less", "html", "htm", "json", "xml", "yml", "yaml", "sql", "sh", "py", "go", "rb", "twig":
		return "code"
	case "ini", "conf", "cnf", "env", "toml":
		return "config"
	case "txt", "md", "log", "csv", "markdown":
		return "text"
	}
	return "file"
}

type crumb struct{ Name, Path string }

const maxEditBytes = 2 << 20

func (s *Server) maxUpload() int64 {
	if s.MaxUpload > 0 {
		return s.MaxUpload
	}
	return 1 << 30
}

// openFM logs in as the site's account, opens SFTP and says where its home directory is.
func (s *Server) openFM(user string) (fm.Client, func(), error) {
	if s.Dialer == nil {
		return fm.Client{}, nil, errors.New(T("err.nohelper"))
	}
	cli, err := s.Dialer.Dial(user)
	if err != nil {
		return fm.Client{}, nil, err
	}
	sc, err := sftp.NewClient(cli)
	if err != nil {
		cli.Close()
		return fm.Client{}, nil, err
	}
	closeAll := func() { sc.Close(); cli.Close() }
	var root string
	if s.FMRoot != nil {
		root, err = s.FMRoot(sc)
	} else {
		root, err = sc.Getwd()
	}
	if err != nil || !strings.HasPrefix(path.Clean(root), "/") && !strings.Contains(root, ":") {
		closeAll()
		if err == nil {
			err = errors.New("unexpected home directory")
		}
		return fm.Client{}, nil, err
	}
	return fm.Client{C: sc, Root: path.Clean(root)}, closeAll, nil
}

// fmMessage turns an error of the file manager into a sentence for the page.
func fmMessage(err error) string {
	for e, key := range map[error]string{
		fm.ErrBadPath: "fm.err.path", fm.ErrBadName: "fm.err.name", fm.ErrExists: "fm.err.exists", fm.ErrNotFound: "fm.err.notfound",
		fm.ErrDenied: "fm.err.denied", fm.ErrBinary: "fm.err.binary", fm.ErrChanged: "fm.err.changed", fm.ErrTooBig: "fm.err.toobig",
		fm.ErrIsDir: "fm.err.isdir", fm.ErrRoot: "fm.err.root", fm.ErrTooMany: "fm.err.toomany", fm.ErrBadMode: "fm.err.mode",
	} {
		if errors.Is(err, e) {
			return T(key)
		}
	}
	return T("fm.err.other", err.Error())
}

// filesCtx is what every handler of the file manager needs: the site, an open SFTP login and the folder asked for.
type filesCtx struct {
	row   siteRow
	f     fm.Client
	dir   string // the folder, relative to the home directory, tidy
	close func()
}

// withFiles finds the site, logs in as its account and checks the folder in the address. On failure it has already
// answered and returns false.
func (s *Server) withFiles(w http.ResponseWriter, r *http.Request, ss webauth.Session, dirParam string) (*filesCtx, bool) {
	row, status, msg := s.findSite(r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return nil, false
	}
	f, closeFn, err := s.openFM(row.User)
	if err != nil {
		s.termLog("files login failed", row, err)
		s.show(w, http.StatusBadGateway, ss, view{Page: "site", Site: row, Tab: "files", Error: T("fm.err.login")})
		return nil, false
	}
	raw := r.URL.Query().Get(dirParam)
	// A multipart body is an upload and is read as a stream by its own handler; asking for its form values here would
	// read and store all of it first.
	if r.Method == http.MethodPost && !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		raw = r.PostFormValue(dirParam)
	}
	if _, err := f.Abs(raw); err != nil {
		closeFn()
		http.Error(w, fmMessage(err), http.StatusBadRequest)
		return nil, false
	}
	return &filesCtx{row: row, f: f, dir: fm.Clean(raw), close: closeFn}, true
}

func filesURL(site, dir string) string {
	u := "/sites/" + site + "/files"
	if dir != "" {
		u += "?path=" + url.QueryEscape(dir)
	}
	return u
}

// back sends the browser to the folder with a message that is shown once.
func (s *Server) back(w http.ResponseWriter, r *http.Request, site, dir string, res *result) {
	if c, err := r.Cookie(s.cookieName()); err == nil {
		s.putFlash(c.Value, res)
	}
	http.Redirect(w, r, filesURL(site, dir), http.StatusSeeOther)
}

func humanSize(n int64) string {
	const k = 1024.0
	f := float64(n)
	switch {
	case f < k:
		return fmt.Sprintf("%d B", n)
	case f < k*k:
		return fmt.Sprintf("%.1f KB", f/k)
	case f < k*k*k:
		return fmt.Sprintf("%.1f MB", f/k/k)
	}
	return fmt.Sprintf("%.2f GB", f/k/k/k)
}

func (s *Server) filesPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	fc, ok := s.withFiles(w, r, ss, "path")
	if !ok {
		return
	}
	defer fc.close()
	v := view{Page: "site", Site: fc.row, Tab: "files", Dir: fc.dir}
	if c, err := r.Cookie(s.cookieName()); err == nil {
		v.Notice = s.takeFlash(c.Value)
	}
	// The page's own script uploads files and needs to call the panel; it may reach no other address.
	w.Header().Set("Content-Security-Policy", basePolicy+"; connect-src 'self'")
	entries, cut, err := fc.f.List(fc.dir)
	if err != nil {
		v.Error = fmMessage(err)
	}
	v.Cut = cut
	for _, e := range entries {
		rel := path.Join(fc.dir, e.Name)
		row := fileRow{Name: e.Name, Path: rel, IsDir: e.IsDir, IsLink: e.IsLink, Time: e.ModTime.Local().Format("02/01/2006 15:04"),
			IsZip: !e.IsDir && strings.HasSuffix(strings.ToLower(e.Name), ".zip"), CanEdit: !e.IsDir && e.Size <= maxEditBytes && !binaryKind(fileKind(e.Name, false)),
			Kind: fileKind(e.Name, e.IsDir), Bytes: e.Size, Unix: e.ModTime.Unix()}
		if !e.IsDir {
			row.Size = humanSize(e.Size)
		}
		if e.Mode&os.ModeSymlink == 0 {
			row.Mode = fmt.Sprintf("%03o", uint32(e.Mode.Perm()))
		}
		v.Files = append(v.Files, row)
	}
	v.Crumbs = []crumb{{Name: "~", Path: ""}}
	acc := ""
	for _, el := range strings.Split(fc.dir, "/") {
		if el != "" {
			acc = path.Join(acc, el)
			v.Crumbs = append(v.Crumbs, crumb{Name: el, Path: acc})
		}
	}
	v.Parent = path.Dir(fc.dir)
	if v.Parent == "." {
		v.Parent = ""
	}
	s.show(w, 200, ss, v)
}

func (s *Server) download(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	fc, ok := s.withFiles(w, r, ss, "path")
	if !ok {
		return
	}
	defer fc.close()
	file, fi, err := fc.f.Open(fc.dir)
	if err != nil {
		http.Error(w, fmMessage(err), http.StatusNotFound)
		return
	}
	defer file.Close()
	// A file of the website is never shown inside the panel's own page: it could carry script that would run with the
	// panel's rights. It is always sent as a download of unknown type.
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(fi.Name()))
	h.Set("Cache-Control", "no-store")
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(2 * time.Hour))
	http.ServeContent(w, r, fi.Name(), fi.ModTime(), file)
}

// authStream is auth for a request whose body must not be read before the handler does: the form token travels in the
// address, because looking for it in the body would make the server read (and store) the whole upload first.
func (s *Server) authStream(h func(http.ResponseWriter, *http.Request, webauth.Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(s.cookieName())
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ss, ok := s.Sessions.Get(c.Value)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !s.Sessions.CheckCSRF(c.Value, r.URL.Query().Get("csrf")) {
			http.Error(w, "invalid form token", http.StatusForbidden)
			return
		}
		h(w, r, ss)
	}
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(2 * time.Hour)) // the server's short read limit is for ordinary forms
	fc, ok := s.withFiles(w, r, ss, "dir")
	if !ok {
		return
	}
	defer fc.close()
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "bad upload", http.StatusBadRequest)
		return
	}
	overwrite, done := false, 0
	var lines []string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			lines = append(lines, T("fm.err.other", err.Error()))
			break
		}
		switch part.FormName() {
		case "overwrite": // sent before the files, because the form lists it first
			overwrite = true
		case "files":
			name := path.Base(strings.ReplaceAll(part.FileName(), "\\", "/"))
			if part.FileName() == "" {
				continue
			}
			if _, err := fc.f.Upload(fc.dir, name, part, overwrite, s.maxUpload()); err != nil {
				lines = append(lines, name+": "+fmMessage(err))
			} else {
				done++
			}
		}
	}
	// The page's script sends an upload with a progress bar and wants the outcome as data. It shows the message itself,
	// so nothing is left over to be shown a second time.
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": len(lines) == 0, "done": done, "title": T("fm.uploaded", done), "errors": lines})
		return
	}
	res := &result{OK: len(lines) == 0, Output: strings.Join(lines, "\n")}
	res.Title = T("fm.uploaded", done)
	s.back(w, r, fc.row.Name, fc.dir, res)
}

func (s *Server) simpleAction(title string, do func(fc *filesCtx, r *http.Request) error) func(http.ResponseWriter, *http.Request, webauth.Session) {
	return func(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
		fc, ok := s.withFiles(w, r, ss, "dir")
		if !ok {
			return
		}
		defer fc.close()
		res := &result{Title: T(title), OK: true}
		if err := do(fc, r); err != nil {
			// The title says what was asked for ("Đã đổi tên."), so it must not stay over a message that says it failed.
			res.OK, res.Title, res.Error = false, T("fm.fail"), fmMessage(err)
		}
		s.back(w, r, fc.row.Name, fc.dir, res)
	}
}

func (s *Server) fileRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /sites/{name}/files", s.auth(s.filesPage))
	mux.HandleFunc("GET /sites/{name}/files/download", s.auth(s.download))
	mux.HandleFunc("POST /sites/{name}/files/upload", s.authStream(s.upload))
	mux.HandleFunc("POST /sites/{name}/files/mkdir", s.auth(s.simpleAction("fm.done.mkdir", func(fc *filesCtx, r *http.Request) error {
		return fc.f.Mkdir(fc.dir, strings.TrimSpace(r.PostFormValue("name")))
	})))
	mux.HandleFunc("POST /sites/{name}/files/newfile", s.auth(s.simpleAction("fm.done.newfile", func(fc *filesCtx, r *http.Request) error {
		return fc.f.Create(fc.dir, strings.TrimSpace(r.PostFormValue("name")))
	})))
	mux.HandleFunc("GET /sites/{name}/files/rename", s.auth(s.renamePage))
	mux.HandleFunc("POST /sites/{name}/files/rename", s.auth(s.simpleAction("fm.done.rename", func(fc *filesCtx, r *http.Request) error {
		// The dialog of the list sends "name" like the other dialogs; the page for browsers without script sends "newname".
		newName := r.PostFormValue("newname")
		if newName == "" {
			newName = r.PostFormValue("name")
		}
		return fc.f.Rename(fm.Clean(r.PostFormValue("path")), strings.TrimSpace(newName))
	})))
	// The list of chosen names is sent by POST, so the form token never appears in an address.
	mux.HandleFunc("POST /sites/{name}/files/confirm-delete", s.auth(s.deleteConfirm))
	mux.HandleFunc("POST /sites/{name}/files/delete", s.auth(s.simpleAction("fm.done.delete", func(fc *filesCtx, r *http.Request) error {
		return fc.f.Remove(s.itemPaths(fc.dir, r.PostFormValue("items"))...)
	})))
	mux.HandleFunc("POST /sites/{name}/files/zip", s.auth(s.zipAction))
	mux.HandleFunc("POST /sites/{name}/files/chmod", s.auth(s.chmodAction))
	mux.HandleFunc("POST /sites/{name}/files/unzip", s.auth(s.unzipAction))
	s.editorRoutes(mux)
}

// itemPaths turns the names sent by the form (one per line, never containing a line break or a slash) into paths.
func (s *Server) itemPaths(dir, items string) []string {
	var out []string
	for _, n := range strings.Split(items, "\n") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, path.Join(dir, n))
		}
	}
	return out
}

func (s *Server) renamePage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	fc, ok := s.withFiles(w, r, ss, "dir")
	if !ok {
		return
	}
	defer fc.close()
	name := r.URL.Query().Get("name")
	if !fm.ValidName(name) {
		http.Error(w, fmMessage(fm.ErrBadName), http.StatusBadRequest)
		return
	}
	s.show(w, 200, ss, view{Page: "filerename", Site: fc.row, Dir: fc.dir, FileName: name})
}

func (s *Server) deleteConfirm(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	fc, ok := s.withFiles(w, r, ss, "dir")
	if !ok {
		return
	}
	defer fc.close()
	names := r.PostForm["item"]
	if len(names) == 0 {
		http.Error(w, fmMessage(fm.ErrNotFound), http.StatusBadRequest)
		return
	}
	for _, n := range names {
		if !fm.ValidName(n) {
			http.Error(w, fmMessage(fm.ErrBadName), http.StatusBadRequest)
			return
		}
	}
	s.show(w, 200, ss, view{Page: "confirm", Confirm: &confirm{
		Title: T("fm.delete.title", len(names)), Warning: T("fm.delete.warn"), Button: T("fm.delete.btn"), NoType: true,
		Action: "/sites/" + fc.row.Name + "/files/delete", Back: filesURL(fc.row.Name, fc.dir), Lines: names,
		Hidden: map[string]string{"dir": fc.dir, "items": strings.Join(names, "\n")}}})
}

func (s *Server) zipAction(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	fc, ok := s.withFiles(w, r, ss, "dir")
	if !ok {
		return
	}
	defer fc.close()
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(2 * time.Hour))
	name := strings.TrimSpace(r.PostFormValue("zipname"))
	if name == "" {
		name = "archive.zip"
	}
	res := &result{OK: true}
	st, err := fc.f.Zip(fc.dir, r.PostForm["item"], name, fm.DefaultLimits)
	if err != nil {
		res.OK, res.Title, res.Error = false, T("fm.done.zip.fail"), fmMessage(err)
	} else {
		res.Title = T("fm.done.zip", st.Files, st.Dirs)
		if st.Skipped > 0 {
			res.Output = T("fm.skipped", st.Skipped)
		}
	}
	s.back(w, r, fc.row.Name, fc.dir, res)
}

func (s *Server) unzipAction(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	fc, ok := s.withFiles(w, r, ss, "dir")
	if !ok {
		return
	}
	defer fc.close()
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(2 * time.Hour))
	zipName := r.PostFormValue("zip")
	res := &result{OK: true}
	if !fm.ValidName(zipName) {
		res.OK, res.Title, res.Error = false, T("fm.done.unzip.fail"), fmMessage(fm.ErrBadName)
		s.back(w, r, fc.row.Name, fc.dir, res)
		return
	}
	st, err := fc.f.Unzip(path.Join(fc.dir, zipName), fc.dir, r.PostFormValue("overwrite") != "", fm.DefaultLimits)
	if err != nil {
		res.OK, res.Title, res.Error = false, T("fm.done.unzip.fail"), fmMessage(err)
	} else {
		res.Title = T("fm.done.unzip", st.Files, st.Dirs)
		if st.Skipped > 0 {
			res.Output = T("fm.skipped", st.Skipped)
		}
	}
	s.back(w, r, fc.row.Name, fc.dir, res)
}

// binaryKind says whether files of that kind are never text, so the list does not offer to edit them.
func binaryKind(kind string) bool { return kind == "zip" || kind == "image" }
