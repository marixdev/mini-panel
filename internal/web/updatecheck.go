package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mpn/internal/update"
)

// The panel looks for a new release by itself: soon after it starts, then every few hours. What it finds is kept in a file next to the
// account, so a restart does not ask GitHub again at once and a page never waits for GitHub. When a newer release exists, the Update page
// shows its changelog and every page shows a notice. Once the program runs that release (or a newer one), the notice is gone by itself.

// UpdateInfo is what the last look found.
type UpdateInfo struct {
	Checked time.Time `json:"checked"`
	Latest  string    `json:"latest,omitempty"`
	Error   string    `json:"error,omitempty"`
	// The changelog in each language of the panel, of the releases after the one that ran when it was read, newest first. A language
	// whose file could not be read is missing; the page then shows the English one.
	Changelogs map[string][]update.Section `json:"changelogs,omitempty"`
}

// changelogLanguages are the languages a changelog is read in; English is the main one and comes first.
var changelogLanguages = []string{"en", "vi", "zh"}

// changelogIn is the changelog in lang, or the English one.
func (i UpdateInfo) changelogIn(lang string) []update.Section {
	if s, ok := i.Changelogs[lang]; ok && len(s) > 0 {
		return s
	}
	return i.Changelogs["en"]
}

// UpdateChecker looks for releases. Its fields are set once, before Run.
type UpdateChecker struct {
	File    string        // where the last answer is kept
	Every   time.Duration // how often GitHub is asked; 6 hours when 0
	Current func() string // the version that runs; Version when nil
	// Latest and Changelog ask GitHub; they are replaced in tests.
	Latest    func(ctx context.Context) (update.Version, error)
	Changelog func(ctx context.Context, v update.Version, lang string) (string, error)

	mu   sync.Mutex
	info UpdateInfo
	busy bool
}

func (u *UpdateChecker) current() string {
	if u.Current != nil {
		return u.Current()
	}
	return Version
}

func (u *UpdateChecker) every() time.Duration {
	if u.Every > 0 {
		return u.Every
	}
	return 6 * time.Hour
}

// Load reads the answer kept from before a restart.
func (u *UpdateChecker) Load() {
	if u.File == "" {
		return
	}
	b, err := os.ReadFile(u.File)
	if err != nil {
		return
	}
	var info UpdateInfo
	if json.Unmarshal(b, &info) == nil {
		u.mu.Lock()
		u.info = info
		u.mu.Unlock()
	}
}

// Info is the last answer.
func (u *UpdateChecker) Info() UpdateInfo {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.info
}

// Available is the newer release and its changelog in lang, or "" when the program runs the latest one (or nothing is known yet). It
// is read against the version that runs now, so after an update the notice goes away before GitHub is asked again.
func (u *UpdateChecker) Available(lang string) (string, []update.Section) {
	info := u.Info()
	cur, err := update.ParseVersion(u.current())
	if err != nil || info.Latest == "" {
		return "", nil
	}
	latest, err := update.ParseVersion(info.Latest)
	if err != nil || !cur.Less(latest) {
		return "", nil
	}
	return latest.String(), update.Newer(info.changelogIn(lang), cur)
}

// Run checks soon after the start and then every few hours, until ctx ends. A build without a release number is never checked.
func (u *UpdateChecker) Run(ctx context.Context) {
	if _, err := update.ParseVersion(u.current()); err != nil {
		return
	}
	wait := 30 * time.Second
	if info := u.Info(); !info.Checked.IsZero() {
		if since := time.Since(info.Checked); since < u.every() {
			wait = u.every() - since
		}
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		u.Check(ctx)
		t.Reset(u.every())
	}
}

// Check asks GitHub now. A second call while one runs returns at once.
func (u *UpdateChecker) Check(ctx context.Context) UpdateInfo {
	u.mu.Lock()
	if u.busy {
		info := u.info
		u.mu.Unlock()
		return info
	}
	u.busy = true
	old := u.info
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.busy = false
		u.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	info := UpdateInfo{Checked: time.Now().UTC()}
	latest, err := u.latest(ctx)
	if err != nil {
		info.Error = err.Error()
		info.Latest, info.Changelogs = old.Latest, old.Changelogs // what was known stays known
		u.save(info)
		return info
	}
	info.Latest = latest.String()
	cur, cerr := update.ParseVersion(u.current())
	switch {
	case cerr != nil || !cur.Less(latest):
		// Nothing newer: no changelog to keep.
	case old.Latest == info.Latest && len(old.Changelogs["en"]) > 0 && len(old.Changelogs) == len(changelogLanguages):
		info.Changelogs = old.Changelogs // the same release, read in full already: it is not read again
	default:
		info.Changelogs = map[string][]update.Section{}
		for _, lang := range changelogLanguages {
			if s, ok := old.Changelogs[lang]; ok && old.Latest == info.Latest && len(s) > 0 {
				info.Changelogs[lang] = s
				continue
			}
			md, err := u.changelog(ctx, latest, lang)
			if err != nil {
				// A translation may come after the release; the English one is shown meanwhile. Only a missing English one is an error.
				if lang == "en" {
					info.Error = "changelog: " + err.Error()
				}
				continue
			}
			info.Changelogs[lang] = update.Newer(update.Sections(md, latest), cur)
		}
	}
	u.save(info)
	return info
}

func (u *UpdateChecker) latest(ctx context.Context) (update.Version, error) {
	if u.Latest != nil {
		return u.Latest(ctx)
	}
	return update.LatestTag(ctx, update.DefaultClient())
}

func (u *UpdateChecker) changelog(ctx context.Context, v update.Version, lang string) (string, error) {
	if u.Changelog != nil {
		return u.Changelog(ctx, v, lang)
	}
	b, err := update.Fetch(ctx, update.DefaultClient(), update.ChangelogURL(v, lang), 512<<10)
	return string(b), err
}

func (u *UpdateChecker) save(info UpdateInfo) {
	u.mu.Lock()
	u.info = info
	u.mu.Unlock()
	if u.File == "" {
		return
	}
	b, err := json.Marshal(info)
	if err == nil {
		err = writeFileAtomic(u.File, b)
	}
	if err != nil {
		log.Printf("update check: could not keep the answer: %v", err)
	}
}

func writeFileAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".update-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// updateNotice is what the notice on every page needs: the release. Whether this browser closed it for that release is app.js's.
type updateNotice struct {
	Version string
}

// noticeFor is the notice for a page, or nil. The Update page has the full story, so it has no notice.
func (s *Server) noticeFor(page string) *updateNotice {
	if s.Updates == nil || page == "update" {
		return nil
	}
	if v, _ := s.Updates.Available("en"); v != "" {
		return &updateNotice{Version: v}
	}
	return nil
}

// updateCheckNow is the "Check now" button: it asks GitHub while the person waits, then shows the Update page again.
func (s *Server) updateCheckNow(w http.ResponseWriter, r *http.Request) {
	if s.Updates != nil {
		s.Updates.Check(r.Context())
	}
	http.Redirect(w, r, "/update", http.StatusSeeOther)
}
