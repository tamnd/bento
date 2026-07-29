//go:build !linux && !darwin && !windows

package nodehost

// readHostFacts answers nothing on a platform no file measures yet. It is the
// state every platform was in before any of them were measured, so a build for
// one of them is no worse than it was; the fields it leaves zero are the ones
// os.release, os.totalmem and their kin report as empty or zero.
//
// FreeBSD and its relatives are the near ones: uname and the sysctls they publish
// are close enough to Darwin's that a file for them is mostly a rename. They are
// left out rather than written blind, because this repository builds for them but
// does not run anything on them, and untested platform code that looks right is
// worse than a zero that is honest.
func readHostFacts() hostFacts {
	return hostFacts{}
}
