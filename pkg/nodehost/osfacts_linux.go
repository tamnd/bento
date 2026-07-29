package nodehost

import "golang.org/x/sys/unix"

// loadScale is the fixed-point divisor the kernel scales the load averages by
// before putting them in the sysinfo struct, 1 << SI_LOAD_SHIFT.
const loadScale = 1 << 16

// readHostFacts answers the Linux facts from the two calls the kernel offers for
// them: uname for the strings, and sysinfo for the memory, the uptime and the load
// average. Node reaches the same numbers through libuv, which reads sysconf and
// /proc for some of them, but the values are the same ones the kernel keeps.
func readHostFacts() hostFacts {
	var f hostFacts
	var u unix.Utsname
	if err := unix.Uname(&u); err == nil {
		f.Release = utsString(u.Release[:])
		f.Version = utsString(u.Version[:])
		f.Machine = utsString(u.Machine[:])
	}
	var si unix.Sysinfo_t
	if err := unix.Sysinfo(&si); err != nil {
		return f
	}
	// Every memory field in the struct is a count of units, not of bytes, so the
	// unit is what turns them into the byte counts os.totalmem reports. A kernel
	// that reports no unit means one byte per unit.
	unit := uint64(si.Unit)
	if unit == 0 {
		unit = 1
	}
	f.TotalMem = uint64(si.Totalram) * unit
	f.FreeMem = uint64(si.Freeram) * unit
	f.Uptime = float64(si.Uptime)
	for i := range f.Loadavg {
		f.Loadavg[i] = float64(si.Loads[i]) / loadScale
	}
	return f
}
