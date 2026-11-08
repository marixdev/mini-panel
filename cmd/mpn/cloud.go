package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mpn/internal/backup"
	"mpn/internal/cloudsync"
	"mpn/internal/gdrive"
	"mpn/internal/installer"
	"mpn/internal/site"
	"mpn/internal/store"
)

const (
	// cloudDropIn makes the nightly backup copy to the drive when it is done. A drop-in is used so that servers installed before this
	// existed get it without their own unit being rewritten. The "-" lets the backup count as done even when the copy fails.
	cloudDropIn     = "/etc/systemd/system/mpn-backup.service.d/cloud.conf"
	cloudDropInText = "[Service]\nExecStartPost=-" + mpnPath + " cloud sync -all -apply\n"
	mpnPath         = "/usr/local/bin/mpn"
	cloudUnit       = "mpn-cloud-sync"
	defaultKeep     = 7
)

func cloudUsage() {
	fmt.Fprintln(os.Stderr, "usage: mpn cloud <status|connect|finish|disconnect|enable|sync|list|fetch> [flags]   (add -h for the flags)")
	os.Exit(2)
}

// cloudStatus is the answer of "mpn cloud status -json".
type cloudStatus struct {
	Connected bool                  `json:"connected"`
	Email     string                `json:"email"`
	Pending   *cloudPending         `json:"pending"`
	Enabled   bool                  `json:"enabled"`
	State     *cloudsync.State      `json:"state"`
	Fetch     *cloudsync.FetchState `json:"fetch"`
}

type cloudPending struct {
	UserCode string `json:"user_code"`
	URL      string `json:"url"`
	Expires  int64  `json:"expires"`
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	return err == nil && p.Signal(syscall.Signal(0)) == nil
}

func cloudClient() (*gdrive.Client, cloudsync.Config) {
	cfg, err := cloudsync.LoadConfig(cloudsync.ConfigFile)
	if errors.Is(err, os.ErrNotExist) {
		fail(errors.New("cloud-not-connected: connect to Google Drive first"))
	}
	fail(err)
	return &gdrive.Client{EP: gdrive.Google, ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, RefreshToken: cfg.RefreshToken}, cfg
}

func readCloudState(st *store.Store, name string) *cloudsync.State {
	v, _ := st.Setting(cloudsync.StateKey(name), "")
	if v == "" {
		return nil
	}
	var s cloudsync.State
	if json.Unmarshal([]byte(v), &s) != nil {
		return nil
	}
	return &s
}

