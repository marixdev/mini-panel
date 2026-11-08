package gdrive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// stall is how long a transfer may go without a single byte before it is cut and tried again. A test makes it shorter.
var stall = 2 * time.Minute

// watch resets the timer each time bytes arrive, so a slow connection is fine and a dead one is noticed.
type watch struct {
	r     io.Reader
	timer *time.Timer
}

func (w *watch) Read(p []byte) (int, error) {
	n, err := w.r.Read(p)
	if n > 0 {
		w.timer.Reset(stall)
	}
	return n, err
}

// transferClient is the client for a download. It has no limit on the whole transfer, because a backup can take an hour; stall
// ends one that stops moving.
func (c *Client) transferClient() *http.Client {
	if c.HTTP == nil {
		return &http.Client{}
	}
	cp := *c.HTTP
	cp.Timeout = 0
	return &cp
}

// pull asks for the file from byte `from` and writes it into f at that place. It returns how many bytes f holds afterwards, and the
// status of the answer when there was one. When Google ignores the range and sends the whole file, f is emptied and the file is
// taken from the start.
func (c *Client) pull(ctx context.Context, id string, f *os.File, from int64, forceToken bool) (have int64, status int, err error) {
	tok, err := c.token(ctx, forceToken)
	if err != nil {
		return from, 0, err
	}
	actx, cancel := context.WithCancel(ctx)
	defer cancel()
	timer := time.AfterFunc(stall, cancel)
	defer timer.Stop()
	req, err := http.NewRequestWithContext(actx, "GET", c.EP.API+"/files/"+url.PathEscape(id)+"?alt=media", nil)
	if err != nil {
		return from, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if from > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", from))
	}
	resp, err := c.transferClient().Do(req)
	if err != nil {
		return from, 0, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusOK && from > 0:
		if err := f.Truncate(0); err != nil {
			return from, resp.StatusCode, err
		}
		from = 0
	case resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return from, resp.StatusCode, apiError(resp.StatusCode, b)
	}
	n, err := io.Copy(io.NewOffsetWriter(f, from), &watch{r: resp.Body, timer: timer})
	return from + n, resp.StatusCode, err
}

// maxFailures is how many tries in a row may move no byte before the download gives up.
const maxFailures = 8

// Download copies a file of Drive to path (mode 0600). size is the size Google reported. When the connection breaks the download goes
// on from the bytes it has, so a cut costs only what had not yet been written. A file that ends short counts as a break.
func (c *Client) Download(ctx context.Context, id, path string, size int64) error {
	if size < 0 {
		return errors.New("the size of the file is not known")
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if size == 0 {
		return nil
	}
	var have int64
	var failures int
	forced := false
	for have < size {
		if err := ctx.Err(); err != nil {
			return err
		}
		before := have
		var status int
		have, status, err = c.pull(ctx, id, f, have, forced)
		forced = false
		if have > size {
			return fmt.Errorf("Google Drive sent more than the %d bytes of the file", size)
		}
		if have > before {
			failures = 0
		}
		var ae *APIError
		switch {
		case errors.As(err, &ae) && ae.Status == http.StatusUnauthorized && status == http.StatusUnauthorized && failures == 0:
			forced = true // the access token was refused: the next try asks for a new one
			failures++
			continue
		case errors.As(err, &ae) && ae.Status != http.StatusTooManyRequests && ae.Status < 500:
			return err
		case errors.Is(err, ErrRevoked):
			return err
		case err == nil && have >= size:
			return nil
		}
		if failures++; failures > maxFailures {
			if err == nil {
				err = errors.New("the answer ended before the whole file arrived")
			}
			return fmt.Errorf("download of the file stopped after %d tries: %w", maxFailures, err)
		}
		c.sleep(backoff(failures))
	}
	return nil
}
