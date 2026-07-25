package node

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/engine"
)

// mkdtemp calls the host primitive the way the fs module does and returns the
// created path, failing the test if the envelope came back with an error.
func mkdtemp(t *testing.T, prefix string) string {
	t.Helper()
	raw, err := hostFSMkdtemp([]any{prefix})
	if err != nil {
		t.Fatalf("mkdtemp(%q): %v", prefix, err)
	}
	var res fsResult
	if err := json.Unmarshal([]byte(raw.(string)), &res); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if !res.OK {
		t.Fatalf("mkdtemp(%q) failed: %s %s", prefix, res.Code, res.Desc)
	}
	return res.Path
}

// TestMkdtempAppendsToThePrefix pins Node's contract: the six random characters
// go straight onto the prefix, in the prefix's own directory.
func TestMkdtempAppendsToThePrefix(t *testing.T) {
	parent := t.TempDir()
	dir := mkdtemp(t, filepath.Join(parent, "run-"))
	if got := filepath.Dir(dir); got != parent {
		t.Fatalf("created %q under %q, want it under %q", dir, got, parent)
	}
	if base := filepath.Base(dir); !strings.HasPrefix(base, "run-") {
		t.Fatalf("created dir base %q does not start with the prefix", base)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("stat %q: %v", dir, err)
	}
}

// TestMkdtempPrefixEndingInSeparator pins that a prefix ending in a separator
// asks for a randomly named directory inside it, the way Node's does. Splitting
// the prefix with Dir and Base instead of Split got this wrong on every platform,
// since Base("/tmp/") is "tmp": the directory came out as /tmp/tmpXXXXXX where
// Node makes /tmp/XXXXXX.
func TestMkdtempPrefixEndingInSeparator(t *testing.T) {
	parent := t.TempDir()
	dir := mkdtemp(t, parent+string(filepath.Separator))
	if got := filepath.Dir(dir); got != parent {
		t.Fatalf("created %q under %q, want it under %q", dir, got, parent)
	}
	if base := filepath.Base(dir); strings.HasPrefix(base, filepath.Base(parent)) {
		t.Fatalf("created dir base %q repeats the parent's name", base)
	}
}

// TestMkdtempBarePrefixLandsInCwd pins that a prefix with no directory part is
// created in the working directory. os.MkdirTemp reads an empty dir argument as
// the temp directory, which is not where Node puts it.
func TestMkdtempBarePrefixLandsInCwd(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := mkdtemp(t, "run-")
	if filepath.Dir(dir) != "." {
		t.Fatalf("mkdtemp(%q) = %q, want it relative to the working directory", "run-", dir)
	}
}

// caughtError runs a filesystem call expected to throw and reports the error's
// Node-visible properties. Anything the call does not set comes back empty.
func caughtError(t *testing.T, eng engine.Engine, call string) map[string]string {
	t.Helper()
	raw := evalString(t, eng, `(function () {
		const fs = require("fs");
		try { `+call+`; return "no error"; }
		catch (e) {
			return JSON.stringify({
				code: e.code, errno: String(e.errno), syscall: e.syscall,
				path: e.path === undefined ? "" : e.path,
				dest: e.dest === undefined ? "" : e.dest,
				message: e.message, name: e.name,
			});
		}
	})()`)
	var got map[string]string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("%s: %s", call, raw)
	}
	return got
}

