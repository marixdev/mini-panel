package web

import (
	"net/http"
	"time"

	"mpn/internal/fm"
	"mpn/internal/webauth"
)

// pasteAction finishes a cut or a copy: the page remembers which items of which folder were cut or copied, and sends
// them here with the folder that is open when "Paste" is chosen ("dir"). The items always come as names inside "from",
// never as paths, so every one of them is checked the same way as the other actions check theirs.
func (s *Server) pasteAction(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	fc, ok := s.withFiles(w, r, ss, "dir")
	if !ok {
		return
	}
	defer fc.close()
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(2 * time.Hour))
	res := &result{OK: true}
	from := r.PostFormValue("from")
	if _, err := fc.f.Abs(from); err != nil {
		res.OK, res.Title, res.Error = false, T("fm.done.paste.fail"), fmMessageText(T, err)
		s.back(w, r, fc.row.Name, fc.dir, res)
		return
	}
	names := r.PostForm["item"]
	var st fm.Stats
	var err error
	cut := r.PostFormValue("mode") == "cut"
	if cut {
		st, err = fc.f.Move(fm.Clean(from), names, fc.dir)
	} else {
		st, err = fc.f.Copy(fm.Clean(from), names, fc.dir, fm.DefaultLimits)
	}
	switch {
	case err != nil:
		res.OK, res.Title, res.Error = false, T("fm.done.paste.fail"), fmMessageText(T, err)
		if st.Files+st.Dirs > 0 {
			res.Output = T("fm.paste.partial", st.Files, st.Dirs)
		}
	case cut:
		res.Title = T("fm.done.move", st.Files, st.Dirs)
	default:
		res.Title = T("fm.done.copy", st.Files, st.Dirs)
		if st.Skipped > 0 {
			res.Output = T("fm.skipped", st.Skipped)
		}
	}
	s.back(w, r, fc.row.Name, fc.dir, res)
}
