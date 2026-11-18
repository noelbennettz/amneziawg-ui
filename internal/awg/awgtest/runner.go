// Package awgtest is a Runner for tests: it records every command and answers
// from a table of stubs, so a test can say what the host would have said
// without a kernel or amneziawg-tools present.
package awgtest

import (
	"errors"
	"strings"
	"sync"
)

// ErrNotInstalled is what an unstubbed command fails with, standing in for
// "command not found".
var ErrNotInstalled = errors.New("awgtest: command not stubbed")

// Runner answers commands by longest matching prefix.
type Runner struct {
	mu       sync.Mutex
	stubs    map[string]stub
	Commands []string
}

type stub struct {
	out string
	err error
}

// New returns a Runner that fails every command until stubbed.
func New() *Runner {
	return &Runner{stubs: map[string]stub{}}
}

// Stub makes every command starting with prefix succeed with out.
func (r *Runner) Stub(prefix, out string) *Runner {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stubs[prefix] = stub{out: out}
	return r
}

// Fail makes every command starting with prefix fail with err.
func (r *Runner) Fail(prefix string, err error) *Runner {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stubs[prefix] = stub{err: err}
	return r
}

// Unstub forgets a stub, so the command fails again.
func (r *Runner) Unstub(prefix string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.stubs, prefix)
}

// Run implements awg.Runner.
func (r *Runner) Run(command string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Commands = append(r.Commands, command)

	best, found := "", false
	for prefix := range r.stubs {
		if strings.HasPrefix(command, prefix) && len(prefix) >= len(best) {
			best, found = prefix, true
		}
	}
	if !found {
		return "", ErrNotInstalled
	}
	s := r.stubs[best]
	return s.out, s.err
}

// Ran reports whether some recorded command starts with prefix.
func (r *Runner) Ran(prefix string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.Commands {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}
