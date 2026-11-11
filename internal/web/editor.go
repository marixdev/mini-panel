package web

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"mpn/internal/fm"
	"mpn/internal/webauth"
)

var editorFile = regexp.MustCompile(`^[A-Za-z0-9._-]{1,80}\.js$`)

func (s *Server) editorRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /sites/{name}/files/edit", s.auth(s.editPage))
	mux.HandleFunc("POST /sites/{name}/files/save", s.auth(s.editSave))
	// The editor is one small file plus a file for each language; the language files are named after their content, so
	// they can be kept for a year, while the small entry file is checked every time.
	mux.HandleFunc("GET /static/editor/{file}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("file")
		if !editorFile.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		body, err := assetFS.ReadFile("assets/editor/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		sum := sha256.Sum256(body)
		etag := `"` + hex.EncodeToString(sum[:6]) + `"`
		h := w.Header()
		h.Set("Content-Type", "text/javascript; charset=utf-8")
		h.Set("ETag", etag)
		if strings.HasPrefix(name, "chunk-") {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", "no-cache")
		}
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write(body)
	})
}

// renderEditor shows the editing page. Like the terminal page it has its own policy with a one-time nonce, because the
// editor makes style elements while it runs.
func (s *Server) renderEditor(w http.ResponseWriter, ss webauth.Session, fc *filesCtx, status int, v view) {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	nonce := base64.RawURLEncoding.EncodeToString(raw)
	w.Header().Set("Content-Security-Policy", stylePolicy(nonce))
	v.Page, v.Site, v.Nonce = "fileedit", fc.row, nonce
	v.EditPath = fc.dir
	v.FileName = path.Base(fc.dir)
	v.FileKind = fileKind(v.FileName, false)
	v.Dir = path.Dir(fc.dir)
	if v.Dir == "." {
		v.Dir = ""
	}
	s.show(w, status, ss, v)
}

func (s *Server) editPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	fc, ok := s.withFiles(w, r, ss, "path")
	if !ok {
		return
	}
	defer fc.close()
	text, mtime, err := fc.f.ReadText(fc.dir, maxEditBytes)
	if err != nil {
		parent := path.Dir(fc.dir)
		if parent == "." {
			parent = ""
		}
		s.back(w, r, fc.row.Name, parent, &result{Title: T("fm.edit.fail"), Error: fmMessageText(T, err)})
		return
	}
	v := view{EditText: text, EditMtime: mtime, CRLF: strings.Contains(text, "\r\n")}
	if c, err := r.Cookie(s.cookieName()); err == nil {
		v.Notice = s.takeFlash(c.Value)
	}
	s.renderEditor(w, ss, fc, 200, v)
}

// editSave stores the text of the editing page. A browser sends the line breaks of a text box as CRLF; they are turned
// back into what the file used (LF unless the file already used CRLF). If the file changed while it was open, the text is
// kept on the page and nothing is overwritten unless the person saves again.
func (s *Server) editSave(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	fc, ok := s.withFiles(w, r, ss, "path")
	if !ok {
		return
	}
	defer fc.close()
	content := strings.ReplaceAll(r.PostFormValue("content"), "\r\n", "\n")
	crlf := r.PostFormValue("eol") == "crlf"
	if crlf {
		content = strings.ReplaceAll(content, "\n", "\r\n")
	}
	if len(content) > maxEditBytes {
		s.renderEditor(w, ss, fc, http.StatusRequestEntityTooLarge, view{EditText: strings.ReplaceAll(content, "\r\n", "\n"), CRLF: crlf, Error: fmMessageText(T, fm.ErrTooBig)})
		return
	}
	mtime, _ := strconv.ParseInt(r.PostFormValue("mtime"), 10, 64)
	newMtime, err := fc.f.WriteText(fc.dir, content, mtime)
	if err != nil {
		v := view{EditText: strings.ReplaceAll(content, "\r\n", "\n"), EditMtime: mtime, CRLF: crlf, Error: fmMessageText(T, err)}
		status := http.StatusBadRequest
		if err == fm.ErrChanged {
			if fi, serr := fc.f.Stat(fc.dir); serr == nil {
				v.EditMtime = fi.ModTime().Unix() // saving once more is the person's choice to overwrite
			}
			v.Error = T("fm.err.changed.keep")
			status = http.StatusConflict
		}
		s.renderEditor(w, ss, fc, status, v)
		return
	}
	_ = newMtime
	if c, err := r.Cookie(s.cookieName()); err == nil {
		s.putFlash(c.Value, &result{Title: T("fm.saved"), OK: true})
	}
	http.Redirect(w, r, "/sites/"+fc.row.Name+"/files/edit?path="+url.QueryEscape(fc.dir), http.StatusSeeOther)
}
