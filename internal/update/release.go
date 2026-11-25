package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// What a new release says about itself: its number, read from where GitHub sends /releases/latest, and its part of the changelog,
// read from the CHANGELOG file of its tag. Neither is trusted to install anything; the program itself is checked against the release
// key when it is installed. They are only shown.

// LatestTag asks GitHub for the newest release. Only the redirect is read, so no API rate limit applies.
func LatestTag(ctx context.Context, c *http.Client) (Version, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://github.com/"+Repo+"/releases/latest", nil)
	if err != nil {
		return Version{}, err
	}
	nc := *c
	nc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := nc.Do(req)
	if err != nil {
		return Version{}, err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	i := strings.LastIndex(loc, "/tag/")
	if i < 0 {
		return Version{}, fmt.Errorf("could not find the latest release (%s)", resp.Status)
	}
	return ParseVersion(loc[i+5:])
}

// ChangelogURL is the changelog of a release in one language: "en", "vi" or "zh". English is the main one.
func ChangelogURL(v Version, lang string) string {
	name := "CHANGELOG.md"
	if lang == "vi" || lang == "zh" {
		name = "CHANGELOG." + lang + ".md"
	}
	return "https://raw.githubusercontent.com/" + Repo + "/v" + v.String() + "/" + name
}

// Fetch reads a URL of at most max bytes.
func Fetch(ctx context.Context, c *http.Client, url string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("%s is larger than %d bytes", url, max)
	}
	return b, nil
}

// Section is the part of the changelog about one release.
type Section struct {
	Version string `json:"version"`
	Date    string `json:"date,omitempty"`
	Body    string `json:"body"`
}

// MaxSections is how many releases are kept from a changelog, the newest first.
const MaxSections = 12

// Sections reads the "## [1.2.3] - 2026-11-22" parts of a changelog, newest first, up to MaxSections, leaving out any part whose
// number is newer than upto (a changelog may list work that is not released yet) or is not a version.
func Sections(md string, upto Version) []Section {
	var out []Section
	var cur *Section
	var body []string
	flush := func() {
		if cur != nil {
			cur.Body = strings.TrimSpace(strings.Join(body, "\n"))
			out = append(out, *cur)
		}
		cur, body = nil, nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			head := strings.TrimSpace(line[3:])
			if !strings.HasPrefix(head, "[") {
				continue
			}
			end := strings.Index(head, "]")
			if end < 0 {
				continue
			}
			v, err := ParseVersion(head[1:end])
			if err != nil || upto.Less(v) {
				continue
			}
			date := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(head[end+1:]), "-–— "))
			if len(out) >= MaxSections {
				break
			}
			cur = &Section{Version: v.String(), Date: date}
			continue
		}
		if cur != nil {
			body = append(body, line)
		}
	}
	flush()
	if len(out) > MaxSections {
		out = out[:MaxSections]
	}
	return out
}

// Newer keeps the sections of the releases after current, newest first.
func Newer(all []Section, current Version) []Section {
	var out []Section
	for _, s := range all {
		if v, err := ParseVersion(s.Version); err == nil && current.Less(v) {
			out = append(out, s)
		}
	}
	return out
}

// DefaultClient is the client the panel asks GitHub with.
func DefaultClient() *http.Client { return &http.Client{Timeout: 30 * time.Second} }
