package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newLog returns a log writing to a temp file, with its clock under the test's
// control.
func newLog(t *testing.T, retention time.Duration) (*Log, string, *time.Time) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "audit.log")
	log := New(true, path, retention)
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	log.now = func() time.Time { return now }
	return log, path, &now
}

func read(t *testing.T, path string) []Event {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var events []Event
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// The sweep keeps a line it cannot read; a test may well have
			// put one there on purpose.
			continue
		}
		events = append(events, event)
	}
	return events
}

// A disabled log is a nil one: every call is accepted and nothing is created.
func TestDisabledLogWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	log := New(false, path, time.Hour)

	log.Record(Event{Event: EventLogin, IP: "10.0.0.1"})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a disabled log created %s", path)
	}
}

// The file keeps its window: what is older than the retention is dropped, what
// is inside it stays, and a line that cannot be parsed is kept rather than
// silently deleted.
func TestSweepDropsWhatAgedOut(t *testing.T) {
	log, path, now := newLog(t, 24*time.Hour)

	old := Event{Event: EventLogin, IP: "10.0.0.1", Time: now.Add(-25 * time.Hour)}
	recent := Event{Event: EventChange, IP: "10.0.0.2", Time: now.Add(-1 * time.Hour)}
	log.Record(old)
	log.Record(recent)

	// A hand-written line the package cannot read: it is evidence all the
	// same, so the sweep leaves it alone.
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("not json\n"); err != nil {
		t.Fatal(err)
	}
	file.Close()

	// The sweep runs once an hour at most; move the clock past that and add
	// an event inside the window.
	*now = now.Add(2 * time.Hour)
	log.Record(Event{Event: EventLogout, IP: "10.0.0.3"})

	events := read(t, path)
	if len(events) != 2 {
		t.Fatalf("kept %d events, want 2: %+v", len(events), events)
	}
	for _, event := range events {
		if event.Time.Before(now.Add(-24 * time.Hour)) {
			t.Errorf("%s aged out but is still there (%s)", event.Event, event.Time)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "not json") {
		t.Error("the unreadable line was deleted with the old ones")
	}
}

// Between two sweeps nothing is read or rewritten, even if every event is old:
// the panel must not pay a file copy per login attempt.
func TestSweepRunsAtMostOnceAnHour(t *testing.T) {
	log, path, now := newLog(t, time.Hour)

	log.Record(Event{Event: EventLogin, Time: now.Add(-10 * time.Hour)})
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// An hour has not passed, so the second record appends without sweeping.
	*now = now.Add(30 * time.Minute)
	log.Record(Event{Event: EventLogout, Time: now.Add(-10 * time.Hour)})

	if events := read(t, path); len(events) == 0 {
		t.Fatal("the file was swept on every record")
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.ModTime().Before(before.ModTime()) {
		t.Error("the file was rewritten between sweeps")
	}
}

// The log is written 0600: it names the addresses that reached the panel.
func TestLogIsPrivate(t *testing.T) {
	log, path, _ := newLog(t, time.Hour)
	log.Record(Event{Event: EventLogin, IP: "10.0.0.1"})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %o, want 600", mode)
	}
}

// A retention of zero means the default: a caller that has nothing to say gets
// a day, not an eternity.
func TestZeroRetentionMeansADay(t *testing.T) {
	log := New(true, filepath.Join(t.TempDir(), "audit.log"), 0)
	if log.retention != DefaultRetention {
		t.Errorf("retention = %v, want %v", log.retention, DefaultRetention)
	}
}