func runCloud(args []string) {
	if len(args) == 0 {
		cloudUsage()
	}
	sub := args[0]
	fs := flag.NewFlagSet("cloud "+sub, flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("site", "", "the website")
	all := fs.Bool("all", false, "sync: every website that has the copy turned on")
	on := fs.Bool("on", true, "enable: turn the copy of the site on (false turns it off)")
	keep := fs.Int("keep", defaultKeep, "sync: how many of the newest backups of a site stay on the drive")
	detach := fs.Bool("detach", false, "sync, fetch: start in the background and return at once")
	stamp := fs.String("backup", "", "fetch: the backup to bring back, such as 20261104-031500")
	asJSON := fs.Bool("json", false, "status: print JSON")
	apply := fs.Bool("apply", false, "change this machine and contact Google (Linux, root); otherwise only check")
	_ = fs.Parse(args[1:])
	st := openStore(*db)
	defer st.Close()

	switch sub {
	case "status":
		s := cloudStatus{}
		if cfg, err := cloudsync.LoadConfig(cloudsync.ConfigFile); err == nil {
			s.Connected, s.Email = true, cfg.Email
		}
		if p, err := cloudsync.LoadPending(cloudsync.PendingFile); err == nil && !p.Expired(time.Now()) {
			s.Pending = &cloudPending{UserCode: p.UserCode, URL: p.URL, Expires: p.Expires}
		}
		if *name != "" {
			if !site.ValidName(*name) {
				fail(fmt.Errorf("invalid site name %q", *name))
			}
			v, _ := st.Setting(cloudsync.SiteKey(*name), "0")
			s.Enabled, s.State, s.Fetch = v == "1", readCloudState(st, *name), effectiveFetch(st, *name)
		}
		if *asJSON {
			printJSON(s)
			return
		}
		fmt.Printf("connected: %v %s\nsite enabled: %v\n", s.Connected, s.Email, s.Enabled)
	case "connect":
		raw, err := io.ReadAll(io.LimitReader(os.Stdin, 4<<10))
		fail(err)
		var in struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		}
		fail(json.Unmarshal(raw, &in))
		in.ClientID, in.ClientSecret = strings.TrimSpace(in.ClientID), strings.TrimSpace(in.ClientSecret)
		fail(cloudsync.ValidClient(in.ClientID, in.ClientSecret))
		if !*apply {
			fmt.Println("# checked only. Add -apply to ask Google for a code.")
			return
		}
		requireLinuxRoot()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d, err := gdrive.StartDevice(ctx, &http.Client{Timeout: 20 * time.Second}, gdrive.Google, in.ClientID)
		fail(err)
		fail(cloudsync.SavePending(cloudsync.PendingFile, cloudsync.Pending{ClientID: in.ClientID, ClientSecret: in.ClientSecret, DeviceCode: d.DeviceCode,
			UserCode: d.UserCode, URL: d.VerificationURL, Expires: time.Now().Add(time.Duration(d.ExpiresIn) * time.Second).Unix()}))
		fmt.Printf("open %s and type the code %s\n", d.VerificationURL, d.UserCode)
	case "finish":
		if !*apply {
			fmt.Println("# checked only. Add -apply to ask Google whether access was allowed.")
			return
		}
		requireLinuxRoot()
		cloudFinish()
	case "disconnect":
		if !*apply {
			fmt.Println("# checked only. Add -apply to forget the connection. Files already on the drive stay there.")
			return
		}
		requireLinuxRoot()
		cloudDisconnect()
	case "enable":
		s, err := st.GetSite(*name)
		fail(err)
		if *on {
			if _, err := cloudsync.LoadConfig(cloudsync.ConfigFile); err != nil {
				fail(errors.New("cloud-not-connected: connect to Google Drive first"))
			}
		}
		if !*apply {
			fmt.Printf("would turn the copy of %s to Google Drive %v\n", s.Name, *on)
			return
		}
		fail(st.SetSetting(cloudsync.SiteKey(s.Name), map[bool]string{true: "1", false: "0"}[*on]))
		fmt.Printf("the copy of %s to Google Drive is %s\n", s.Name, map[bool]string{true: "on", false: "off"}[*on])
	case "sync":
		cloudSyncCmd(st, *name, *all, *keep, *detach, *apply)
	case "list":
		cloudListCmd(*name)
	case "fetch":
		cloudFetchCmd(st, *name, *stamp, *detach, *apply)
	default:
		cloudUsage()
	}
}

