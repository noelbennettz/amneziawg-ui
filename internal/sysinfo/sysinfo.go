// Package sysinfo reads the host the backend runs on - processor, memory and
// the disk under the data volume - for the dashboard's system tiles. It is
// the one package besides awg that looks at the host, through gopsutil
// rather than a command.
//
// Every reading is instantaneous. The CPU figures are the cumulative times
// the kernel counts since boot; the page differences two of them to get the
// load over its own polling interval, so nothing has to be remembered here.
package sysinfo

import (
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"

	"amneziawg-web-ui/web-ui/api"
)

// Host names what to report on: Data is a path on the filesystem whose usage
// the storage tile shows.
type Host struct {
	Data string
}

// Default reports the disk under dataDir.
func Default(dataDir string) Host {
	return Host{Data: dataDir}
}

// Metrics takes one reading of everything. A reading that fails leaves its
// part zeroed rather than failing the whole call: the page shows a dash for
// it and the other tiles keep working.
func (h Host) Metrics() api.SystemMetrics {
	return api.SystemMetrics{
		CPU:     cpuMetrics(),
		Memory:  memory(),
		Storage: storage(h.Data),
	}
}

// cpuMetrics sums the times of every core since boot. Idle and iowait are
// the two states that are not work; everything else the kernel lists (user,
// nice, system, irq, softirq, steal) is.
func cpuMetrics() api.CPUMetrics {
	var m api.CPUMetrics
	m.Cores, _ = cpu.Counts(true)

	times, err := cpu.Times(false)
	if err != nil || len(times) == 0 {
		return m
	}
	t := times[0]
	m.TotalSeconds = t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
	m.BusySeconds = m.TotalSeconds - t.Idle - t.Iowait
	return m
}

// memory is what free(1) reports: used is what the kernel could not hand to
// a new allocation without reclaiming, not everything that is not free.
func memory() api.UsageMetrics {
	v, err := mem.VirtualMemory()
	if err != nil || v.Total == 0 {
		return api.UsageMetrics{}
	}
	return api.UsageMetrics{Total: v.Total, Used: v.Used}
}

// storage reports the filesystem holding path, the way df does.
func storage(path string) api.UsageMetrics {
	u, err := disk.Usage(path)
	if err != nil || u.Total == 0 {
		return api.UsageMetrics{}
	}
	return api.UsageMetrics{Total: u.Total, Used: u.Used}
}
