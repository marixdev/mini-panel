package cloudsync

import (
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"mpn/internal/backup"
	"mpn/internal/dbadmin"
	"mpn/internal/gdrive"
	"mpn/internal/site"
)

// RemoteBackup is a finished copy on the drive.
type RemoteBackup struct {
	Stamp string `json:"stamp"`
	Bytes int64  `json:"bytes"`
}

// remoteSiteFolder returns the id of the folder of a site on the drive, or "" when the drive has none. It makes nothing.
func remoteSiteFolder(ctx context.Context, d Drive, rootID, siteName string) (string, error) {
	top, err := d.List(ctx, rootID)
	if err != nil {
		return "", err
	}
	for _, f := range top {
		if f.IsFolder() && f.Name == siteName {
			return f.ID, nil
		}
	}
	return "", nil
}

// backupFiles keeps the files of a folder that belong to a backup. A name that is not one of those is never downloaded, so a stranger's
// file in the folder cannot end up in /var/backups.
func backupFiles(in []gdrive.File) map[string]gdrive.File {
	out := map[string]gdrive.File{}
	for _, x := range in {
		if !x.IsFolder() && fileRe.MatchString(x.Name) {
			if _, dup := out[x.Name]; !dup {
				out[x.Name] = x
			}
		}
	}
	return out
}

// ListRemoteBackups returns the finished copies of a site on the drive, newest first. A copy without MANIFEST was cut off and is left out.
func ListRemoteBackups(ctx context.Context, d Drive, rootID, siteName string) ([]RemoteBackup, error) {
	if !site.ValidName(siteName) {
		return nil, fmt.Errorf("invalid site name %q", siteName)
	}
	folder, err := remoteSiteFolder(ctx, d, rootID, siteName)
	if err != nil || folder == "" {
		return nil, err
	}
	stamps, err := d.List(ctx, folder)
	if err != nil {
		return nil, err
	}
	var out []RemoteBackup
	for _, s := range stamps {
		if !s.IsFolder() || !backup.ValidStamp(s.Name) {
			continue
		}
		inside, err := d.List(ctx, s.ID)
		if err != nil {
			return nil, err
		}
		files := backupFiles(inside)
		if _, done := files["MANIFEST"]; !done {
			continue
		}
		var n int64
		for _, f := range files {
			n += f.Size
		}
		out = append(out, RemoteBackup{Stamp: s.Name, Bytes: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Stamp > out[j].Stamp })
	return out, nil
}

func md5Of(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// gzipOK reads a .gz file to its end, the check "gzip -t" makes: a cut-off or damaged file fails.
func gzipOK(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	_, err = io.Copy(io.Discard, r)
	return err
}

func download(ctx context.Context, d Drive, f gdrive.File, dir string, log io.Writer) error {
	p := filepath.Join(dir, f.Name)
	fmt.Fprintf(log, "downloading %s (%.1f MB)\n", f.Name, float64(f.Size)/(1<<20))
	if err := d.Download(ctx, f.ID, p, f.Size); err != nil {
		return fmt.Errorf("%s: %w", f.Name, err)
	}
	if f.MD5 != "" {
		got, err := md5Of(p)
		if err != nil {
			return err
		}
		if got != f.MD5 {
			return fmt.Errorf("%s arrived damaged: its checksum is %s, Google has %s", f.Name, got, f.MD5)
		}
	}
	if filepath.Ext(f.Name) == ".gz" {
		if err := gzipOK(p); err != nil {
			return fmt.Errorf("%s is not a whole gzip file: %w", f.Name, err)
		}
	}
	return nil
}

// FetchBackup copies one finished backup of a site from the drive into siteDir/<stamp>, where the usual restore finds it. It is
// downloaded into a folder of a different name and renamed at the end, so the backup appears whole or not at all. Everything that
// comes from the drive is checked before it counts: the MANIFEST is read first and must be for this site and stamp; every file is
// compared with the checksum Google holds and every .gz is read to its end. freeBytes is the free space of the disk, -1 when unknown.
func FetchBackup(ctx context.Context, d Drive, rootID, siteName, stamp, siteDir string, freeBytes int64, log io.Writer) error {
	if !site.ValidName(siteName) {
		return fmt.Errorf("invalid site name %q", siteName)
	}
	if !backup.ValidStamp(stamp) {
		return fmt.Errorf("invalid backup name %q", stamp)
	}
	final := filepath.Join(siteDir, stamp)
	if _, err := os.Stat(final); err == nil {
		return fmt.Errorf("cloud-exists: the backup %s is on this server already", stamp)
	}
	siteFolder, err := remoteSiteFolder(ctx, d, rootID, siteName)
	if err != nil {
		return err
	}
	var folder string
	if siteFolder != "" {
		stamps, err := d.List(ctx, siteFolder)
		if err != nil {
			return err
		}
		for _, s := range stamps {
			if s.IsFolder() && s.Name == stamp {
				folder = s.ID
			}
		}
	}
	missing := fmt.Errorf("cloud-nocopy: the drive has no finished copy of %s/%s", siteName, stamp)
	if folder == "" {
		return missing
	}
	inside, err := d.List(ctx, folder)
	if err != nil {
		return err
	}
	files := backupFiles(inside)
	manifest, ok := files["MANIFEST"]
	if !ok {
		return missing
	}
	if err := os.MkdirAll(siteDir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(siteDir, ".dl-"+stamp)
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.Mkdir(tmp, 0o700); err != nil {
		return err
	}
	done := false
	defer func() {
		if !done {
			_ = os.RemoveAll(tmp)
		}
	}()

	if err := download(ctx, d, manifest, tmp, log); err != nil {
		return err
	}
	text, err := os.ReadFile(filepath.Join(tmp, "MANIFEST"))
	if err != nil {
		return err
	}
	m, err := backup.ParseManifest(string(text))
	if err != nil {
		return fmt.Errorf("the MANIFEST on the drive is not valid: %w", err)
	}
	if m.Site != siteName || m.Stamp != stamp {
		return fmt.Errorf("the MANIFEST on the drive is for %s/%s, not %s/%s", m.Site, m.Stamp, siteName, stamp)
	}
	var want []string
	if m.Files {
		want = append(want, "files.tar.gz")
	}
	for _, db := range m.DBs {
		if !dbadmin.OwnedBy(db, siteName) {
			return fmt.Errorf("the MANIFEST on the drive names the database %q, which is not a database of the site %q", db, siteName)
		}
		want = append(want, db+".sql.gz")
	}
	var total int64
	for _, n := range want {
		f, ok := files[n]
		if !ok {
			return fmt.Errorf("cloud-nocopy: the copy on the drive lacks %s, which its MANIFEST lists", n)
		}
		total += f.Size
	}
	if need := total + total/10 + 64<<20; freeBytes >= 0 && need > freeBytes {
		return fmt.Errorf("cloud-nospace: the backup needs about %d MB and this server has %d MB free", need>>20, freeBytes>>20)
	}
	for _, n := range want {
		if err := download(ctx, d, files[n], tmp, log); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, final); err != nil {
		return err
	}
	done = true
	return nil
}
