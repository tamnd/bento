package value

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestWriteReadRoundtrip pins that WriteFileSync then ReadFileSyncUTF8 returns the
// bytes written, the core of the readwrite workload: a file written through the
// value helper reads back byte for byte through the value helper.
func TestWriteReadRoundtrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "data.txt")
	WriteFileSync(FromGoString(p), FromGoString("hello world"))
	if got := ReadFileSyncUTF8(FromGoString(p)).ToGoString(); got != "hello world" {
		t.Fatalf("read back %q, want %q", got, "hello world")
	}
}

// TestMkdtempCreatesDir pins that Mkdtemp creates a real directory whose name
// begins with the prefix's base, the shape Node's mkdtempSync returns.
func TestMkdtempCreatesDir(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "run-")
	dir := Mkdtemp(FromGoString(prefix)).ToGoString()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat created dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("Mkdtemp returned %q, not a directory", dir)
	}
	if !strings.HasPrefix(filepath.Base(dir), "run-") {
		t.Fatalf("created dir base %q does not start with prefix", filepath.Base(dir))
	}
}

// TestRmSyncFile pins that RmSync without recursive removes a single file.
func TestRmSyncFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "gone.txt")
	WriteFileSync(FromGoString(p), FromGoString("x"))
	RmSync(FromGoString(p), false, false)
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("file still present after RmSync: %v", err)
	}
}

// TestRmSyncRecursive pins that RmSync with recursive removes a non-empty
// directory tree, the cleanup the readwrite workload does at the end.
func TestRmSyncRecursive(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tree")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o777); err != nil {
		t.Fatal(err)
	}
	WriteFileSync(FromGoString(filepath.Join(root, "sub", "f.txt")), FromGoString("x"))
	RmSync(FromGoString(root), true, false)
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("tree still present after recursive RmSync: %v", err)
	}
}

// TestRmSyncForceMissing pins that force suppresses the error a missing path
// raises, so a cleanup that runs when nothing is there does not panic.
func TestRmSyncForceMissing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "never")
	RmSync(FromGoString(p), true, true) // must not panic
}

// TestRmSyncMissingPanics pins that without force a missing path is a thrown
// error, surfaced as a panic.
func TestRmSyncMissingPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("RmSync of a missing path without force did not panic")
		}
	}()
	RmSync(FromGoString(filepath.Join(t.TempDir(), "never")), false, false)
}

// TestMkdtempPrefixEndingInSeparator pins that a prefix ending in a separator
// creates a randomly named directory inside it, the way Node's mkdtempSync does,
// rather than one named after the parent directory. Splitting with Dir and Base
// instead of Split got this wrong on every platform: Base("/tmp/") is "tmp", so
// the created directory came out as /tmp/tmpXXXXXX.
func TestMkdtempPrefixEndingInSeparator(t *testing.T) {
	parent := t.TempDir()
	dir := Mkdtemp(FromGoString(parent + string(filepath.Separator))).ToGoString()
	if got := filepath.Dir(dir); got != parent {
		t.Fatalf("created %q under %q, want it under %q", dir, got, parent)
	}
	base := filepath.Base(dir)
	if strings.HasPrefix(base, filepath.Base(parent)) {
		t.Fatalf("created dir base %q repeats the parent's name", base)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("stat created dir %q: %v", dir, err)
	}
}

// TestMkdtempBarePrefixLandsInCwd pins that a prefix with no directory part is
// created in the working directory, which is where Node puts it, rather than
// erroring on an empty parent.
func TestMkdtempBarePrefixLandsInCwd(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := Mkdtemp(FromGoString("run-")).ToGoString()
	if filepath.Dir(dir) != "." {
		t.Fatalf("Mkdtemp(%q) = %q, want it relative to the working directory", "run-", dir)
	}
}

// TestPathJoin pins that PathJoin matches path.join's normalization: redundant
// separators collapse and "." and ".." segments resolve. The expected string is
// built with filepath because node:path is path.win32 on Windows, so the answer
// carries the platform's separator and Node's does too.
func TestPathJoin(t *testing.T) {
	got := PathJoin(FromGoString("a"), FromGoString("b"), FromGoString("../c")).ToGoString()
	if want := filepath.Join("a", "c"); got != want {
		t.Fatalf("PathJoin = %q, want %q", got, want)
	}
}

// TestPathJoinNoParts pins path.join()'s empty-input answer, ".", which
// filepath.Join spells as the empty string.
func TestPathJoinNoParts(t *testing.T) {
	if got := PathJoin().ToGoString(); got != "." {
		t.Fatalf("PathJoin() = %q, want %q", got, ".")
	}
}

// TestPathJoinUsesThePlatformSeparator pins that the joined string is a host path
// and not a module path, so it is the user program's answer rather than the
// compiler's bookkeeping. On Unix the two conventions agree, so this asserts
// against filepath rather than against a literal.
func TestPathJoinUsesThePlatformSeparator(t *testing.T) {
	got := PathJoin(FromGoString("a"), FromGoString("b")).ToGoString()
	if !strings.Contains(got, string(filepath.Separator)) {
		t.Fatalf("PathJoin = %q, want the platform separator %q", got, string(filepath.Separator))
	}
}

// TestTmpdirNonEmpty pins that Tmpdir returns a non-empty path, the directory a
// temp workload roots its tree at.
func TestTmpdirNonEmpty(t *testing.T) {
	if Tmpdir().ToGoString() == "" {
		t.Fatal("Tmpdir returned empty")
	}
}

// TestOSConstantsMatchNode pins os.EOL and os.devNull against the spellings Node
// reports, per platform. They are the two facts about the host a program reads
// off node:os rather than computing, and Node's win32 devNull is a device
// namespace path rather than a file path, which is the part worth writing down.
func TestOSConstantsMatchNode(t *testing.T) {
	wantEOL, wantDevNull := "\n", "/dev/null"
	if runtime.GOOS == "windows" {
		wantEOL, wantDevNull = "\r\n", `\\.\nul`
	}
	if got := OSEOL().ToGoString(); got != wantEOL {
		t.Errorf("OSEOL = %q, want %q on %s", got, wantEOL, runtime.GOOS)
	}
	if got := OSDevNull().ToGoString(); got != wantDevNull {
		t.Errorf("OSDevNull = %q, want %q on %s", got, wantDevNull, runtime.GOOS)
	}
}