// TestFailedCallCarriesNodesError walks the failure path of every filesystem call
// that names a path, on whatever platform the test runs on, and pins what a
// program actually reads: err.code, err.syscall, err.path, and the message Node
// prints, which is "${code}: ${description}, ${syscall} '${path}'".
//
// The three failures are the ones whose Windows answer is settled, so the
// expectations hold on every platform without a branch: a missing file is
// ERROR_FILE_NOT_FOUND, an existing directory is ERROR_ALREADY_EXISTS, and a
// non-empty one is ERROR_DIR_NOT_EMPTY, which libuv calls ENOENT, EEXIST and
// ENOTEMPTY.
func TestFailedCallCarriesNodesError(t *testing.T) {
	eng := harness(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "child"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "nope")
	quoted := strconv.Quote(missing)

	cases := []struct {
		call    string
		code    string
		syscall string
		path    string
	}{
		{`fs.readFileSync(` + quoted + `)`, "ENOENT", "open", missing},
		{`fs.writeFileSync(` + strconv.Quote(filepath.Join(missing, "f")) + `, "x")`, "ENOENT", "open", filepath.Join(missing, "f")},
		{`fs.statSync(` + quoted + `)`, "ENOENT", "stat", missing},
		{`fs.lstatSync(` + quoted + `)`, "ENOENT", "lstat", missing},
		{`fs.accessSync(` + quoted + `)`, "ENOENT", "access", missing},
		{`fs.readdirSync(` + quoted + `)`, "ENOENT", "scandir", missing},
		{`fs.unlinkSync(` + quoted + `)`, "ENOENT", "unlink", missing},
		// Node's realpathSync reports the lstat it was walking with, not realpath.
		{`fs.realpathSync(` + quoted + `)`, "ENOENT", "lstat", missing},
		{`fs.mkdirSync(` + strconv.Quote(dir) + `)`, "EEXIST", "mkdir", dir},
		{`fs.rmdirSync(` + strconv.Quote(dir) + `)`, "ENOTEMPTY", "rmdir", dir},
	}
	for _, c := range cases {
		got := caughtError(t, eng, c.call)
		if got["code"] != c.code {
			t.Errorf("%s: code = %q, want %q (message %q)", c.call, got["code"], c.code, got["message"])
			continue
		}
		if got["syscall"] != c.syscall {
			t.Errorf("%s: syscall = %q, want %q", c.call, got["syscall"], c.syscall)
		}
		if got["path"] != c.path {
			t.Errorf("%s: path = %q, want %q", c.call, got["path"], c.path)
		}
		want := c.code + ": " + descOf(t, c.code) + ", " + c.syscall + " '" + c.path + "'"
		if got["message"] != want {
			t.Errorf("%s: message = %q, want %q", c.call, got["message"], want)
		}
		if got["name"] != "Error" {
			t.Errorf("%s: name = %q, want Error", c.call, got["name"])
		}
		if n, err := strconv.Atoi(got["errno"]); err != nil || n >= 0 {
			t.Errorf("%s: errno = %q, want a negative number", c.call, got["errno"])
		}
	}
}

// TestFailedReadOfADirectoryBlamesTheRead pins the one call whose failure is not
// the one the caller made. readFileSync opens and then reads, and a directory
// opens perfectly well on every platform bento builds for, so the failure is the
// read. Node reports it as such and names no path at all, which is why the host
// gets to override the syscall the JavaScript side would otherwise report.
func TestFailedReadOfADirectoryBlamesTheRead(t *testing.T) {
	eng := harness(t)
	dir := t.TempDir()
	got := caughtError(t, eng, `fs.readFileSync(`+strconv.Quote(dir)+`)`)
	if got["code"] != "EISDIR" {
		t.Fatalf("code = %q, want EISDIR (message %q)", got["code"], got["message"])
	}
	if got["syscall"] != "read" {
		t.Errorf("syscall = %q, want read", got["syscall"])
	}
	if got["path"] != "" {
		t.Errorf("path = %q, want none: Node reports no path for this one", got["path"])
	}
	want := "EISDIR: illegal operation on a directory, read"
	if got["message"] != want {
		t.Errorf("message = %q, want %q", got["message"], want)
	}
}

