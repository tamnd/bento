package nodehost

import (
	"os"
	"strconv"
	"strings"
)

// Linux counts processor time in ticks of USER_HZ, and os.cpus() reports
// milliseconds, so every count is multiplied by this. USER_HZ is what
// sysconf(_SC_CLK_TCK) answers, and the kernel fixes it at 100 on every
// architecture this compiler targets; reading it properly needs libc, which the
// pure-Go build does not have.
const msPerTick = 1000 / 100

// readCPUs answers os.cpus() on Linux out of the two files the kernel publishes:
// /proc/stat, which has a line per online core with the time it has spent in each
// scheduler state, and /proc/cpuinfo, which has the model name and the clock.
//
// /proc/stat is what decides how many cores there are, since it lists the cores
// the machine has rather than the ones this process is allowed on.
func readCPUs() []cpuInfo {
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil
	}
	models, speeds := readCPUInfoFile()
	var out []cpuInfo
	for _, line := range strings.Split(string(stat), "\n") {
		// The first line is the whole machine's total, named "cpu" with no number, and
		// is not a core. The per-core lines are cpu0, cpu1 and so on, in order.
		if !strings.HasPrefix(line, "cpu") || len(line) < 4 || line[3] < '0' || line[3] > '9' {
			continue
		}
		fields := strings.Fields(line)
		n, err := strconv.Atoi(strings.TrimPrefix(fields[0], "cpu"))
		if err != nil {
			continue
		}
		// user nice system idle iowait irq softirq, of which Node reports five: the two
		// it leaves out, iowait and softirq, have no field in the object it hands back.
		if len(fields) < 8 {
			continue
		}
		info := cpuInfo{Model: "unknown"}
		if m, ok := models[n]; ok {
			info.Model = m
		}
		info.Speed = speeds[n]
		if info.Speed == 0 {
			info.Speed = readCPUFreq(n)
		}
		info.Times = cpuTimes{
			User: ticksToMS(fields[1]),
			Nice: ticksToMS(fields[2]),
			Sys:  ticksToMS(fields[3]),
			Idle: ticksToMS(fields[4]),
			IRQ:  ticksToMS(fields[6]),
		}
		out = append(out, info)
	}
	return out
}

// ticksToMS converts one /proc/stat field to milliseconds. A field that does not
// parse counts as zero, which is what the kernel would have meant by a core that
// has never been in that state.
func ticksToMS(field string) int {
	ticks, err := strconv.Atoi(field)
	if err != nil {
		return 0
	}
	return ticks * msPerTick
}

// readCPUInfoFile pulls the model name and the clock out of /proc/cpuinfo, keyed
// by the core number the file itself gives each block.
//
// The keys differ by architecture. x86 writes "model name" and "cpu MHz"; arm
// writes neither on most kernels, and what it writes instead is the part number
// under "Processor" or the board under "Hardware". A core the file says nothing
// useful about is left out of both maps and reported as unknown at whatever the
// cpufreq driver says, which is what Node does there too.
func readCPUInfoFile() (map[int]string, map[int]int) {
	models, speeds := map[int]string{}, map[int]int{}
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return models, speeds
	}
	n := -1
	for _, line := range strings.Split(string(b), "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "processor":
			num, err := strconv.Atoi(val)
			if err != nil {
				n = -1
				continue
			}
			n = num
		case "model name", "Processor", "Hardware":
			// The first of the three that appears wins, so a kernel that writes both a
			// model name and a board name reports the model.
			if n >= 0 && val != "" {
				if _, seen := models[n]; !seen {
					models[n] = val
				}
			}
		case "cpu MHz", "clock":
			// The clock is written with a fraction, and megahertz is an integer to Node.
			if n < 0 {
				continue
			}
			if mhz, err := strconv.ParseFloat(strings.TrimSuffix(val, "MHz"), 64); err == nil {
				speeds[n] = int(mhz)
			}
		}
	}
	return models, speeds
}

// readCPUFreq answers a core's clock from the cpufreq driver, in megahertz, for
// the kernels that do not write one into /proc/cpuinfo. The driver publishes
// kilohertz. A machine with no cpufreq driver at all, which is most virtual ones,
// answers zero, and zero is what Node reports there.
func readCPUFreq(n int) int {
	dir := "/sys/devices/system/cpu/cpu" + strconv.Itoa(n) + "/cpufreq/"
	for _, name := range []string{"scaling_cur_freq", "cpuinfo_max_freq"} {
		b, err := os.ReadFile(dir + name)
		if err != nil {
			continue
		}
		khz, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil {
			continue
		}
		return khz / 1000
	}
	return 0
}
