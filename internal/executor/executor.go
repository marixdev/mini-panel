// Package executor applies an installer.Plan to the machine it runs on.
package executor

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mpn/internal/installer"
)

// Runner performs the two kinds of action a plan contains.
type Runner interface {
	Run(args []string) error
	WriteFile(path string, content []byte, mode uint32) error
}

// Rollbacker is implemented by runners that can undo the files they wrote.
type Rollbacker interface{ Rollback() error }

// Apply runs the plan in order and stops at the first failure. If the runner can roll back, the files
// written during the whole run are restored. Commands that already ran (apt installs, for example) are not undone.
func Apply(p installer.Plan, r Runner, log io.Writer) error {
	for i, s := range p {
		fmt.Fprintf(log, "[%d/%d] %s\n", i+1, len(p), s.Name)
		for _, a := range s.Actions {
			var err error
			if a.Args != nil {
				fmt.Fprintf(log, "  run   %s\n", strings.Join(a.Args, " "))
				err = r.Run(a.Args)
			} else {
				fmt.Fprintf(log, "  write %s\n", a.Path)
				err = r.WriteFile(a.Path, []byte(a.Content), a.Mode)
			}
			if err == nil {
				continue
			}
			err = fmt.Errorf("step %q failed: %w", s.Name, err)
			if rb, ok := r.(Rollbacker); ok {
				if rerr := rb.Rollback(); rerr != nil {
					err = fmt.Errorf("%w (rollback also failed: %v)", err, rerr)
				} else {
					fmt.Fprintln(log, "  restored the files written during this run")
				}
			}
			return err
		}
	}
	return nil
}

// OS runs commands and writes files on the local machine, remembering what it overwrote.
type OS struct {
	Out  io.Writer
	undo []undo
}

type undo struct {
	path    string
	old     []byte
	mode    os.FileMode
	existed bool
}

func (o *OS) Run(args []string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout, cmd.Stderr = o.Out, o.Out
	return cmd.Run()
}

func (o *OS) WriteFile(path string, content []byte, mode uint32) error {
	u := undo{path: path}
	if fi, err := os.Stat(path); err == nil {
		old, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		u.old, u.mode, u.existed = old, fi.Mode().Perm(), true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := atomicWrite(path, content, os.FileMode(mode)); err != nil {
		return err
	}
	o.undo = append(o.undo, u)
	return nil
}

// Rollback restores every file written so far, newest first, and removes the ones that did not exist before.
func (o *OS) Rollback() error {
	var first error
	for i := len(o.undo) - 1; i >= 0; i-- {
		u := o.undo[i]
		var err error
		if u.existed {
			err = atomicWrite(u.path, u.old, u.mode)
		} else {
			err = os.Remove(u.path)
		}
		if err != nil && first == nil {
			first = err
		}
	}
	o.undo = nil
	return first
}

// atomicWrite writes to a temporary file in the same directory and renames it over the target.
func atomicWrite(path string, content []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mpn-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}