func cloudFinish() {
	p, err := cloudsync.LoadPending(cloudsync.PendingFile)
	if err != nil {
		fail(errors.New("cloud-expired: there is no connection waiting; start again"))
	}
	if p.Expired(time.Now()) {
		_ = os.Remove(cloudsync.PendingFile)
		fail(errors.New("cloud-expired: the code has expired; start again"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	hc := &http.Client{Timeout: 30 * time.Second}
	rt, err := gdrive.PollDevice(ctx, hc, gdrive.Google, p.ClientID, p.ClientSecret, p.DeviceCode)
	switch {
	case errors.Is(err, gdrive.ErrPending), errors.Is(err, gdrive.ErrSlowDown):
		fail(errors.New("cloud-pending: Google has not been told yet that you allowed access; open the address, type the code, then press the button again"))
	case errors.Is(err, gdrive.ErrDenied):
		_ = os.Remove(cloudsync.PendingFile)
		fail(errors.New("cloud-denied: access was refused at Google"))
	case errors.Is(err, gdrive.ErrExpired):
		_ = os.Remove(cloudsync.PendingFile)
		fail(errors.New("cloud-expired: the code has expired; start again"))
	}
	fail(err)
	cli := &gdrive.Client{HTTP: hc, EP: gdrive.Google, ClientID: p.ClientID, ClientSecret: p.ClientSecret, RefreshToken: rt}
	root, err := cli.EnsureFolder(ctx, "root", cloudsync.RootName)
	fail(err)
	cfg := cloudsync.Config{ClientID: p.ClientID, ClientSecret: p.ClientSecret, RefreshToken: rt, RootID: root, Email: cli.Email(ctx)}
	fail(cloudsync.SaveConfig(cloudsync.ConfigFile, cfg))
	_ = os.Remove(cloudsync.PendingFile)
	fail(runPlanQuiet(installer.Plan{{Name: "copy after the nightly backup", Actions: []installer.Action{
		installer.Run("install", "-d", "-m", "0755", "/etc/systemd/system/mpn-backup.service.d"),
		installer.WriteFile(cloudDropIn, cloudDropInText, 0o644),
		installer.Run("systemctl", "daemon-reload"),
	}}}))
	fmt.Printf("connected to Google Drive as %s; the backups go to the folder %s\n", cfg.Email, cloudsync.RootName)
}

func cloudDisconnect() {
	if cfg, err := cloudsync.LoadConfig(cloudsync.ConfigFile); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/revoke", strings.NewReader(url.Values{"token": {cfg.RefreshToken}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if resp, err := http.DefaultClient.Do(req); err == nil { // the permission is taken back at Google as well; failing to is no reason to keep ours
			resp.Body.Close()
		}
	}
	fail(runPlanQuiet(installer.Plan{{Name: "forget the connection", Actions: []installer.Action{
		installer.Run("rm", "-f", cloudsync.ConfigFile, cloudsync.PendingFile, cloudDropIn),
		installer.Run("systemctl", "daemon-reload"),
	}}}))
	fmt.Println("disconnected from Google Drive; files already on the drive stay there")
}

// cloudSyncCmd copies the backups of one site, or of every site that has the copy on.
func cloudSyncCmd(st *store.Store, name string, all bool, keep int, detach, apply bool) {
	if all == (name != "") {
		fail(errors.New("give either -site NAME or -all"))
	}
	if keep < 1 || keep > 365 {
		fail(errors.New("-keep must be between 1 and 365"))
	}
	var sites []site.Site
	if all {
		list, err := st.ListSites()
		fail(err)
		for _, s := range list {
			if v, _ := st.Setting(cloudsync.SiteKey(s.Name), "0"); v == "1" {
				sites = append(sites, s)
			}
		}
	} else {
		s, err := st.GetSite(name)
		fail(err)
		sites = []site.Site{s}
	}
	if !apply {
		fmt.Printf("would copy the newest %d backup(s) of %d site(s) to Google Drive\n", keep, len(sites))
		return
	}
	requireLinuxRoot()
	if detach {
		args := []string{"--collect", "--quiet", "--unit=" + cloudUnit, "--description=mpn: copy backups to Google Drive", mpnPath, "cloud", "sync", "-keep", strconv.Itoa(keep), "-apply"}
		if all {
			args = append(args, "-all")
		} else {
			args = append(args, "-site", name)
		}
		if out, err := exec.Command("systemd-run", args...).CombinedOutput(); err != nil {
			if strings.Contains(string(out), "already") || strings.Contains(string(out), "exists") {
				fail(errors.New("cloud-busy: a copy is already running; wait for it to finish"))
			}
			fail(fmt.Errorf("could not start the copy: %v %s", err, strings.TrimSpace(string(out))))
		}
		fmt.Println("the copy has started in the background")
		return
	}
	if len(sites) == 0 {
		fmt.Println("no website has the copy to Google Drive turned on")
		return
	}
	lock, err := cloudsync.TakeLock(cloudsync.LockFile, processAlive)
	if errors.Is(err, cloudsync.ErrBusy) {
		fail(errors.New("cloud-busy: a copy is already running; wait for it to finish"))
	}
	fail(err)
	defer lock.Release()
	cli, cfg := cloudClient()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Hour)
	defer cancel()
	failed := 0
	for _, s := range sites {
		complete, _ := listStamps(s.Name)
		res, err := cloudsync.SyncSite(ctx, cli, cfg.RootID, s.Name, complete, func(stamp string) string { return backup.Dir(s.Name, stamp) }, keep, os.Stdout)
		state := cloudsync.State{At: time.Now().Unix(), OK: err == nil, Uploaded: len(res.Uploaded), Present: res.Present}
		if err != nil {
			state.Message = err.Error()
			if errors.Is(err, gdrive.ErrRevoked) {
				state.Message = "cloud-revoked: " + err.Error()
			}
			fmt.Fprintf(os.Stderr, "site %s: %v\n", s.Name, err)
			failed++
		} else {
			fmt.Printf("site %s: %d new, %d on the drive\n", s.Name, len(res.Uploaded), res.Present)
		}
		b, _ := json.Marshal(state)
		_ = st.SetSetting(cloudsync.StateKey(s.Name), string(b))
	}
	if failed > 0 {
		os.Exit(1)
	}
}
