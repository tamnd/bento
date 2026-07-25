package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestExeNameFollowsThePlatform pins the naming rule on both sides of it: Windows
// decides what a program is by extension, so a name without .exe gets one there
// and keeps whatever it has everywhere else. The expectation is written from
// runtime.GOOS rather than from the function, so the test says what should happen
// instead of agreeing with whatever does.
func TestExeNameFollowsThePlatform(t *testing.T) {
	cases := []struct{ name, windows string }{
		{"hello", "hello.exe"},
		{"hello.exe", "hello.exe"},
		// Windows does not care how the extension is spelled, so an upper-case one
		// is already an executable and appending to it would name a file nobody
		// asked for.
		{"HELLO.EXE", "HELLO.EXE"},
		// An extension that is not .exe is still not an executable on Windows, so
		// the suffix goes on the end of the whole name the way go build's does.
		{"a.out", "a.out.exe"},
		{filepath.Join("build", "prog"), filepath.Join("build", "prog") + ".exe"},
	}
	for _, c := range cases {
		want := c.name
		if runtime.GOOS == "windows" {
			want = c.windows
		}
		if got := ExeName(c.name); got != want {
			t.Errorf("ExeName(%q) = %q, want %q", c.name, got, want)
		}
	}
}

// TestBuildWritesTheNameItReports is the claim a caller depends on: the path Build
// hands back is a file, and running it runs the program. On Windows that is not
// the path the caller named, which is the whole reason Build reports one.
//
// Both naming routes are covered, the default taken from the entry and an explicit
// output that leaves the extension off, because a Windows user meets them both.
func TestBuildWritesTheNameItReports(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "hello.ts")
	if err := os.WriteFile(entry, []byte("console.log(\"hello\");\n"), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}

	// The default output lands in the working directory, so the build runs in a
	// temporary one rather than dropping a binary in the checkout. Moving out of
	// the checkout is also how the module root gets lost, since it is found by
	// walking up from the test binary or the working directory and the test binary
	// lives in a temporary directory of its own, so it is pinned first.
	if wd, err := os.Getwd(); err == nil {
		if root, ok := findModuleRoot(wd); ok {
			t.Setenv("BENTO_MODULE_ROOT", root)
		}
	}
	t.Chdir(dir)

	for _, c := range []struct{ what, output, base string }{
		{"the default name", "", "hello"},
		{"an explicit -o", filepath.Join(dir, "named"), "named"},
	} {
		prog, err := Build(Options{Entry: entry, Output: c.output})
		if err != nil {
			t.Fatalf("build with %s: %v", c.what, err)
		}
		want := c.base
		if runtime.GOOS == "windows" {
			want += ".exe"
		}
		if got := filepath.Base(prog); got != want {
			t.Errorf("build with %s wrote %q, want the base name %q", c.what, got, want)
		}
		if _, err := os.Stat(prog); err != nil {
			t.Fatalf("build with %s reported %q: %v", c.what, prog, err)
		}
		out, err := exec.Command(prog).CombinedOutput()
		if err != nil {
			t.Fatalf("run %q: %v (%s)", prog, err, out)
		}
		if got := strings.TrimSpace(string(out)); got != "hello" {
			t.Errorf("run %q printed %q, want hello", prog, got)
		}
	}
}
