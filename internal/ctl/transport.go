package ctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Response struct {
	OK     bool   `json:"ok"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Runner runs mpn with an argument list that Request.Argv has already checked.
type Runner interface {
	// Run starts mpn with the arguments and, as its standard input, whatever the request needs to keep off a command
	// line (a password); stdin is empty for most requests.
	Run(ctx context.Context, argv []string, stdin string) (string, error)
}

const (
	maxRequest  = 160 << 10 // a configuration of 48 KB can grow to twice that once it is JSON
	maxResponse = 2 << 20
	readTimeout = 10 * time.Second
)

// Server answers requests on a unix socket. Only a caller whose uid Allow accepts is served, and the uid comes from the
// kernel (SO_PEERCRED), so the caller cannot claim another. Operations that change the machine run one at a time:
// two of them at once could fight over the package manager, nginx or the mpn database.
type Server struct {
	Runner  Runner
	Allow   func(uid uint32) bool
	Peer    func(net.Conn) (uint32, error)
	Timeout time.Duration // longest a single operation may run
	// Log, when set, is told about every operation that ran: its arguments and whether it succeeded. Requests carry
	// no passwords, and the output is never passed on, so this is safe to write to the journal.
	Log func(argv []string, err error)

	mu sync.Mutex
}

// AllowUIDs returns an Allow function that accepts exactly the given uids.
func AllowUIDs(uids ...uint32) func(uint32) bool {
	return func(u uint32) bool {
		for _, x := range uids {
			if x == u {
				return true
			}
		}
		return false
	}
}

func (s *Server) Serve(l net.Listener) error {
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go s.handle(c)
	}
}

// readRequest reads one request of at most maxRequest bytes. A field it does not know is an error, so a caller cannot
// slip extra data past the checks in Argv.
func readRequest(r io.Reader) (Request, error) {
	var req Request
	dec := json.NewDecoder(io.LimitReader(r, maxRequest))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return Request{}, errors.New("bad request")
	}
	return req, nil
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	reply := func(r Response) {
		_ = c.SetWriteDeadline(time.Now().Add(readTimeout))
		_ = json.NewEncoder(c).Encode(r)
	}
	uid, err := s.Peer(c)
	if err != nil || !s.Allow(uid) {
		reply(Response{Error: "not allowed"})
		return
	}
	_ = c.SetReadDeadline(time.Now().Add(readTimeout))
	req, err := readRequest(c)
	if err != nil {
		reply(Response{Error: err.Error()})
		return
	}
	argv, err := req.Argv()
	if err != nil {
		reply(Response{Error: err.Error()})
		return
	}
	if IsWrite(req.Op) {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout)
	defer cancel()
	out, err := s.Runner.Run(ctx, argv, req.Stdin())
	if s.Log != nil {
		s.Log(argv, err)
	}
	if err != nil {
		reply(Response{Output: out, Error: err.Error()})
		return
	}
	reply(Response{OK: true, Output: out})
}

// Listen opens the socket for the helper. The file belongs to root and the given group and only they can use it.
// It replaces an old socket file but refuses to remove anything else.
func Listen(path, group string) (net.Listener, error) {
	g, err := user.LookupGroup(group)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("%s exists and is not a socket", path)
		}
		_ = os.Remove(path)
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chown(path, 0, gid); err != nil {
		l.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o660); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

// Call sends one request to the helper's socket and waits up to timeout for the answer.
func Call(socket string, r Request, timeout time.Duration) (Response, error) {
	c, err := net.DialTimeout("unix", socket, 5*time.Second)
	if err != nil {
		return Response{}, fmt.Errorf("the root helper is not reachable: %w", err)
	}
	defer c.Close()
	return CallConn(c, r, timeout)
}

func CallConn(c net.Conn, r Request, timeout time.Duration) (Response, error) {
	_ = c.SetDeadline(time.Now().Add(timeout))
	if err := json.NewEncoder(c).Encode(r); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.NewDecoder(io.LimitReader(c, maxResponse)).Decode(&resp); err != nil {
		return Response{}, fmt.Errorf("no valid answer from the root helper: %w", err)
	}
	return resp, nil
}

// ExecRunner runs the mpn binary with a clean environment and keeps at most Max bytes of what it prints.
type ExecRunner struct {
	Binary string
	Max    int
}

func (e ExecRunner) Run(ctx context.Context, argv []string, stdin string) (string, error) {
	cmd := exec.CommandContext(ctx, e.Binary, argv...)
	cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "HOME=/root", "LANG=C.UTF-8"}
	cmd.Stdin = strings.NewReader(stdin)
	w := &capWriter{max: e.Max}
	cmd.Stdout, cmd.Stderr = w, w
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	return w.String(), err
}

// capWriter keeps the first max bytes and swallows the rest, so a noisy command can neither fill memory nor block.
type capWriter struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (w *capWriter) Write(p []byte) (int, error) {
	if room := w.max - w.buf.Len(); room < len(p) {
		w.truncated = true
		if room > 0 {
			w.buf.Write(p[:room])
		}
	} else {
		w.buf.Write(p)
	}
	return len(p), nil
}

func (w *capWriter) String() string {
	if w.truncated {
		return w.buf.String() + "\n[output cut]\n"
	}
	return w.buf.String()
}
