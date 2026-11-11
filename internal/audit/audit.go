// Package audit appends the events an operator wants to find after the fact -
// who logged in, from where, and what was changed - to a file on the config
// volume, and keeps that file to a window of time.
//
// It is off unless WEB_UI_AUDIT_LOG is set: the log names the addresses that
// reached the panel, and keeping that record is the operator's decision, not
// the image's. Nothing that is not a login, a logout or a change through the
// API is written, and request bodies never are, so no key and no .conf can
// end up here.
package audit

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"amneziawg-web-ui/internal/atomicfile"
)

// sweepInterval is how often the file is looked at for entries that have
// aged out. Between two sweeps the file grows by whatever happened in that
// hour - a handful of lines on a panel - so the read is small and the panel
// never pays for it during a request.
const sweepInterval = time.Hour

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
	// retention is how long an entry stays in the file: entries older than
	// this are dropped the next time the file is swept.
	retention time.Duration
	now       func() time.Time

	// mu serialises the appends and the sweeps. O_APPEND already keeps two
	// writes from interleaving on Linux, but the encoder is stateful and
	// shared, and a sweep rewrites the whole file.
	mu sync.Mutex
	// lastSweep is when the file was last looked at, so a busy panel does
	// not read and rewrite it on every event.
	lastSweep time.Time
}

// New returns the log for path, or nil - auditing off - when enabled is false.
// retention is the window the file is kept to; a zero one means the default.
// A file that cannot be opened is reported and then written per event, so a
// broken volume degrades to a line in the container log rather than taking the
// panel down.
func New(enabled bool, path string, retention time.Duration) *Log {
	if !enabled {
		return nil
	}
	if retention <= 0 {
		retention = DefaultRetention
	}
	return &Log{path: path, retention: retention, now: time.Now}
}

// Retention bounds. The default is a day of events - enough to answer "who
// changed this last night" - and a month is the ceiling, because a log of
// addresses that never ends is a liability rather than a record.
const (
	DefaultRetention = 24 * time.Hour
	MaxRetention     = 720 * time.Hour // 30 days
)

// Record stamps the event and appends it, and sweeps the file when enough
// time has passed since the last sweep.
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

	l.append(line)
	l.sweep(e.Time)
}

// append writes one line at the end of the file.
func (l *Log) append(line []byte) {
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

// sweep drops the entries older than the retention window. It runs at most
// once per sweepInterval, and it rewrites the file only when something has
// actually aged out - a no-op sweep leaves the file exactly as it was.
//
// A line that will not parse is kept: a record this package cannot read is
// still a record, and silently deleting evidence is worse than carrying a
// line that never expires.
func (l *Log) sweep(now time.Time) {
	if !l.lastSweep.IsZero() && now.Sub(l.lastSweep) < sweepInterval {
		return
	}
	l.lastSweep = now

	file, err := os.Open(l.path)
	if err != nil {
		return // nothing written yet
	}
	data, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		log.Printf("audit: %v", err)
		return
	}

	cutoff := now.Add(-l.retention)
	kept := make([][]byte, 0, 32)
	dropped := 0
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err == nil && !event.Time.IsZero() && event.Time.Before(cutoff) {
			dropped++
			continue
		}
		kept = append(kept, line)
	}
	if dropped == 0 {
		return
	}

	out := bytes.Join(kept, []byte("\n"))
	if len(out) > 0 {
		out = append(out, '\n')
	}
	// Atomic: a reader - or the next start of the container - sees the old
	// file or the swept one, never half of each.
	if err := atomicfile.Write(l.path, out, 0o600); err != nil {
		log.Printf("audit: %v", err)
	}
}
