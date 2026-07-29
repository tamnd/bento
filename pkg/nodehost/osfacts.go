package nodehost

// The os module reports a handful of facts about the machine the Go runtime does
// not carry: the kernel release and version, the hardware name uname reports, how
// much memory the machine has and how much of it is free, how long it has been up,
// and the load average. Node reads each of those through libuv, which reaches for
// a different interface on every platform, so each platform answers them here in
// its own file and the shared snapshot reads one struct.
//
// Every field has a zero that means "not known on this platform" rather than an
// error, because the snapshot is a whole-object read: a program that asks for
// os.arch on a platform bento cannot measure memory on should still get its arch.
// A platform with no file of its own answers zeros for all of them, which is what
// the whole set answered before any of them were measured.

// hostFacts is the platform detail collectOSInfo cannot get from the Go runtime.
type hostFacts struct {
	Release  string
	Version  string
	Machine  string
	TotalMem uint64
	FreeMem  uint64
	Uptime   float64
	Loadavg  [3]float64
}

// utsString reads a NUL-terminated field out of a utsname buffer. The buffers are
// fixed-size byte arrays whose contents stop at the first NUL, and their length
// differs by platform (65 bytes on Linux, 256 on Darwin), so this takes a slice
// and both callers pass their own array.
func utsString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
