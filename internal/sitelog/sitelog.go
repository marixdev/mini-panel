// Package sitelog reads the end of a website's log for the panel. It only reads: it never follows a link out of the log folder,
// never runs a shell, and keeps what it returns small enough for the answer of the root helper.
package sitelog

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// The kinds of log a site can have. "app" is the output of a Node or Python site's service, read from the journal.
const (
	Access = "access"
	Error  = "error"
	App    = "app"
)

const (
	DefaultLines = 200
	MaxLines     = 1000
	MaxQuery     = 100
	// maxTail is how much of the end of a file is looked at. A busy site writes far more than a person can read, and a search
	// finds only what is in this part.
	maxTail = 2 << 20
	// maxLine keeps one very long request line from filling the answer.
	maxLine = 600
)

// Result is what "mpn site log" prints and the web page draws.
type Result struct {
	Kind      string   `json:"kind"`
	Path      string   `json:"path"`
	Exists    bool     `json:"exists"`
	Size      int64    `json:"size"`      // size of the whole file, 0 for the journal
	Lines     []string `json:"lines"`     // oldest first
	Matched   int      `json:"matched"`   // lines of the part looked at that match the search (all of them without one)
	Truncated bool     `json:"truncated"` // the file is longer than the part looked at
}

// ValidKind reports whether k is a kind of log. The kind decides which file is read, so nothing else is accepted.
func ValidKind(k string) error {
	switch k {
	case Access, Error, App:
		return nil
	}
	return fmt.Errorf("log kind %q must be access, error or app", k)
}

// ValidLines returns n, or the default when it is 0, and refuses a count outside 1 to MaxLines.
func ValidLines(n int) (int, error) {
	if n == 0 {
		return DefaultLines, nil
	}
	if n < 1 || n > MaxLines {
		return 0, fmt.Errorf("the number of lines must be between 1 and %d", MaxLines)
	}
	return n, nil
}

// ValidQuery accepts a short piece of plain text. It is matched as text, never as a pattern.
func ValidQuery(q string) error {
	if len(q) > MaxQuery {
		return fmt.Errorf("the search text is longer than %d characters", MaxQuery)
	}
	if !utf8.ValidString(q) {
		return errors.New("the search text is not valid UTF-8")
	}
	for _, r := range q {
		if r < 0x20 || r == 0x7f {
			return errors.New("the search text has a control character")
		}
	}
	return nil
}

// Tail reads the end of the file at path: at most maxTail bytes, cut at a line start. size is the length of the whole file.
// A file that is not there is not an error; exists is false. Only a regular file is read, so a link put in the folder cannot
// lead anywhere else.
func Tail(path string) (data []byte, size int64, exists bool, err error) {
	fi, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	if !fi.Mode().IsRegular() {
		return nil, 0, false, fmt.Errorf("%s is not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, false, err
	}
	defer f.Close()
	size = fi.Size()
	start := int64(0)
	if size > maxTail {
		start = size - maxTail
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, 0, false, err
	}
	data, err = io.ReadAll(io.LimitReader(f, maxTail))
	if err != nil {
		return nil, 0, false, err
	}
	if start > 0 { // the first line is cut in the middle: leave it out
		if i := strings.IndexByte(string(data), '\n'); i >= 0 {
			data = data[i+1:]
		} else {
			data = nil
		}
	}
	return data, size, true, nil
}

// Select keeps the lines of text that contain query (all of them when it is empty), then the last n of those. Lines are cut
// at maxLine bytes and a control character other than a tab becomes "?", so a request that carried escape codes cannot
// change how the page looks.
func Select(text string, n int, query string) (lines []string, matched int) {
	all := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(all) == 1 && all[0] == "" {
		return []string{}, 0
	}
	out := make([]string, 0, n)
	for _, l := range all {
		l = strings.TrimRight(l, "\r")
		if query != "" && !strings.Contains(l, query) {
			continue
		}
		matched++
		out = append(out, clean(l))
		if len(out) > n {
			out = out[1:]
		}
	}
	return out, matched
}

func clean(l string) string {
	if len(l) > maxLine {
		l = l[:maxLine]
		for !utf8.ValidString(l) {
			l = l[:len(l)-1]
		}
		l += " ..."
	}
	return strings.Map(func(r rune) rune {
		if (r < 0x20 && r != '\t') || r == 0x7f || r == utf8.RuneError {
			return '?'
		}
		return r
	}, l)
}
