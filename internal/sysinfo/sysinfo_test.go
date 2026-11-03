package sysinfo

import (
	"path/filepath"
	"testing"
)

// The readings come from the machine the test runs on, so what can be
// checked is their shape: a core count, times that add up, a used figure
// that fits its capacity.
func TestMetricsAreConsistent(t *testing.T) {
	m := Default(t.TempDir()).Metrics()

	if m.CPU.Cores == 0 {
		t.Error("no cores")
	}
	if m.CPU.TotalSeconds <= 0 || m.CPU.BusySeconds < 0 || m.CPU.BusySeconds > m.CPU.TotalSeconds {
		t.Errorf("cpu = %+v", m.CPU)
	}
	if m.Memory.Total == 0 || m.Memory.Used > m.Memory.Total {
		t.Errorf("memory = %+v", m.Memory)
	}
	if m.Storage.Total == 0 || m.Storage.Used > m.Storage.Total {
		t.Errorf("storage = %+v", m.Storage)
	}
}

// A data path that does not exist must degrade to zeros, which the page
// shows as a dash, rather than fail the request.
func TestMissingDataPathReadsAsZero(t *testing.T) {
	m := Default(filepath.Join(t.TempDir(), "missing")).Metrics()
	if m.Storage.Total != 0 || m.Storage.Used != 0 {
		t.Errorf("storage = %+v, want zeros", m.Storage)
	}
}
