package nodehost

import (
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// processorPerformance is the SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION the kernel
// fills in per core, one entry of an array. Every time in it is counted in
// hundreds of nanoseconds, the unit Windows counts every time in.
type processorPerformance struct {
	IdleTime       int64
	KernelTime     int64
	UserTime       int64
	DpcTime        int64
	InterruptTime  int64
	InterruptCount uint32
	_              uint32 // The struct is eight-byte aligned, so the count is padded.
}

// hundredNSPerMS is how many of the kernel's ticks make the millisecond os.cpus()
// reports.
const hundredNSPerMS = 10000

// readCPUs answers os.cpus() on Windows. The model and the clock are what the
// firmware wrote to the registry at boot, one key per core, and the times come
// from the undocumented system call that is the only interface to them, which is
// the same one libuv uses.
func readCPUs() []CPUInfo {
	n := int(nativeSystemInfo().NumberOfProcessors)
	if n <= 0 {
		return nil
	}
	times := processorTimes(n)
	out := make([]CPUInfo, n)
	for i := range out {
		out[i] = CPUInfo{Model: "unknown"}
		if model, mhz, ok := registryCPU(i); ok {
			out[i].Model, out[i].Speed = model, mhz
		}
		if i < len(times) {
			t := times[i]
			out[i].Times = CPUTimes{
				User: int(t.UserTime / hundredNSPerMS),
				// Windows does not schedule by niceness and keeps no count for it, and Node
				// reports zero there rather than folding it into another state.
				Nice: 0,
				// KernelTime counts idle time as well as kernel time, so the time actually
				// spent in the kernel is the difference.
				Sys:  int((t.KernelTime - t.IdleTime) / hundredNSPerMS),
				Idle: int(t.IdleTime / hundredNSPerMS),
				IRQ:  int(t.InterruptTime / hundredNSPerMS),
			}
		}
	}
	return out
}

// processorTimes asks the kernel for the per-core time counters. There is no
// documented call for them; NtQuerySystemInformation is what Windows itself uses
// and what libuv calls, and a kernel that refuses answers no times rather than
// failing the whole of os.cpus(), which still has a model and a clock to report.
func processorTimes(n int) []processorPerformance {
	buf := make([]processorPerformance, n)
	var got uint32
	err := windows.NtQuerySystemInformation(
		windows.SystemProcessorPerformanceInformation,
		unsafe.Pointer(&buf[0]),
		uint32(len(buf))*uint32(unsafe.Sizeof(buf[0])),
		&got,
	)
	if err != nil {
		return nil
	}
	// The kernel reports how much it wrote, which is one entry per core it counted.
	// That can be fewer than were asked for, and reading past it would be reading
	// the zeroes this buffer was made with.
	if filled := int(got) / int(unsafe.Sizeof(buf[0])); filled < n {
		return buf[:filled]
	}
	return buf
}

// registryCPU reads one core's model name and clock from the key the firmware
// filled in at boot. The clock there is a fixed number written once, the rated
// speed rather than the current one, which is the number Node reports on Windows.
func registryCPU(n int) (string, int, bool) {
	path := `HARDWARE\DESCRIPTION\System\CentralProcessor\` + strconv.Itoa(n)
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return "", 0, false
	}
	defer k.Close()
	model, _, err := k.GetStringValue("ProcessorNameString")
	if err != nil {
		return "", 0, false
	}
	mhz, _, err := k.GetIntegerValue("~MHz")
	if err != nil {
		return model, 0, true
	}
	return model, int(mhz), true
}
