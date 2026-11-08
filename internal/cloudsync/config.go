package cloudsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"mpn/internal/site"
)

// Paths of the files root keeps. They hold the client secret and the permission Google gave, so only root can read them.
const (
	ConfigFile  = "/etc/mpn/gdrive.json"
	PendingFile = "/etc/mpn/gdrive-pending.json"
	LockFile    = "/run/mpn/cloud-sync.lock"
)

// Config is the connection to Google Drive.
type Config struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	Email        string `json:"email,omitempty"`
	RootID       string `json:"root_id"` // the folder mpn-backups
}

// Pending is a connection that waits for the person to allow access at Google.
type Pending struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	DeviceCode   string `json:"device_code"`
	UserCode     string `json:"user_code"`
	URL          string `json:"url"`
	Expires      int64  `json:"expires"`
}

func (p Pending) Expired(now time.Time) bool { return now.Unix() >= p.Expires }

var (
	clientIDRe     = regexp.MustCompile(`^[A-Za-z0-9._-]{10,200}$`)
	clientSecretRe = regexp.MustCompile(`^[A-Za-z0-9._~+/=-]{8,200}$`)
)

// ValidClient checks what the person typed. These end up in a request to Google and in a file, never in a command line.
func ValidClient(id, secret string) error {
	id, secret = strings.TrimSpace(id), strings.TrimSpace(secret)
	switch {
	case !clientIDRe.MatchString(id):
		return errors.New("the client ID looks wrong; copy it whole from the Google Cloud Console (it ends in .apps.googleusercontent.com)")
	case !clientSecretRe.MatchString(secret):
		return errors.New("the client secret looks wrong; copy it whole from the Google Cloud Console")
	}
	return nil
}

func writeJSON(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gdrive-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// LoadConfig returns the connection, or os.ErrNotExist when there is none.
func LoadConfig(path string) (Config, error) {
	var c Config
	if err := readJSON(path, &c); err != nil {
		return Config{}, err
	}
	if c.ClientID == "" || c.RefreshToken == "" || c.RootID == "" {
		return Config{}, fmt.Errorf("%s is incomplete: connect to Google Drive again", path)
	}
	return c, nil
}

func SaveConfig(path string, c Config) error { return writeJSON(path, c) }

func LoadPending(path string) (Pending, error) {
	var p Pending
	err := readJSON(path, &p)
	return p, err
}

func SavePending(path string, p Pending) error { return writeJSON(path, p) }

// SiteSync is the setting that says a site is copied to the drive.
func SiteKey(siteName string) string { return "gdrive_sync_" + siteName }

// Used reports whether a settings key belongs to a valid site; a key is never built from anything but a checked name.
func ValidSiteForKey(name string) bool { return site.ValidName(name) }

// State is how the last sync of a site ended; the panel shows it.
type State struct {
	At       int64  `json:"at"`
	OK       bool   `json:"ok"`
	Message  string `json:"message,omitempty"`
	Uploaded int    `json:"uploaded"`
	Present  int    `json:"present"`
}

func StateKey(siteName string) string { return "gdrive_state_" + siteName }

// Lock is a file that says a sync is running. A lock whose process is gone is taken over.
type Lock struct{ path string }

// TakeLock returns ErrBusy when another sync runs.
func TakeLock(path string, alive func(pid int) bool) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	for try := 0; try < 2; try++ {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			f.Close()
			return &Lock{path}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		b, _ := os.ReadFile(path)
		var pid int
		fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pid)
		if pid > 0 && alive(pid) {
			return nil, ErrBusy
		}
		_ = os.Remove(path)
	}
	return nil, ErrBusy
}

// ErrBusy means another sync is running.
var ErrBusy = errors.New("another sync is running")

func (l *Lock) Release() { _ = os.Remove(l.path) }
