// Package audit appends the events an operator wants to find after the fact -
// who logged in, from where, and what was changed - to a file on the config
// volume.
//
// It is off unless WEB_UI_AUDIT_LOG is set: the log names the addresses that
// reached the panel, and keeping that record is the operator's decision, not
// the image's. Nothing that is not a login, a logout or a change through the
// API is written, and request bodies never are, so no key and no .conf can
// end up here.
package audit

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

// Event is one line of the log.
type Event struct {
	Time   time.Time `json:"time"`
	Event  string    `json:"event"`
	User   string    `json:"user,omitempty"`
	IP     string    `json:"ip,omitempty"`
	Method string    `json:"method,omitempty"`
	// Path is the route, never the query string: the query is where a
	// token or a config would travel if one ever did.
	Path   string `json:"path,omitempty"`
	Status int    `json:"status,omitempty"`
}

// The event names. Kept short and stable: they are what a log query matches.
const (
	EventLogin        = "login"
	EventLoginFailed  = "login-failed"
	EventLogout       = "logout"
	EventCSRFRejected = "csrf-rejected"
	EventChange       = "change"
)

// Log writes events to path when it is enabled. A nil *Log accepts every
// Record and does nothing, so callers never branch on whether auditing is on.
type Log struct {
	path string
	now  func() time.Time

	// mu serialises the appends. O_APPEND already keeps two writes from
	// interleaving on Linux, but the encoder is stateful and shared.
	mu sync.Mutex
}

// New returns the log for path, or nil - audting off - when enabled is false.
// A file that cannot be opened is reported once and then written per event,
// so a broken volume degrades to a line in the container log rather than
// taking the panel down.
func New(enabled bool, path string) *Log {
	if !enabled {
		return nil
	}
	return &Log{path: path, now: time.Now}
}

// Record stamps the event and appends it.
func (l *Log) Record(e Event) {
	if l == nil {
		return
	}
	if e.Time.IsZero() {
		e.Time = l.now()
	}

	line, err := json.Marshal(e)
	if err != nil {
		log.Printf("audit: %v", err)
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	file, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("audit: %v", err)
		return
	}
	defer file.Close()

	if _, err := file.Write(append(line, '\n')); err != nil {
		log.Printf("audit: %v", err)
	}
}
