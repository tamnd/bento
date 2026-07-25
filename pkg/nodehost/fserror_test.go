package nodehost

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

// wantErrno is the number Node reports as err.errno for a code on this platform.
// It is written out here rather than read from the package under test, because a
// test that asks the implementation what the answer is proves nothing.
//
// Off Windows libuv negates the platform's own errno, so the expectation is
// derived the same way the implementation derives it but from the syscall
// constant directly, which is what catches a wrong entry in the table. ENOTEMPTY
// is 39 on Linux and 66 on Darwin, so a hardcoded POSIX number would be wrong on
// one of them. On Windows libuv assigns its own block, which is not derivable
// from anything, so those are the literal numbers from include/uv/errno.h.
func wantErrno(code string) int {
	if runtime.GOOS == "windows" {
		switch code {
		case "ENOENT":
			return -4058
		case "EEXIST":
			return -4075
		case "ENOTEMPTY":
			return -4051
		case "EACCES":
			return -4092
		case "EPERM":
			return -4048
		case "UNKNOWN":
			return -4094
		}
		return 0
	}
	switch code {
	case "ENOENT":
		return -int(syscall.ENOENT)
	case "EEXIST":
		return -int(syscall.EEXIST)
	case "ENOTEMPTY":
		return -int(syscall.ENOTEMPTY)
	case "EACCES":
		return -int(syscall.EACCES)
	case "EPERM":
		return -int(syscall.EPERM)
	case "UNKNOWN":
		return -4094
	}
	return 0
}

// TestClassifyNamesNodesCodeForARealFailure runs the three failures a program
// actually meets against the real file system, on whatever platform the test
// runs on. They are the three whose Windows translation is settled: a missing
// file is ERROR_FILE_NOT_FOUND, an existing directory is ERROR_ALREADY_EXISTS,
// and a non-empty one is ERROR_DIR_NOT_EMPTY, which libuv calls ENOENT, EEXIST
// and ENOTEMPTY. ENOTEMPTY is the one the old raw-errno comparison could not
// answer there at all.
func TestClassifyNamesNodesCodeForARealFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "child"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, missing := os.Open(filepath.Join(dir, "nope"))
	existing := os.Mkdir(dir, 0o755)
	notEmpty := os.Remove(dir)

	cases := []struct {
		err  error
		code string
		desc string
	}{
		{missing, "ENOENT", "no such file or directory"},
		{existing, "EEXIST", "file already exists"},
		{notEmpty, "ENOTEMPTY", "directory not empty"},
	}
	for _, c := range cases {
		if c.err == nil {
			t.Fatalf("want an error for %s, got none", c.code)
		}
		got := ClassifyFSError(c.err)
		if got.Code != c.code {
			t.Errorf("ClassifyFSError(%v).Code = %q, want %q", c.err, got.Code, c.code)
		}
		if got.Desc != c.desc {
			t.Errorf("ClassifyFSError(%v).Desc = %q, want %q", c.err, got.Desc, c.desc)
		}
		if want := wantErrno(c.code); got.Errno != want {
			t.Errorf("ClassifyFSError(%v).Errno = %d, want %d", c.err, got.Errno, want)
		}
	}
}

// TestClassifyKeepsPermissionAndOwnershipApart pins the reason the platform errno
// is read before the standard library's sentinels. EACCES and EPERM are one
// sentinel in Go, fs.ErrPermission, and two different codes in Node, so a
// classification that led with the sentinel would answer EACCES for both and a
// program branching on err.code would never see EPERM.
func TestClassifyKeepsPermissionAndOwnershipApart(t *testing.T) {
	cases := []struct {
		errno syscall.Errno
		code  string
	}{
		{syscall.EPERM, "EPERM"},
		{syscall.EACCES, "EACCES"},
	}
	for _, c := range cases {
		err := &fs.PathError{Op: "open", Path: "/x", Err: c.errno}
		if got := ClassifyFSError(err); got.Code != c.code {
			t.Errorf("ClassifyFSError(%v).Code = %q, want %q", err, got.Code, c.code)
		}
	}
}

// TestClassifyFallsBackToTheSentinels covers the error that carries no errno at
// all: one an io/fs implementation returned, or a sentinel a helper wrapped.
// There is no number to read there, so the sentinel is the only evidence.
func TestClassifyFallsBackToTheSentinels(t *testing.T) {
	cases := []struct {
		err  error
		code string
	}{
		{fs.ErrNotExist, "ENOENT"},
		{fmt.Errorf("read config: %w", fs.ErrNotExist), "ENOENT"},
		{fs.ErrExist, "EEXIST"},
		{fs.ErrPermission, "EACCES"},
		{fs.ErrClosed, "EBADF"},
	}
	for _, c := range cases {
		if got := ClassifyFSError(c.err); got.Code != c.code {
			t.Errorf("ClassifyFSError(%v).Code = %q, want %q", c.err, got.Code, c.code)
		}
	}
}

// TestClassifySaysWhatHappenedForAnUnknownError pins the one place bento does not
// copy libuv. An unclassified failure gets the code and the number libuv uses,
// because a program reading err.code has to see something it recognizes, but the
// description stays the Go error's own text: "unknown error" would throw away the
// only account of the failure there is.
func TestClassifySaysWhatHappenedForAnUnknownError(t *testing.T) {
	got := ClassifyFSError(errors.New("the disk fell over"))
	if got.Code != "UNKNOWN" {
		t.Errorf("Code = %q, want UNKNOWN", got.Code)
	}
	if got.Errno != wantErrno("UNKNOWN") {
		t.Errorf("Errno = %d, want %d", got.Errno, wantErrno("UNKNOWN"))
	}
	if got.Desc != "the disk fell over" {
		t.Errorf("Desc = %q, want the Go error's text", got.Desc)
	}
}

// TestEveryCodeHasADescriptionAndANumber guards the two tables against drifting
// apart. A code that reaches a program with no description prints a message with
// a hole in it, and one with no number reports errno as UNKNOWN's while its code
// says otherwise.
func TestEveryCodeHasADescriptionAndANumber(t *testing.T) {
	for code := range fsErrorDesc {
		if code == "UNKNOWN" {
			continue
		}
		if got := uvErrno(code); got == uvUnknownErrno {
			t.Errorf("uvErrno(%q) has no number of its own", code)
		}
	}
}
