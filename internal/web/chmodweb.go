package web

import (
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"mpn/internal/fm"
	"mpn/internal/webauth"
)

// A permission comes from the page as three octal digits: owner, group, others. Anything else (including the special
// bits a fourth digit would carry) is refused.
var permRe = regexp.MustCompile(`^[0-7]{3}$`)

func (s *Server) chmodAction(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	fc, ok := s.withFiles(w, r, ss, "dir")
	if !ok {
		return
	}
	defer fc.close()
	res := &result{OK: true}
	fail := func(err error) {
		res.OK, res.Title, res.Error = false, T("fm.done.chmod.fail"), fmMessageText(T, err)
		s.back(w, r, fc.row.Name, fc.dir, res)
	}
	mode := r.PostFormValue("mode")
	if !permRe.MatchString(mode) {
		fail(fm.ErrBadMode)
		return
	}
	n, _ := strconv.ParseUint(mode, 8, 32)
	var paths []string
	for _, name := range r.PostForm["item"] {
		if !fm.ValidName(name) {
			fail(fm.ErrBadName)
			return
		}
		paths = append(paths, path.Join(fc.dir, name))
	}
	scope := r.PostFormValue("scope")
	opts := fm.ChmodOpts{Recursive: r.PostFormValue("recursive") != "", Files: scope != "dirs", Dirs: scope != "files"}
	st, err := fc.f.Chmod(paths, os.FileMode(n), opts, fm.DefaultLimits)
	if err != nil {
		fail(err)
		return
	}
	res.Title = T("fm.done.chmod", st.Files, st.Dirs)
	var notes []string
	if st.Skipped > 0 {
		notes = append(notes, T("fm.skipped", st.Skipped))
	}
	if st.Lost > 0 {
		notes = append(notes, T("fm.perm.lost", st.Lost))
	}
	res.Output = strings.Join(notes, "\n")
	s.back(w, r, fc.row.Name, fc.dir, res)
}

// modeText is the permission of a file as shown and edited on the page: the nine bits, in octal.
func modeText(m os.FileMode) string {
	return strings.TrimSpace(strconv.FormatUint(uint64(m.Perm()), 8))
}
