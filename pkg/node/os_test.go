package node

import "testing"

// node:os has two data exports a program reads for a platform fact rather than
// calls: EOL, the line ending, and devNull, the path of the null device. The
// compiled path answers them from value.OSEOL and value.OSDevNull, and this pins
// the engine to the same answers, so a program gets one value whether it was run
// or built.
//
// Both sides are checked against Node's own spellings rather than against each
// other, which is what makes the check worth running: two implementations that
// agree on a wrong value would still pass a comparison between themselves.
func TestOSConstantsMatchNode(t *testing.T) {
	eng := harness(t)
	eol, devNull := `"\n"`, "/dev/null"
	if nodePlatform() == "win32" {
		eol, devNull = `"\r\n"`, `\\.\nul`
	}
	if got := evalString(t, eng, `JSON.stringify(require("os").EOL)`); got != eol {
		t.Errorf("os.EOL = %s, want %s on %s", got, eol, nodePlatform())
	}
	if got := evalString(t, eng, `require("os").devNull`); got != devNull {
		t.Errorf("os.devNull = %q, want %q on %s", got, devNull, nodePlatform())
	}
}