// TestFailedTwoPathCallNamesBothPaths pins the other message shape. rename,
// copyfile and symlink name a source and a destination, and Node prints both:
// "ENOENT: no such file or directory, rename '/a' -> '/b'", with err.dest
// alongside err.path.
func TestFailedTwoPathCallNamesBothPaths(t *testing.T) {
	eng := harness(t)
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope")
	to := filepath.Join(dir, "to")

	for _, c := range []struct{ call, syscall string }{
		{`fs.renameSync(` + strconv.Quote(missing) + `, ` + strconv.Quote(to) + `)`, "rename"},
		{`fs.copyFileSync(` + strconv.Quote(missing) + `, ` + strconv.Quote(to) + `)`, "copyfile"},
	} {
		got := caughtError(t, eng, c.call)
		if got["code"] != "ENOENT" {
			t.Errorf("%s: code = %q, want ENOENT (message %q)", c.call, got["code"], got["message"])
			continue
		}
		if got["path"] != missing || got["dest"] != to {
			t.Errorf("%s: path/dest = %q/%q, want %q/%q", c.call, got["path"], got["dest"], missing, to)
		}
		want := "ENOENT: no such file or directory, " + c.syscall + " '" + missing + "' -> '" + to + "'"
		if got["message"] != want {
			t.Errorf("%s: message = %q, want %q", c.call, got["message"], want)
		}
	}
}

// TestFailedMkdtempNamesThePattern pins that a failed mkdtemp reports the pattern
// it tried rather than the prefix it was handed, the way Node does.
func TestFailedMkdtempNamesThePattern(t *testing.T) {
	eng := harness(t)
	prefix := filepath.Join(t.TempDir(), "nope", "run-")
	got := caughtError(t, eng, `fs.mkdtempSync(`+strconv.Quote(prefix)+`)`)
	if got["code"] != "ENOENT" {
		t.Fatalf("code = %q, want ENOENT (message %q)", got["code"], got["message"])
	}
	want := "ENOENT: no such file or directory, mkdtemp '" + prefix + "XXXXXX'"
	if got["message"] != want {
		t.Errorf("message = %q, want %q", got["message"], want)
	}
}

// descOf is libuv's description for a code, written out here so the expected
// message is built from something other than the implementation.
func descOf(t *testing.T, code string) string {
	t.Helper()
	switch code {
	case "ENOENT":
		return "no such file or directory"
	case "EEXIST":
		return "file already exists"
	case "ENOTEMPTY":
		return "directory not empty"
	}
	t.Fatalf("no description written down for %s", code)
	return ""
}

// TestSymlinkEitherWorksOrSaysWhy is the Windows audit of fs.symlinkSync in a
// runnable form.
//
// Creating a symbolic link on Windows needs SeCreateSymbolicLinkPrivilege, which
// an ordinary account does not hold unless Developer Mode is on. Go asks for the
// unprivileged flag first and falls back, so the call succeeds on a developer box
// and fails with ERROR_PRIVILEGE_NOT_HELD on a locked-down one. Node is in
// exactly the same position, so there is nothing here to fix: bento is as capable
// as the runtime it stands in for.
//
// What is worth pinning is the failure being legible. ERROR_PRIVILEGE_NOT_HELD is
// 1314, which matches no POSIX errno, so the old raw comparison reported UNKNOWN
// and a program had no way to tell a missing privilege from a broken path. libuv
// calls it EPERM and so does bento now.
func TestSymlinkEitherWorksOrSaysWhy(t *testing.T) {
	eng := harness(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")

	// caughtError insists on a failure, and half of what this test allows is
	// success, so the call is run here and both outcomes come back as JSON.
	raw := evalString(t, eng, `(function () {
		const fs = require("fs");
		try { fs.symlinkSync(`+strconv.Quote(target)+`, `+strconv.Quote(link)+`); return "{}"; }
		catch (e) { return JSON.stringify({ code: e.code, message: e.message }); }
	})()`)
	var got map[string]string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}

	if got["code"] == "" {
		// The call succeeded, which is the only acceptable outcome off Windows.
		if data, err := os.ReadFile(link); err != nil || string(data) != "x" {
			t.Fatalf("symlink reported success but reading through it gave %q: %v", data, err)
		}
		return
	}
	if runtime.GOOS != "windows" {
		t.Fatalf("symlink failed with %s: %s", got["code"], got["message"])
	}
	if got["code"] != "EPERM" {
		t.Errorf("symlink without the privilege reported %q, want EPERM: %s", got["code"], got["message"])
	}
}
