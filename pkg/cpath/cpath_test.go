package cpath

import (
	"path/filepath"
	"runtime"
	"testing"
)

// TestFromOSTurnsWindowsSeparatorsAround pins the conversion in the direction that
// matters, and pins that it is a no-op for a path that is already a checker path.
// The Windows cases are asserted on every platform, because the normalizer is the
// compiler's and does not consult runtime.GOOS: a checker path is the same string
// wherever it is computed, which is what makes a golden portable.
func TestFromOSTurnsWindowsSeparatorsAround(t *testing.T) {
	cases := []struct{ in, want string }{
		{`C:\Users\x\main.ts`, "C:/Users/x/main.ts"},
		{`C:\Users\x\..\y\main.ts`, "C:/Users/y/main.ts"},
		{"C:/Users/x/main.ts", "C:/Users/x/main.ts"},
		{"/home/x/main.ts", "/home/x/main.ts"},
		{"/home/x/./main.ts", "/home/x/main.ts"},
		{"", ""},
	}
	for _, c := range cases {
		if got := FromOS(c.in); got != c.want {
			t.Errorf("FromOS(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestFromOSIsIdempotent is the guard against the half-normalized state the package
// exists to prevent: a path that has crossed the boundary must be unharmed by
// crossing it again, or the seam cannot be widened one caller at a time.
func TestFromOSIsIdempotent(t *testing.T) {
	for _, in := range []string{`C:\Users\x\main.ts`, "/home/x/main.ts", "C:/a/../b/main.ts"} {
		once := FromOS(in)
		if twice := FromOS(once); twice != once {
			t.Errorf("FromOS(%q) = %q, then %q: not idempotent", in, once, twice)
		}
	}
}

// TestToOSRoundTrips pins that a path survives the trip out to the operating system
// and back, which is what happens to every path bento reads from disk.
func TestToOSRoundTrips(t *testing.T) {
	for _, p := range []string{"/home/x/main.ts", "C:/Users/x/main.ts"} {
		if runtime.GOOS != "windows" && Volume(p) != "" {
			continue // A drive-lettered path is not a path this platform can name.
		}
		if got := FromOS(ToOS(p)); got != p {
			t.Errorf("FromOS(ToOS(%q)) = %q", p, got)
		}
	}
	if got := ToOS("/home/x/main.ts"); got != filepath.FromSlash("/home/x/main.ts") {
		t.Errorf("ToOS = %q", got)
	}
}

// TestIsAbsSeesAVolume pins the reason IsAbs is not path.IsAbs: a drive-lettered
// path is absolute and path.IsAbs says it is not.
func TestIsAbsSeesAVolume(t *testing.T) {
	for _, p := range []string{"C:/Users/x", "/home/x"} {
		if !IsAbs(p) {
			t.Errorf("IsAbs(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"x/main.ts", "./main.ts", ""} {
		if IsAbs(p) {
			t.Errorf("IsAbs(%q) = true, want false", p)
		}
	}
}

// TestDirKeepsTheVolume pins the one place path.Dir gives an answer a caller cannot
// use: for "C:/main.ts" it says "C:", which names the working directory on that
// drive rather than its root.
func TestDirKeepsTheVolume(t *testing.T) {
	cases := []struct{ in, want string }{
		{"C:/main.ts", "C:/"},
		{"C:/Users/x/main.ts", "C:/Users/x"},
		{"C:/", "C:/"},
		{"/main.ts", "/"},
		{"/home/x/main.ts", "/home/x"},
		{"", "."},
	}
	for _, c := range cases {
		if got := Dir(c.in); got != c.want {
			t.Errorf("Dir(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestJoinNormalizes pins that Join resolves a ".." in a later element rather than
// carrying it, so a joined path is a checker path and not a path that has to be
// normalized again before it is used as a key.
func TestJoinNormalizes(t *testing.T) {
	cases := []struct {
		elem []string
		want string
	}{
		{[]string{"C:/Users/x", "..", "y", "main.ts"}, "C:/Users/y/main.ts"},
		{[]string{"/home/x", "sub", "main.ts"}, "/home/x/sub/main.ts"},
		{[]string{"/home/x", "./main.ts"}, "/home/x/main.ts"},
	}
	for _, c := range cases {
		if got := Join(c.elem...); got != c.want {
			t.Errorf("Join(%q) = %q, want %q", c.elem, got, c.want)
		}
	}
}

// TestVolumeReadsADOSRoot pins what counts as a volume, including the cases that
// look like one and are not.
func TestVolumeReadsADOSRoot(t *testing.T) {
	cases := []struct{ in, want string }{
		{"C:/Users/x", "C:/"},
		{"c:/users/x", "c:/"},
		{"/home/x", ""},
		{"C:", ""},
		{"CC:/x", ""},
		{"1:/x", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := Volume(c.in); got != c.want {
			t.Errorf("Volume(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestVirtualJoinsTheProgramsVolume pins the rule that keeps the checker's file map
// from mixing path styles: a synthetic file's path takes the volume of a real file
// in the same program. Without it a Windows build hands the compiler a map with a
// "/__bento_ambient__.d.ts" key beside a "C:/..." key and the compiler panics.
func TestVirtualJoinsTheProgramsVolume(t *testing.T) {
	cases := []struct{ virtual, sibling, want string }{
		{"/__bento_ambient__.d.ts", "C:/Users/x/main.ts", "C:/__bento_ambient__.d.ts"},
		{"/__bento_ambient__.d.ts", "/home/x/main.ts", "/__bento_ambient__.d.ts"},
		{"/__bento_go__/fmt.d.ts", "D:/src/main.ts", "D:/__bento_go__/fmt.d.ts"},
		// A virtual path that already carries a volume is left alone, so the call is
		// safe to make twice.
		{"C:/__bento_ambient__.d.ts", "C:/Users/x/main.ts", "C:/__bento_ambient__.d.ts"},
		// And a relative sibling has no volume to give.
		{"/__bento_ambient__.d.ts", "main.ts", "/__bento_ambient__.d.ts"},
	}
	for _, c := range cases {
		if got := Virtual(c.virtual, c.sibling); got != c.want {
			t.Errorf("Virtual(%q, %q) = %q, want %q", c.virtual, c.sibling, got, c.want)
		}
	}
}

// TestAbsAnswersInCheckerPaths pins that Abs never hands back a backslash, which is
// the whole point of it existing next to filepath.Abs.
func TestAbsAnswersInCheckerPaths(t *testing.T) {
	got, err := Abs("main.ts")
	if err != nil {
		t.Fatal(err)
	}
	if !IsAbs(got) {
		t.Errorf("Abs(%q) = %q, which is not absolute", "main.ts", got)
	}
	if FromOS(got) != got {
		t.Errorf("Abs returned %q, which is not a checker path", got)
	}
}
