package nodehost

import "runtime"

// CPUInfo is one entry of the array os.cpus() answers: the processor's model
// name, its clock in megahertz, and the time its core has spent in each of the
// scheduler's states since the machine booted.
type CPUInfo struct {
	Model string   `json:"model"`
	Speed int      `json:"speed"`
	Times CPUTimes `json:"times"`
}

// CPUTimes counts milliseconds, which is the unit Node reports and not the unit
// any kernel keeps. Every platform counts in something else, ticks on Linux and
// hundreds of nanoseconds on Windows, and converts on the way out.
type CPUTimes struct {
	User int `json:"user"`
	Nice int `json:"nice"`
	Sys  int `json:"sys"`
	Idle int `json:"idle"`
	IRQ  int `json:"irq"`
}

// cpuList answers os.cpus(). The per-platform readCPUs owns the whole answer,
// including how many cores there are, because the machine's core count is not the
// same question as the count the Go runtime reports: the runtime counts the cores
// this process may run on, and os.cpus() lists the cores the machine has.
//
// A platform with no file of its own, or one whose interface refused to answer,
// falls back to a row per runtime core with an unknown model. That keeps the
// length of the array right, which is the one thing about it programs read
// without checking, and says plainly that the rest is not known.
func cpuList() []CPUInfo {
	if cpus := readCPUs(); len(cpus) > 0 {
		return cpus
	}
	out := make([]CPUInfo, runtime.NumCPU())
	for i := range out {
		out[i] = CPUInfo{Model: "unknown"}
	}
	return out
}

// availableParallelism is os.availableParallelism(), the count of cores this
// process may actually run on. That is the question the Go runtime answers, from
// the same affinity mask the scheduler reads, so it is not derived from the length
// of os.cpus(): a process pinned to two cores of a machine with sixteen is told
// two here and still sees all sixteen there.
func availableParallelism() int {
	return runtime.NumCPU()
}
