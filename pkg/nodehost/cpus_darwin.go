package nodehost

import "golang.org/x/sys/unix"

// appleSiliconSpeed is the clock reported for a Mac whose kernel publishes no
// hw.cpufrequency, which is every Apple Silicon one. libuv writes the same number
// in for the same reason, so this is what Node answers on these machines, and it
// is a constant rather than a measurement on both sides.
const appleSiliconSpeed = 2400

// readCPUs answers os.cpus() on Darwin. The model and the count come from sysctls,
// and the clock comes from a sysctl on Intel and from a constant on Apple Silicon,
// where the kernel publishes none.
//
// The times are left at zero, and that is a gap rather than a measurement. Darwin
// keeps per-core times behind host_processor_info, a Mach call in libSystem with
// no sysctl in front of it and no route to it from a pure Go build, and this
// repository builds without cgo so that its output cross-compiles. Reporting zeros
// is the honest shape of not knowing; a plausible invented number would not be.
func readCPUs() []CPUInfo {
	n, err := unix.SysctlUint32("hw.logicalcpu")
	if err != nil || n == 0 {
		return nil
	}
	model, err := unix.Sysctl("machdep.cpu.brand_string")
	if err != nil || model == "" {
		model = "unknown"
	}
	speed := appleSiliconSpeed
	if hz, err := unix.SysctlUint64("hw.cpufrequency"); err == nil && hz > 0 {
		speed = int(hz / 1e6)
	}
	out := make([]CPUInfo, n)
	for i := range out {
		out[i] = CPUInfo{Model: model, Speed: speed}
	}
	return out
}
