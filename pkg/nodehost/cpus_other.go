//go:build !linux && !darwin && !windows

package nodehost

// readCPUs answers nothing on a platform no file reads a processor list from, and
// cpuList falls back to one unknown core per core the Go runtime can see. That is
// the same answer every platform gave before any of them read a real one, so a
// build for one of these is no worse off than it was.
func readCPUs() []cpuInfo {
	return nil
}
