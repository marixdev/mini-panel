package fm

import (
	"archive/zip"
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// Limits keep an archive from filling the disk or the memory: a zip file can say it holds a few kilobytes and expand
// to terabytes, so the size that counts is the one actually read.
type Limits struct {
	Entries int
	Bytes   int64
}

var DefaultLimits = Limits{Entries: 100000, Bytes: 4 << 30}

// Stats says what an archive operation did.
type Stats struct {
	Files, Dirs, Skipped int
	// Lost counts folders whose setgid bit the system removed while their permissions were changed.
	Lost  int
	Bytes int64
}

var ErrTooMany = errors.New("too many entries")

// Zip packs the named items of one directory (folders with all they hold) into a new zip file in that directory. Links
// are left out, not followed. The archive is built under a hidden name and renamed when it is complete.
func (c Client) Zip(dirRel string, names []string, zipName string, lim Limits) (Stats, error) {
	var st Stats
	if !strings.HasSuffix(strings.ToLower(zipName), ".zip") {
		zipName += ".zip"
	}
	final, err := c.child(dirRel, zipName)
	if err != nil {
		return st, err
	}
	if c.exists(final) {
		return st, ErrExists
	}
	dir := path.Dir(final)
	var roots []string
	for _, n := range names {
		if !ValidName(n) {
			return st, ErrBadName
		}
		p := path.Join(dir, n)
		if _, err := c.C.Lstat(p); err != nil {
			return st, mapErr(err)
		}
		roots = append(roots, p)
	}
	if len(roots) == 0 {
		return st, ErrNotFound
	}
	tmp := path.Join(dir, tempName(zipName))
	f, err := c.C.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return st, mapErr(err)
	}
	bw := bufio.NewWriterSize(f, 256<<10)
	zw := zip.NewWriter(bw)
	var add func(p string) error
	add = func(p string) error {
		fi, err := c.C.Lstat(p)
		if err != nil {
			return mapErr(err)
		}
		if fi.Mode()&os.ModeSymlink != 0 || p == tmp {
			st.Skipped++
			return nil
		}
		if st.Files+st.Dirs >= lim.Entries {
			return ErrTooMany
		}
		h, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}
		h.Name = strings.TrimPrefix(p, dir+"/")
		if fi.IsDir() {
			h.Name += "/"
			if _, err := zw.CreateHeader(h); err != nil {
				return err
			}
			st.Dirs++
			infos, err := c.C.ReadDir(p)
			if err != nil {
				return mapErr(err)
			}
			for _, child := range infos {
				if err := add(path.Join(p, child.Name())); err != nil {
					return err
				}
			}
			return nil
		}
		if !fi.Mode().IsRegular() {
			st.Skipped++ // sockets, devices and the like
			return nil
		}
		h.Method = zip.Deflate
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		src, err := c.C.Open(p)
		if err != nil {
			return mapErr(err)
		}
		defer src.Close()
		n, err := io.Copy(w, io.LimitReader(src, lim.Bytes-st.Bytes+1))
		st.Bytes += n
		if err == nil && st.Bytes > lim.Bytes {
			err = ErrTooBig
		}
		if err != nil {
			return mapErr(err)
		}
		st.Files++
		return nil
	}
	var werr error
	for _, r := range roots {
		if werr = add(r); werr != nil {
			break
		}
	}
	if werr == nil {
		werr = zw.Close()
	}
	if werr == nil {
		werr = bw.Flush()
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = c.C.Chmod(tmp, 0o644)
	}
	if werr == nil {
		werr = c.replace(tmp, final)
	}
	if werr != nil {
		_ = c.C.Remove(tmp)
		return Stats{}, mapErr(werr)
	}
	return st, nil
}

