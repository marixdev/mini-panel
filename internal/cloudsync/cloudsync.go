// Package cloudsync copies the backups of a site to a cloud drive and keeps the copies tidy. It knows nothing about Google: it works
// with the small Drive interface, so it can be tested with a drive in memory.
//
// On the drive a backup is a folder <root>/<site>/<stamp>/ holding the same files as /var/backups/mpn/<site>/<stamp>/. MANIFEST is
// uploaded last, so a folder without it is a copy that was cut off, and is made again.
package cloudsync

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"

	"mpn/internal/backup"
	"mpn/internal/gdrive"
	"mpn/internal/site"
)

// RootName is the folder in the drive that holds everything mpn uploads.
const RootName = "mpn-backups"

// Drive is what the sync needs from a cloud drive.
type Drive interface {
	EnsureFolder(ctx context.Context, parent, name string) (string, error)
	List(ctx context.Context, parent string) ([]gdrive.File, error)
	Upload(ctx context.Context, parent, name, path string) (string, error)
	Delete(ctx context.Context, id string) error
	Download(ctx context.Context, id, path string, size int64) error
}

// fileRe names the files of a backup that are uploaded; anything else in the folder stays where it is.
var fileRe = regexp.MustCompile(`^(files\.tar\.gz|[A-Za-z0-9_]{1,64}\.sql\.gz|MANIFEST)$`)

// Result says what a sync did.
type Result struct {
	Uploaded []string // stamps copied now
	Deleted  []string // stamps removed from the drive
	Present  int      // finished copies on the drive afterwards
}

// Stamps lists the finished and the unfinished copies on the drive, as folder ids by stamp.
type remote struct{ complete, partial map[string]string }

func listRemote(ctx context.Context, d Drive, siteFolder string) (remote, error) {
	r := remote{complete: map[string]string{}, partial: map[string]string{}}
	folders, err := d.List(ctx, siteFolder)
	if err != nil {
		return r, err
	}
	for _, f := range folders {
		if !f.IsFolder() || !backup.ValidStamp(f.Name) {
			continue
		}
		inside, err := d.List(ctx, f.ID)
		if err != nil {
			return r, err
		}
		done := false
		for _, x := range inside {
			if x.Name == "MANIFEST" && !x.IsFolder() {
				done = true
			}
		}
		if done {
			r.complete[f.Name] = f.ID
		} else {
			r.partial[f.Name] = f.ID
		}
	}
	return r, nil
}

// filesOf returns the files of a backup in the order to upload them: MANIFEST last.
func filesOf(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	hasManifest := false
	for _, e := range entries {
		if !e.Type().IsRegular() || !fileRe.MatchString(e.Name()) {
			continue
		}
		if e.Name() == "MANIFEST" {
			hasManifest = true
			continue
		}
		names = append(names, e.Name())
	}
	if !hasManifest {
		return nil, fmt.Errorf("%s has no MANIFEST: the backup is not finished", dir)
	}
	sort.Strings(names)
	return append(names, "MANIFEST"), nil
}

// SyncSite makes the drive hold the newest `keep` finished backups of the site. local is the list of finished local stamps and dirOf
// the folder of one of them. The newest are uploaded first, so the most useful copy is on the drive before the others. A failure
// stops at the first problem: what was uploaded stays, and the next sync goes on from there.
func SyncSite(ctx context.Context, d Drive, rootID, siteName string, local []string, dirOf func(stamp string) string, keep int, log io.Writer) (Result, error) {
	var res Result
	if !site.ValidName(siteName) {
		return res, fmt.Errorf("invalid site name %q", siteName)
	}
	if keep < 1 {
		return res, fmt.Errorf("keep at least one backup")
	}
	siteFolder, err := d.EnsureFolder(ctx, rootID, siteName)
	if err != nil {
		return res, err
	}
	rem, err := listRemote(ctx, d, siteFolder)
	if err != nil {
		return res, err
	}
	// The newest `keep` local backups are the ones worth having on the drive.
	want := append([]string(nil), local...)
	sort.Sort(sort.Reverse(sort.StringSlice(want)))
	if len(want) > keep {
		want = want[:keep]
	}
	for _, stamp := range want {
		if !backup.ValidStamp(stamp) {
			continue
		}
		if _, ok := rem.complete[stamp]; ok {
			continue
		}
		if id, ok := rem.partial[stamp]; ok {
			fmt.Fprintf(log, "%s: the copy of %s on the drive was cut off, making it again\n", siteName, stamp)
			if err := d.Delete(ctx, id); err != nil {
				return res, err
			}
			delete(rem.partial, stamp)
		}
		names, err := filesOf(dirOf(stamp))
		if err != nil {
			return res, err
		}
		folder, err := d.EnsureFolder(ctx, siteFolder, stamp)
		if err != nil {
			return res, err
		}
		for _, n := range names {
			fmt.Fprintf(log, "%s/%s: uploading %s\n", siteName, stamp, n)
			if _, err := d.Upload(ctx, folder, n, dirOf(stamp)+"/"+n); err != nil {
				return res, fmt.Errorf("%s/%s/%s: %w", siteName, stamp, n, err)
			}
		}
		rem.complete[stamp] = folder
		res.Uploaded = append(res.Uploaded, stamp)
	}
	// Tidy up: only folders named like a backup are ever touched.
	var done []string
	for s := range rem.complete {
		done = append(done, s)
	}
	for _, stamp := range backup.Expired(done, keep) {
		fmt.Fprintf(log, "%s: removing the old copy %s from the drive\n", siteName, stamp)
		if err := d.Delete(ctx, rem.complete[stamp]); err != nil {
			return res, err
		}
		delete(rem.complete, stamp)
		res.Deleted = append(res.Deleted, stamp)
	}
	newest := ""
	for s := range rem.complete {
		if s > newest {
			newest = s
		}
	}
	for stamp, id := range rem.partial { // a cut-off copy older than the newest finished one will never be finished
		if stamp < newest {
			if err := d.Delete(ctx, id); err != nil {
				return res, err
			}
			res.Deleted = append(res.Deleted, stamp)
		}
	}
	sort.Strings(res.Uploaded)
	sort.Strings(res.Deleted)
	res.Present = len(rem.complete)
	return res, nil
}
