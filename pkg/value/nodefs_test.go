package value

import (
	"os"
	"path/filepath"
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

// TestPathJoin pins that PathJoin matches path.join's normalization: redundant
// separators collapse and "." and ".." segments resolve.
func TestPathJoin(t *testing.T) {
	got := PathJoin(FromGoString("a"), FromGoString("b"), FromGoString("../c")).ToGoString()
	if got != "a/c" {
		t.Fatalf("PathJoin = %q, want %q", got, "a/c")
	}
}

// TestTmpdirNonEmpty pins that Tmpdir returns a non-empty path, the directory a
// temp workload roots its tree at.
func TestTmpdirNonEmpty(t *testing.T) {
	if Tmpdir().ToGoString() == "" {
		t.Fatal("Tmpdir returned empty")
	}
}