// entryPath turns the name inside a zip file into a safe relative path, or reports that it must not be used. The
// well-known attacks are all refused here: "../" to climb out of the folder, a leading slash or drive letter, and
// backslashes, which some tools write as separators and which would otherwise hide a "..".
func entryPath(name string) (string, bool) {
	if name == "" || len(name) > 1024 || badChars(name) {
		return "", false
	}
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(name, "/") || (len(name) >= 2 && name[1] == ':') {
		return "", false
	}
	var parts []string
	for _, el := range strings.Split(strings.TrimSuffix(name, "/"), "/") {
		switch el {
		case "", ".":
			continue
		case "..":
			return "", false
		}
		if len(el) > 255 {
			return "", false
		}
		parts = append(parts, el)
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "/"), true
}

// noLinks refuses a path whose existing parts include a link: writing "through" a link in the folder could land
// outside the home directory.
func (c Client) noLinks(base, rel string) error {
	cur := base
	for _, el := range strings.Split(rel, "/") {
		cur = path.Join(cur, el)
		fi, err := c.C.Lstat(cur)
		if err != nil {
			return nil // the rest does not exist yet, so it cannot be a link
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return ErrBadPath
		}
	}
	return nil
}

// Unzip extracts a zip file into a folder that exists. Entries that are unsafe or that are links are skipped and counted;
// files that are already there are kept unless overwrite is set. The extracted size is counted as it is written and
// stops at the limit, whatever the archive claims.
func (c Client) Unzip(zipRel, destRel string, overwrite bool, lim Limits) (Stats, error) {
	var st Stats
	zp, err := c.Abs(zipRel)
	if err != nil {
		return st, err
	}
	dest, err := c.Abs(destRel)
	if err != nil {
		return st, err
	}
	if fi, err := c.C.Stat(dest); err != nil {
		return st, mapErr(err)
	} else if !fi.IsDir() {
		return st, ErrBadPath
	}
	zf, err := c.C.Open(zp)
	if err != nil {
		return st, mapErr(err)
	}
	defer zf.Close()
	info, err := zf.Stat()
	if err != nil {
		return st, mapErr(err)
	}
	zr, err := zip.NewReader(zf, info.Size())
	if err != nil {
		return st, fmt.Errorf("not a zip file: %w", err)
	}
	if len(zr.File) > lim.Entries {
		return st, ErrTooMany
	}
	for _, e := range zr.File {
		rel, ok := entryPath(e.Name)
		if !ok || e.Mode()&os.ModeSymlink != 0 || (!e.FileInfo().IsDir() && !e.Mode().IsRegular()) {
			st.Skipped++
			continue
		}
		if c.noLinks(dest, rel) != nil {
			st.Skipped++
			continue
		}
		target := path.Join(dest, rel)
		if e.FileInfo().IsDir() {
			if err := c.C.MkdirAll(target); err != nil {
				return st, mapErr(err)
			}
			st.Dirs++
			continue
		}
		if err := c.C.MkdirAll(path.Dir(target)); err != nil {
			return st, mapErr(err)
		}
		if c.exists(target) && !overwrite {
			st.Skipped++
			continue
		}
		n, err := c.extract(e, target, lim.Bytes-st.Bytes)
		st.Bytes += n
		if err != nil {
			return st, err
		}
		st.Files++
	}
	return st, nil
}

func (c Client) extract(e *zip.File, target string, room int64) (int64, error) {
	rc, err := e.Open()
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	tmp := path.Join(path.Dir(target), tempName(path.Base(target)))
	f, err := c.C.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return 0, mapErr(err)
	}
	n, cerr := io.Copy(f, io.LimitReader(rc, room+1))
	if err := f.Close(); cerr == nil {
		cerr = err
	}
	if cerr == nil && n > room {
		cerr = ErrTooBig
	}
	mode := os.FileMode(0o644) // never the setuid, setgid or sticky bits an archive may carry
	if e.Mode().Perm()&0o111 != 0 {
		mode = 0o755
	}
	if cerr == nil {
		cerr = c.C.Chmod(tmp, mode)
	}
	if cerr == nil {
		cerr = c.replace(tmp, target)
	}
	if cerr != nil {
		_ = c.C.Remove(tmp)
		return n, mapErr(cerr)
	}
	if !e.Modified.IsZero() {
		_ = c.C.Chtimes(target, e.Modified, e.Modified)
	}
	return n, nil
}
