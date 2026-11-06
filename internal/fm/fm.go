// Package fm is the file manager of a website: it lists, reads, writes, uploads, renames and deletes files in the home
// directory of the website's own account, over SFTP. Everything runs as that account, so it can do no more than the
// account can; this package adds the rule that a path from a web page can never leave the home directory.
package fm

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pkg/sftp"
)

var (
	ErrBadPath  = errors.New("invalid path")
	ErrBadName  = errors.New("invalid name")
	ErrExists   = errors.New("already exists")
	ErrNotFound = errors.New("not found")
	ErrDenied   = errors.New("permission denied")
	ErrBinary   = errors.New("not a text file")
	ErrChanged  = errors.New("the file changed after it was opened")
	ErrTooBig   = errors.New("too big")
	ErrIsDir    = errors.New("is a directory")
	ErrRoot     = errors.New("the home directory itself cannot be changed this way")
)

// MaxEntries is how many entries of one directory are listed; a variable so that a test can lower it.
var MaxEntries = 5000

type Client struct {
	C    *sftp.Client
	Root string // the home directory, absolute and clean; every path stays inside it
}

type Entry struct {
	Name    string
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
	IsDir   bool // a directory, or a link to one
	IsLink  bool
}

func badChars(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// Abs turns a path written relative to the home directory into the absolute path to use. A path with "..", a backslash
// or a control character is refused instead of being quietly tidied, so a request that tries to leave the directory
// fails loudly.
func (c Client) Abs(rel string) (string, error) {
	if len(rel) > 4096 || strings.ContainsRune(rel, '\\') || badChars(rel) {
		return "", ErrBadPath
	}
	for _, el := range strings.Split(rel, "/") {
		if el == ".." {
			return "", ErrBadPath
		}
	}
	return path.Join(c.Root, path.Clean("/"+rel)), nil
}

// Clean returns the tidy form of a relative path: no leading or trailing slash, "" for the home directory itself.
func Clean(rel string) string { return strings.TrimPrefix(path.Clean("/"+rel), "/") }

// ValidName reports whether s can be the name of one file or folder.
func ValidName(s string) bool {
	return s != "" && s != "." && s != ".." && len(s) <= 255 && !strings.ContainsAny(s, "/\\") && !badChars(s) && utf8.ValidString(s)
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var se *sftp.StatusError
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return ErrNotFound
	case errors.Is(err, fs.ErrExist):
		return ErrExists
	case errors.Is(err, fs.ErrPermission):
		return ErrDenied
	case errors.As(err, &se):
		switch se.Code {
		case 2:
			return ErrNotFound
		case 3:
			return ErrDenied
		case 11:
			return ErrExists
		}
	}
	return err
}

