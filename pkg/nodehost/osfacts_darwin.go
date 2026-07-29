package nodehost

import (
	"encoding/binary"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// readHostFacts answers the Darwin facts from uname and from the sysctls the
// kernel publishes the numbers under. Node reaches them through libuv, which calls
// the Mach host_statistics interface for free memory; the sysctls below are the
// same counters that interface reports, which is what makes them comparable.
func readHostFacts() hostFacts {
	var f hostFacts
	var u unix.Utsname
	if err := unix.Uname(&u); err == nil {
		f.Release = utsString(u.Release[:])
		f.Version = utsString(u.Version[:])
		f.Machine = utsString(u.Machine[:])
	}
	if n, err := unix.SysctlUint64("hw.memsize"); err == nil {
		f.TotalMem = n
	}
	f.FreeMem = darwinFreeMem()
	f.Uptime = darwinUptime()
	f.Loadavg = darwinLoadavg()
	return f
}

// darwinFreeMem returns the free physical memory in bytes, the number
// os.freemem() answers. libuv reads free_count out of HOST_VM_INFO, and that
// count is the kernel's free pages plus its speculative pages, which the kernel
// publishes separately as two sysctls. Adding them is what makes this agree with
// Node rather than undercounting by whatever is currently speculative, which on a
// warm machine is hundreds of megabytes.
func darwinFreeMem() uint64 {
	free, err := unix.SysctlUint32("vm.page_free_count")
	if err != nil {
		return 0
	}
	speculative, err := unix.SysctlUint32("vm.page_speculative_count")
	if err != nil {
		speculative = 0
	}
	return (uint64(free) + uint64(speculative)) * uint64(os.Getpagesize())
}

// darwinUptime returns the seconds since boot. The kernel publishes the boot time
// rather than the elapsed time, so the elapsed time is the difference.
func darwinUptime() float64 {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return 0
	}
	boot := time.Unix(int64(tv.Sec), int64(tv.Usec)*1000)
	return time.Since(boot).Seconds()
}

// darwinLoadavg reads the three load averages out of the vm.loadavg sysctl, which
// hands back a struct loadavg: three fixed-point averages followed by the scale to
// divide them by. The scale sits after the three 32-bit averages as a long, which
// the C compiler aligns to eight bytes, so it starts at offset 16 rather than at
// 12. A shorter buffer than that is read the unaligned way instead, so a platform
// that lays the struct out without the padding is still read correctly rather than
// being read past its end.
func darwinLoadavg() [3]float64 {
	raw, err := unix.SysctlRaw("vm.loadavg")
	if err != nil || len(raw) < 16 {
		return [3]float64{}
	}
	var scale float64
	if len(raw) >= 24 {
		scale = float64(binary.LittleEndian.Uint64(raw[16:24]))
	} else {
		scale = float64(binary.LittleEndian.Uint32(raw[12:16]))
	}
	if scale == 0 {
		return [3]float64{}
	}
	var out [3]float64
	for i := range out {
		out[i] = float64(binary.LittleEndian.Uint32(raw[i*4:i*4+4])) / scale
	}
	return out
}
