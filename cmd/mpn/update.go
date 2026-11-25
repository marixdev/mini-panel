package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"time"

	"mpn/internal/update"
)

// insecureLocal is for asking the panel on 127.0.0.1 whether it answers: its certificate may be self-signed or for a name, and
// nothing is sent but a GET of the sign-in page.
func insecureLocal() *http.Transport {
	return &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} // #nosec G402 -- loopback health check only
}

// The program is replaced by "mpn update", which the web panel starts through the root helper (op panel.update). It downloads the
// release, checks it against the key built into this program (update.Check), keeps the program that runs as mpn.prev, puts the
// new one in place and runs "mpn panel migrate". When the panel does not answer afterwards, it puts mpn.prev back.

const (
	mpnPrev       = "/usr/local/bin/mpn.prev"
	mpnCloudFile  = "/usr/local/bin/mpn-cloud"
	updateUnit    = "mpn-update"
	updateLogPath = "/var/lib/mpn-web/update.log" // the panel shows it; owned by mpn-web
)

func runUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	want := fs.String("version", "", "the release to install, such as 1.2.0 (default: the latest)")
	check := fs.Bool("check", false, "only say whether a newer release exists")
	allowOlder := fs.Bool("allow-older", false, "allow a release older than the one that runs")
	apply := fs.Bool("apply", false, "replace the program (Linux, root)")
	detach := fs.Bool("detach", false, "run in the background as the systemd unit "+updateUnit+" (the web panel uses it)")
	_ = fs.Parse(args)

	current, err := update.ParseVersion(version)
	if err != nil && !*allowOlder {
		fail(fmt.Errorf("this program has no release number (%s); install a release with install.sh", version))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	tag := *want
	if tag == "" {
		tag, err = latestTag(ctx)
		fail(err)
	}
	target, err := update.ParseVersion(tag)
	fail(err)
	if *check {
		if current.Less(target) {
			fmt.Printf("%s is available (this is %s)\n", target, current)
		} else {
			fmt.Printf("%s is the latest; this is %s\n", target, current)
		}
		return
	}
	if !*apply {
		fmt.Printf("would install %s over %s, after checking its signature\n(dry run: add -apply to do it)\n", target, current)
		return
	}
	requireLinuxRoot()
	if *detach {
		// The web panel is restarted by the update, so the work must not run inside its request.
		a := []string{"--collect", "--quiet", "--unit=" + updateUnit, "--description=mpn: update", mpnPath, "update", "-apply", "-version", target.String()}
		if *allowOlder {
			a = append(a, "-allow-older")
		}
		if out, err := exec.Command("systemd-run", a...).CombinedOutput(); err != nil {
			if strings.Contains(string(out), "already") {
				fail(errors.New("update-busy: an update is already running"))
			}
			fail(fmt.Errorf("could not start the update: %v %s", err, strings.TrimSpace(string(out))))
		}
		fmt.Println("the update has started in the background")
		return
	}
	logf, _ := os.OpenFile(updateLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	say := func(format string, a ...any) {
		line := fmt.Sprintf(format, a...)
		fmt.Println(line)
		if logf != nil {
			fmt.Fprintln(logf, time.Now().Format("15:04:05"), line)
		}
	}
	if logf != nil {
		defer logf.Close()
		if u, err := user.Lookup("mpn-web"); err == nil {
			uid, _ := strconv.Atoi(u.Uid)
			gid, _ := strconv.Atoi(u.Gid)
			_ = os.Chown(updateLogPath, uid, gid)
		}
	}
	if err := installRelease(ctx, target, current, *allowOlder, say); err != nil {
		say("FAILED: %v", err)
		os.Exit(2)
	}
}

func installRelease(ctx context.Context, target, current update.Version, allowOlder bool, say func(string, ...any)) error {
	asset := "mpn_linux_" + runtime.GOARCH
	base := "https://github.com/" + update.Repo + "/releases/download/v" + target.String() + "/"
	say("downloading %s %s", target, asset)
	sums, err := fetch(ctx, base+"SHA256SUMS", 64<<10)
	if err != nil {
		return err
	}
	sig, err := fetch(ctx, base+"SHA256SUMS.minisig", 4<<10)
	if err != nil {
		return fmt.Errorf("the release has no signature (%v); releases before 1.2.0 are not signed, install them with install.sh", err)
	}
	tmp := mpnPath + ".new"
	v, err := fetchChecked(ctx, base, asset, tmp, sums, sig, current, allowOlder)
	if err != nil {
		return err
	}
	if v != target {
		_ = os.Remove(tmp)
		return fmt.Errorf("the signature is for %s, not %s", v, target)
	}
	if out, err := exec.Command(tmp, "version").Output(); err != nil || strings.TrimSpace(string(out)) != v.String() {
		_ = os.Remove(tmp)
		return fmt.Errorf("the new program does not run here (%v %q)", err, strings.TrimSpace(string(out)))
	}
	// From 1.2.0 the copy to off-site storage is a program of its own, mpn-cloud, built with rclone. A release without it (none
	// is signed before 1.2.0) leaves the one in place alone.
	cloudAsset := "mpn-cloud_linux_" + runtime.GOARCH
	cloudTmp := mpnCloudFile + ".new"
	hasCloud := strings.Contains(string(sums), " "+cloudAsset+"\n") || strings.Contains(string(sums), "*"+cloudAsset+"\n")
	if hasCloud {
		say("downloading %s", cloudAsset)
		if _, err := fetchChecked(ctx, base, cloudAsset, cloudTmp, sums, sig, current, allowOlder); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		if out, err := exec.Command(cloudTmp, "version").Output(); err != nil || strings.TrimSpace(string(out)) != v.String() {
			_ = os.Remove(tmp)
			_ = os.Remove(cloudTmp)
			return fmt.Errorf("the new %s does not run here (%v %q)", cloudAsset, err, strings.TrimSpace(string(out)))
		}
	}
	say("signature and checksum are good")
	if err := copyFile(mpnPath, mpnPrev); err != nil {
		_ = os.Remove(tmp)
		_ = os.Remove(cloudTmp)
		return fmt.Errorf("could not keep the old program: %w", err)
	}
	if err := os.Rename(tmp, mpnPath); err != nil {
		return err
	}
	if hasCloud {
		if err := os.Rename(cloudTmp, mpnCloudFile); err != nil {
			return err
		}
	}
	say("the program is %s; the old one is kept as %s", v, mpnPrev)

	restart := func() error {
		return exec.Command("systemctl", "restart", "mpn-root", "mpn-web").Run()
	}
	_ = restart()
	if out, err := exec.Command(mpnPath, "panel", "migrate", "-apply").CombinedOutput(); err != nil {
		say("mpn panel migrate: %s", strings.TrimSpace(string(out)))
		return rollBack(restart, say, fmt.Errorf("mpn panel migrate failed: %w", err))
	}
	if err := panelAnswers(); err != nil {
		return rollBack(restart, say, err)
	}
	say("done: the panel runs %s", v)
	return nil
}

// rollBack puts the previous program back after a failed update and restarts the panel with it.
func rollBack(restart func() error, say func(string, ...any), cause error) error {
	say("putting the previous program back")
	if err := copyFile(mpnPrev, mpnPath+".new"); err == nil {
		_ = os.Chmod(mpnPath+".new", 0o755)
		_ = os.Rename(mpnPath+".new", mpnPath)
	}
	_ = restart()
	return fmt.Errorf("%w; the previous program runs again", cause)
}

// panelAnswers waits up to 30 seconds for the panel to answer through nginx.
func panelAnswers() error {
	port := panelPortOf()
	if port == 0 {
		port = 2610
	}
	// Before the admin account exists, /login sends the browser to /setup, which answers 403 without its one-time key. The
	// redirect is the panel answering, so it is not followed.
	c := &http.Client{Timeout: 3 * time.Second, Transport: insecureLocal(),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var last error
	for i := 0; i < 15; i++ {
		resp, err := c.Get(fmt.Sprintf("https://127.0.0.1:%d/login", port))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 || resp.StatusCode == http.StatusSeeOther {
				return nil
			}
			err = fmt.Errorf("the sign-in page answered %d", resp.StatusCode)
		}
		last = err
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("the panel does not answer after the update: %v", last)
}

func latestTag(ctx context.Context) (string, error) {
	v, err := update.LatestTag(ctx, update.DefaultClient())
	if err != nil {
		return "", err
	}
	return v.String(), nil
}

// ensureMpnCloud downloads mpn-cloud of this very release when it is missing or of another version. An mpn update of 1.2.0 or older
// knows only one program, then runs "mpn panel migrate" with the new one, which calls this; so does install.sh on an old release.
func ensureMpnCloud() error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil
	}
	if out, err := exec.Command(mpnCloudFile, "version").Output(); err == nil && strings.TrimSpace(string(out)) == version {
		return nil
	}
	v, err := update.ParseVersion(version)
	if err != nil {
		return nil // a build of my own: there is no release to take mpn-cloud from
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	base := "https://github.com/" + update.Repo + "/releases/download/v" + v.String() + "/"
	sums, err := fetch(ctx, base+"SHA256SUMS", 64<<10)
	if err != nil {
		return err
	}
	sig, err := fetch(ctx, base+"SHA256SUMS.minisig", 4<<10)
	if err != nil {
		return err
	}
	asset := "mpn-cloud_linux_" + runtime.GOARCH
	tmp := mpnCloudFile + ".new"
	got, err := fetchChecked(ctx, base, asset, tmp, sums, sig, v, false)
	if err != nil {
		return err
	}
	if got != v {
		_ = os.Remove(tmp)
		return fmt.Errorf("the signature of %s is for %s, not %s", asset, got, v)
	}
	if err := os.Rename(tmp, mpnCloudFile); err != nil {
		return err
	}
	fmt.Printf("installed %s %s\n", mpnCloudFile, v)
	return nil
}

// fetchChecked downloads one program of the release to path (0755), hashing it on the way, and checks it against the signed
// SHA256SUMS. A file that does not pass is removed.
func fetchChecked(ctx context.Context, base, asset, path string, sums, sig []byte, current update.Version, allowOlder bool) (update.Version, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", base+asset, nil)
	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		return update.Version{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return update.Version{}, fmt.Errorf("%s: %s", base+asset, resp.Status)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return update.Version{}, err
	}
	h := sha256.New()
	const max = 512 << 20
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, max+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > max {
		err = fmt.Errorf("%s is larger than %d bytes", asset, max)
	}
	if err != nil {
		_ = os.Remove(path)
		return update.Version{}, err
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	v, err := update.CheckSum(update.ReleaseKey, sums, sig, sum, asset, current, allowOlder)
	if err != nil {
		_ = os.Remove(path)
		return update.Version{}, err
	}
	return v, nil
}

func fetch(ctx context.Context, url string, max int64) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
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

func copyFile(from, to string) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	tmp := to + ".tmp"
	if err := os.WriteFile(tmp, b, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, to)
}
