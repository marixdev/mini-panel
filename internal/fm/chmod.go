package fm

import (
	"errors"
	"os"
	"path"
)

// ErrBadMode is returned for a permission that is not made of the nine owner, group and others bits.
var ErrBadMode = errors.New("invalid permission")

// ChmodOpts says what a change reaches.
type ChmodOpts struct {
	// Recursive also goes through everything below the folders that are named. Links are never followed.
	Recursive bool
	// Files and Dirs choose, when Recursive is set, which kinds are changed. When neither is set, both are.
	Files, Dirs bool
}

// Chmod sets the owner, group and others permission bits of files and folders (Stats.Files and Stats.Dirs count what was
// changed, Stats.Skipped the links that were left alone). The special bits (setuid, setgid, sticky) are passed on as they
// were, and nothing is sent for an item that already has the permission asked for. That is not always enough: the system
// removes the setgid bit of a folder when its permissions change and the account is not a member of the folder's group,
// which is the case for the folder of a website (group www-data). Stats.Lost counts those, so the page can say so.
// The home directory itself is refused, because nginx and PHP need it to stay as it is.
//
// A folder is changed after what is inside it, so that taking away its read or search permission cannot stop the walk.
func (c Client) Chmod(rels []string, perm os.FileMode, o ChmodOpts, lim Limits) (Stats, error) {
	var st Stats
	if perm&^os.ModePerm != 0 {
		return st, ErrBadMode
	}
	if len(rels) == 0 {
		return st, ErrNotFound
	}
	for _, rel := range rels {
		if Clean(rel) == "" {
			return st, ErrRoot
		}
		if _, err := c.Abs(rel); err != nil {
			return st, err
		}
	}
	if !o.Files && !o.Dirs {
		o.Files, o.Dirs = true, true
	}
	for _, rel := range rels {
		p, _ := c.Abs(rel)
		fi, err := c.C.Lstat(p)
		if err != nil {
			return st, mapErr(err)
		}
		if err := c.chmodWalk(p, fi, perm, o, true, &st, lim); err != nil {
			return st, err
		}
	}
	return st, nil
}

func (c Client) chmodWalk(p string, fi os.FileInfo, perm os.FileMode, o ChmodOpts, top bool, st *Stats, lim Limits) error {
	if fi.Mode()&os.ModeSymlink != 0 {
		st.Skipped++
		return nil
	}
	if st.Files+st.Dirs+st.Skipped >= lim.Entries {
		return ErrTooMany
	}
	if fi.IsDir() && o.Recursive {
		kids, err := c.C.ReadDir(p)
		if err != nil {
			return mapErr(err)
		}
		for _, k := range kids {
			if err := c.chmodWalk(path.Join(p, k.Name()), k, perm, o, false, st, lim); err != nil {
				return err
			}
		}
	}
	// Named items are always changed when the change is not recursive; in a recursive change the kinds chosen decide.
	if o.Recursive && ((fi.IsDir() && !o.Dirs) || (!fi.IsDir() && !o.Files)) {
		return nil
	}
	keep := fi.Mode() & (os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	// A change to what is already there is left out: any chmod, even to the same number, makes the system remove the
	// setgid bit when the account is not a member of the group of the folder, and that is the usual case for the folder
	// of a website (group www-data).
	if fi.Mode().Perm() != perm {
		if err := c.C.Chmod(p, perm|keep); err != nil {
			return mapErr(err)
		}
		if keep&os.ModeSetgid != 0 {
			if after, err := c.C.Lstat(p); err == nil && after.Mode()&os.ModeSetgid == 0 {
				st.Lost++
			}
		}
	}
	if fi.IsDir() {
		st.Dirs++
	} else {
		st.Files++
	}
	return nil
}
