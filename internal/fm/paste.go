package fm

import (
	"errors"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
)

// ErrInside is returned when a folder would be copied or moved into itself or into a folder below it.
var ErrInside = errors.New("a folder cannot be put inside itself")

// pastePlan checks the named items of srcRel and where each would land in dstRel, before anything is changed, so that a
// paste either starts with every item able to go or does nothing at all. When copying into the folder the items come
// from, each copy gets a free name ("name (copy)", "name (copy 2)", ...), the way a desktop does it.
func (c Client) pastePlan(srcRel string, names []string, dstRel string, copying bool) (from, to []string, err error) {
	if len(names) == 0 {
		return nil, nil, ErrNotFound
	}
	src, err := c.Abs(srcRel)
	if err != nil {
		return nil, nil, err
	}
	dst, err := c.Abs(dstRel)
	if err != nil {
		return nil, nil, err
	}
	if fi, err := c.C.Stat(dst); err != nil {
		return nil, nil, mapErr(err)
	} else if !fi.IsDir() {
		return nil, nil, ErrBadPath
	}
	// A link in the destination path could lead outside the home directory.
	if rel := strings.TrimPrefix(dst, c.Root); rel != "" && c.noLinks(c.Root, strings.TrimPrefix(rel, "/")) != nil {
		return nil, nil, ErrBadPath
	}
	same := src == dst
	taken := map[string]bool{}
	for _, n := range names {
		if !ValidName(n) {
			return nil, nil, ErrBadName
		}
		f := path.Join(src, n)
		fi, err := c.C.Lstat(f)
		if err != nil {
			return nil, nil, mapErr(err)
		}
		if fi.IsDir() && (dst == f || strings.HasPrefix(dst, f+"/")) {
			return nil, nil, ErrInside
		}
		t := path.Join(dst, n)
		if same {
			if !copying {
				continue // moving an item to where it already is changes nothing
			}
			t = c.freeName(dst, n, taken)
		} else if c.exists(t) || taken[t] {
			return nil, nil, ErrExists
		}
		taken[t] = true
		from, to = append(from, f), append(to, t)
	}
	return from, to, nil
}

// freeName finds a name for a copy next to the original that is not in use.
func (c Client) freeName(dir, name string, taken map[string]bool) string {
	stem, ext := name, ""
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		stem, ext = name[:i], name[i:]
	}
	for i := 1; ; i++ {
		suffix := " (copy)"
		if i > 1 {
			suffix = " (copy " + strconv.Itoa(i) + ")"
		}
		n := stem + suffix + ext
		if len(n) > 255 {
			n = n[len(n)-255:]
		}
		p := path.Join(dir, n)
		if !c.exists(p) && !taken[p] {
			return p
		}
	}
}

// Move moves the named items of srcRel into dstRel. Nothing is replaced: if a name is already used in the destination,
// nothing is moved. Moving within one folder does nothing.
func (c Client) Move(srcRel string, names []string, dstRel string) (Stats, error) {
	var st Stats
	from, to, err := c.pastePlan(srcRel, names, dstRel, false)
	if err != nil {
		return st, err
	}
	for i := range from {
		fi, err := c.C.Lstat(from[i])
		if err != nil {
			return st, mapErr(err)
		}
		if err := c.C.Rename(from[i], to[i]); err != nil {
			return st, mapErr(err)
		}
		if fi.IsDir() {
			st.Dirs++
		} else {
			st.Files++
		}
	}
	return st, nil
}

// Copy copies the named items of srcRel (folders with everything in them) into dstRel. Links and special files are left
// out and counted in Stats.Skipped. Each file is written under a hidden name and renamed when it is complete, and keeps
// the permission of the original. Folders are made the way Mkdir makes them, so a folder of the website keeps its group.
func (c Client) Copy(srcRel string, names []string, dstRel string, lim Limits) (Stats, error) {
	var st Stats
	from, to, err := c.pastePlan(srcRel, names, dstRel, true)
	if err != nil {
		return st, err
	}
	var walk func(f, t string) error
	walk = func(f, t string) error {
		fi, err := c.C.Lstat(f)
		if err != nil {
			return mapErr(err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			st.Skipped++
			return nil
		}
		if st.Files+st.Dirs >= lim.Entries {
			return ErrTooMany
		}
		if fi.IsDir() {
			if err := c.Mkdir(c.rel(path.Dir(t)), path.Base(t)); err != nil {
				return err
			}
			st.Dirs++
			kids, err := c.C.ReadDir(f)
			if err != nil {
				return mapErr(err)
			}
			for _, k := range kids {
				if err := walk(path.Join(f, k.Name()), path.Join(t, k.Name())); err != nil {
					return err
				}
			}
			return nil
		}
		if !fi.Mode().IsRegular() {
			st.Skipped++
			return nil
		}
		n, err := c.copyFile(f, t, fi.Mode().Perm(), lim.Bytes-st.Bytes)
		st.Bytes += n
		if err != nil {
			return err
		}
		st.Files++
		return nil
	}
	for i := range from {
		if err := walk(from[i], to[i]); err != nil {
			return st, err
		}
	}
	return st, nil
}

// rel turns an absolute path inside the home directory back into the relative form the other methods take.
func (c Client) rel(abs string) string { return strings.TrimPrefix(strings.TrimPrefix(abs, c.Root), "/") }

func (c Client) copyFile(from, to string, perm os.FileMode, room int64) (int64, error) {
	if c.exists(to) {
		return 0, ErrExists
	}
	src, err := c.C.Open(from)
	if err != nil {
		return 0, mapErr(err)
	}
	defer src.Close()
	tmp := path.Join(path.Dir(to), tempName(path.Base(to)))
	f, err := c.C.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return 0, mapErr(err)
	}
	n, cerr := io.Copy(f, io.LimitReader(src, room+1))
	if err := f.Close(); cerr == nil {
		cerr = err
	}
	if cerr == nil && n > room {
		cerr = ErrTooBig
	}
	if cerr == nil {
		cerr = c.C.Chmod(tmp, perm)
	}
	if cerr == nil {
		cerr = c.replace(tmp, to)
	}
	if cerr != nil {
		_ = c.C.Remove(tmp)
		return n, mapErr(cerr)
	}
	return n, nil
}