// List returns the entries of a directory, folders first, and whether the list was cut at MaxEntries.
func (c Client) List(rel string) ([]Entry, bool, error) {
	p, err := c.Abs(rel)
	if err != nil {
		return nil, false, err
	}
	infos, err := c.C.ReadDir(p)
	if err != nil {
		return nil, false, mapErr(err)
	}
	cut := len(infos) > MaxEntries
	if cut {
		infos = infos[:MaxEntries]
	}
	out := make([]Entry, 0, len(infos))
	for _, fi := range infos {
		e := Entry{Name: fi.Name(), Size: fi.Size(), Mode: fi.Mode(), ModTime: fi.ModTime(), IsDir: fi.IsDir(), IsLink: fi.Mode()&os.ModeSymlink != 0}
		if e.IsLink {
			if target, err := c.C.Stat(path.Join(p, fi.Name())); err == nil && target.IsDir() {
				e.IsDir = true
			}
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, cut, nil
}

func (c Client) Stat(rel string) (os.FileInfo, error) {
	p, err := c.Abs(rel)
	if err != nil {
		return nil, err
	}
	fi, err := c.C.Stat(p)
	return fi, mapErr(err)
}

// child is the absolute path of a new name inside a directory that exists.
func (c Client) child(dirRel, name string) (string, error) {
	if !ValidName(name) {
		return "", ErrBadName
	}
	d, err := c.Abs(dirRel)
	if err != nil {
		return "", err
	}
	if fi, err := c.C.Stat(d); err != nil {
		return "", mapErr(err)
	} else if !fi.IsDir() {
		return "", ErrBadPath
	}
	return path.Join(d, name), nil
}

func (c Client) exists(p string) bool { _, err := c.C.Lstat(p); return err == nil }

func (c Client) Mkdir(dirRel, name string) error {
	p, err := c.child(dirRel, name)
	if err != nil {
		return err
	}
	if c.exists(p) {
		return ErrExists
	}
	if err := c.C.Mkdir(p); err != nil {
		return mapErr(err)
	}
	// A folder made inside a setgid folder (the folder of a website) inherits that bit and its group. A chmod would remove
	// the bit, because the account is not a member of that group (measured on Ubuntu 22.04 and 24.04). So the mode is left
	// as the session's umask made it in that case, which is what a folder made over SSH gets, and is set to 755 otherwise.
	if fi, err := c.C.Lstat(p); err == nil && (fi.Mode().Perm() == 0o755 || fi.Mode()&os.ModeSetgid != 0) {
		return nil
	}
	return mapErr(c.C.Chmod(p, 0o755))
}

// Create makes an empty file and refuses to touch one that exists.
func (c Client) Create(dirRel, name string) error {
	p, err := c.child(dirRel, name)
	if err != nil {
		return err
	}
	// O_EXCL below is the second line of defence: servers report a refused exclusive create with different status codes,
	// so the plain check comes first and is what gives the clear answer.
	if c.exists(p) {
		return ErrExists
	}
	f, err := c.C.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return mapErr(err)
	}
	if err := f.Close(); err != nil {
		return mapErr(err)
	}
	return mapErr(c.C.Chmod(p, 0o644))
}

// Rename gives a file or folder a new name in the same directory. It never replaces something that is there.
func (c Client) Rename(rel, newName string) error {
	if Clean(rel) == "" {
		return ErrRoot
	}
	from, err := c.Abs(rel)
	if err != nil {
		return err
	}
	if !ValidName(newName) {
		return ErrBadName
	}
	to := path.Join(path.Dir(from), newName)
	if !c.exists(from) {
		return ErrNotFound
	}
	if c.exists(to) {
		return ErrExists
	}
	return mapErr(c.C.Rename(from, to))
}

// Remove deletes files and folders (folders with everything in them). A link is removed, never followed.
func (c Client) Remove(rels ...string) error {
	for _, rel := range rels {
		if Clean(rel) == "" {
			return ErrRoot
		}
		if _, err := c.Abs(rel); err != nil {
			return err
		}
	}
	for _, rel := range rels {
		p, _ := c.Abs(rel)
		fi, err := c.C.Lstat(p)
		if err != nil {
			return mapErr(err)
		}
		if fi.IsDir() {
			err = c.C.RemoveAll(p)
		} else {
			err = c.C.Remove(p)
		}
		if err != nil {
			return mapErr(err)
		}
	}
	return nil
}

// ReadText returns a file's text and its time of change (in seconds, to hand back to WriteText). Anything that is too
// big, contains a zero byte, or is not valid UTF-8 is refused: a web form would send it back changed.
func (c Client) ReadText(rel string, max int64) (string, int64, error) {
	p, err := c.Abs(rel)
	if err != nil {
		return "", 0, err
	}
	fi, err := c.C.Stat(p)
	if err != nil {
		return "", 0, mapErr(err)
	}
	if fi.IsDir() {
		return "", 0, ErrIsDir
	}
	if fi.Size() > max {
		return "", 0, ErrTooBig
	}
	f, err := c.C.Open(p)
	if err != nil {
		return "", 0, mapErr(err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return "", 0, mapErr(err)
	}
	if int64(len(b)) > max {
		return "", 0, ErrTooBig
	}
	if strings.IndexByte(string(b), 0) >= 0 || !utf8.Valid(b) {
		return "", 0, ErrBinary
	}
	return string(b), fi.ModTime().Unix(), nil
}

// replace moves from over to, so that to is never missing or half written. With OpenSSH's sftp-server that is one
// atomic step. A server without that step gets two renames, and the old file is put back if the second one fails.
func (c Client) replace(from, to string) error {
	if _, ok := c.C.HasExtension("posix-rename@openssh.com"); ok {
		return c.C.PosixRename(from, to)
	}
	if !c.exists(to) {
		return c.C.Rename(from, to)
	}
	backup := to + ".mpn-old-" + tempName("")[1:]
	if err := c.C.Rename(to, backup); err != nil {
		return err
	}
	if err := c.C.Rename(from, to); err != nil {
		_ = c.C.Rename(backup, to)
		return err
	}
	_ = c.C.Remove(backup)
	return nil
}

func tempName(name string) string {
	r := make([]byte, 4)
	_, _ = rand.Read(r)
	return "." + name + ".mpn-" + hex.EncodeToString(r)
}

// WriteText replaces a file's content. The new content is written to a hidden file next to it and renamed over it, so
// a reader never sees half a file, and the mode of the file is kept. If the file changed since it was read
// (expect is the time ReadText gave), nothing is written and ErrChanged is returned.
func (c Client) WriteText(rel, content string, expect int64) (int64, error) {
	p, err := c.Abs(rel)
	if err != nil {
		return 0, err
	}
	li, err := c.C.Lstat(p)
	if err != nil {
		return 0, mapErr(err)
	}
	if li.IsDir() {
		return 0, ErrIsDir
	}
	if li.ModTime().Unix() != expect {
		return 0, ErrChanged
	}
	if li.Mode()&os.ModeSymlink != 0 { // write through the link; a rename would replace the link itself
		f, err := c.C.OpenFile(p, os.O_WRONLY|os.O_TRUNC)
		if err != nil {
			return 0, mapErr(err)
		}
		_, werr := f.Write([]byte(content))
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return 0, mapErr(werr)
		}
	} else {
		tmp := path.Join(path.Dir(p), tempName(path.Base(p)))
		f, err := c.C.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
		if err != nil {
			return 0, mapErr(err)
		}
		_, werr := f.Write([]byte(content))
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr == nil {
			werr = c.C.Chmod(tmp, li.Mode().Perm())
		}
		if werr == nil {
			werr = c.replace(tmp, p)
		}
		if werr != nil {
			_ = c.C.Remove(tmp)
			return 0, mapErr(werr)
		}
	}
	fi, err := c.C.Stat(p)
	if err != nil {
		return 0, mapErr(err)
	}
	return fi.ModTime().Unix(), nil
}

// Open opens a file for download.
func (c Client) Open(rel string) (*sftp.File, os.FileInfo, error) {
	p, err := c.Abs(rel)
	if err != nil {
		return nil, nil, err
	}
	fi, err := c.C.Stat(p)
	if err != nil {
		return nil, nil, mapErr(err)
	}
	if fi.IsDir() {
		return nil, nil, ErrIsDir
	}
	f, err := c.C.Open(p)
	return f, fi, mapErr(err)
}

// Upload stores what r delivers as a new file, through a hidden file that is renamed when it is complete. It stops with
// ErrTooBig after max bytes, and refuses to replace an existing file unless overwrite is set.
func (c Client) Upload(dirRel, name string, r io.Reader, overwrite bool, max int64) (int64, error) {
	p, err := c.child(dirRel, name)
	if err != nil {
		return 0, err
	}
	if c.exists(p) && !overwrite {
		return 0, ErrExists
	}
	tmp := path.Join(path.Dir(p), tempName(name))
	f, err := c.C.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return 0, mapErr(err)
	}
	n, cerr := io.Copy(f, io.LimitReader(r, max+1))
	if err := f.Close(); cerr == nil {
		cerr = err
	}
	if cerr == nil && n > max {
		cerr = ErrTooBig
	}
	if cerr == nil {
		cerr = c.C.Chmod(tmp, 0o644)
	}
	if cerr == nil {
		cerr = c.replace(tmp, p)
	}
	if cerr != nil {
		_ = c.C.Remove(tmp)
		return 0, mapErr(cerr)
	}
	return n, nil
}
