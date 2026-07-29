package value

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The path port is checked against Node itself rather than against a hand-written
// expectation, because the whole point of porting the algorithms instead of
// delegating to path/filepath is that Node's answers are the specification.
// testdata/nodepath_node24.json is what a real Node prints for every case below,
// generated once and checked in, so this test is a comparison against the runtime
// bento claims compatibility with and not against my reading of it.
//
// Both variants run on whatever machine the test runs on. Node exposes path.posix
// and path.win32 everywhere, so both columns of the reference are real Node
// output, and the port is held to both on Linux, macOS and Windows alike. That is
// the only way a Windows path bug fails on a Linux developer's machine.

// nodePathRef is the shape of the reference file.
type nodePathRef struct {
	NodeVersion string `json:"nodeVersion"`
	Posix       nodePathVariantRef
	Win32       nodePathVariantRef
}

type nodePathVariantRef struct {
	// Cwd is the working directory the reference was generated with. resolve and
	// relative read it, so the port is driven with the same one and their answers
	// are comparable on any machine.
	Cwd   string `json:"cwd"`
	Unary map[string]struct {
		Normalize        string `json:"normalize"`
		Dirname          string `json:"dirname"`
		Basename         string `json:"basename"`
		Extname          string `json:"extname"`
		IsAbsolute       bool   `json:"isAbsolute"`
		ToNamespacedPath string `json:"toNamespacedPath"`
	} `json:"unary"`
	BasenameExt map[string]string `json:"basenameExt"`
	Join        map[string]string `json:"join"`
	Relative    map[string]string `json:"relative"`
	Resolve     map[string]string `json:"resolve"`
	Sep         string            `json:"sep"`
	Delimiter   string            `json:"delimiter"`
}

// loadNodePathRef reads the checked-in Node output.
func loadNodePathRef(t *testing.T) nodePathRef {
	t.Helper()
	data, err := os.ReadFile("testdata/nodepath_node24.json")
	if err != nil {
		t.Fatalf("read reference: %v", err)
	}
	var ref nodePathRef
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatalf("parse reference: %v", err)
	}
	if len(ref.Posix.Unary) == 0 || len(ref.Win32.Unary) == 0 {
		t.Fatal("reference has no cases")
	}
	if ref.Posix.Cwd == "" || ref.Win32.Cwd == "" {
		t.Fatal("reference records no working directory, so resolve cannot be compared")
	}
	return ref
}

// decodeArgs reads the JSON array the reference uses as a key for resolve, whose
// cases have no fixed length.
func decodeArgs(t *testing.T, key string) []string {
	t.Helper()
	var args []string
	if err := json.Unmarshal([]byte(key), &args); err != nil {
		t.Fatalf("bad reference key %q: %v", key, err)
	}
	return args
}

// noDriveCwd stands in for the per-drive working directories Windows keeps. The
// reference was generated with them cleared, so the port is given none either.
func noDriveCwd(string) string { return "" }

// decodePair reads the two-element JSON array the reference uses as a key for the
// binary cases, so the test drives the port with exactly the arguments Node saw.
func decodePair(t *testing.T, key string) (string, string) {
	t.Helper()
	var pair []string
	if err := json.Unmarshal([]byte(key), &pair); err != nil || len(pair) != 2 {
		t.Fatalf("bad reference key %q: %v", key, err)
	}
	return pair[0], pair[1]
}

// TestPathPortMatchesNodePosix holds the posix variant to Node's answers.
func TestPathPortMatchesNodePosix(t *testing.T) {
	ref := loadNodePathRef(t)
	for in, want := range ref.Posix.Unary {
		if got := pathNormalizePosix(in); got != want.Normalize {
			t.Errorf("normalize(%q) = %q, node says %q", in, got, want.Normalize)
		}
		if got := pathDirnamePosix(in); got != want.Dirname {
			t.Errorf("dirname(%q) = %q, node says %q", in, got, want.Dirname)
		}
		if got := pathBasename(in, "", isPathSeparatorPosix, false); got != want.Basename {
			t.Errorf("basename(%q) = %q, node says %q", in, got, want.Basename)
		}
		if got := pathExtname(in, isPathSeparatorPosix, false); got != want.Extname {
			t.Errorf("extname(%q) = %q, node says %q", in, got, want.Extname)
		}
		if got := pathIsAbsolutePosix(in); got != want.IsAbsolute {
			t.Errorf("isAbsolute(%q) = %v, node says %v", in, got, want.IsAbsolute)
		}
		// posix has no namespaced form, so Node hands the path straight back. The
		// port says so out loud rather than leaving the function out.
		if got := pathToNamespacedPathPosix(in); got != want.ToNamespacedPath {
			t.Errorf("toNamespacedPath(%q) = %q, node says %q", in, got, want.ToNamespacedPath)
		}
	}
	for key, want := range ref.Posix.BasenameExt {
		p, ext := decodePair(t, key)
		if got := pathBasename(p, ext, isPathSeparatorPosix, false); got != want {
			t.Errorf("basename(%q, %q) = %q, node says %q", p, ext, got, want)
		}
	}
	for key, want := range ref.Posix.Join {
		a, b := decodePair(t, key)
		if got := pathJoinPosix([]string{a, b}); got != want {
			t.Errorf("join(%q, %q) = %q, node says %q", a, b, got, want)
		}
	}
}

// TestPathPortMatchesNodeWin32 holds the win32 variant to Node's answers, on
// every platform. The reference was generated through path.win32, which is the
// same code Node runs on Windows.
func TestPathPortMatchesNodeWin32(t *testing.T) {
	ref := loadNodePathRef(t)
	for in, want := range ref.Win32.Unary {
		if got := pathNormalizeWin32(in); got != want.Normalize {
			t.Errorf("normalize(%q) = %q, node says %q", in, got, want.Normalize)
		}
		if got := pathDirnameWin32(in); got != want.Dirname {
			t.Errorf("dirname(%q) = %q, node says %q", in, got, want.Dirname)
		}
		if got := pathBasename(in, "", isPathSeparatorWin32, true); got != want.Basename {
			t.Errorf("basename(%q) = %q, node says %q", in, got, want.Basename)
		}
		if got := pathExtname(in, isPathSeparatorWin32, true); got != want.Extname {
			t.Errorf("extname(%q) = %q, node says %q", in, got, want.Extname)
		}
		if got := pathIsAbsoluteWin32(in); got != want.IsAbsolute {
			t.Errorf("isAbsolute(%q) = %v, node says %v", in, got, want.IsAbsolute)
		}
		got := pathToNamespacedPathWin32(in, ref.Win32.Cwd, noDriveCwd)
		if got != want.ToNamespacedPath {
			t.Errorf("toNamespacedPath(%q) = %q, node says %q", in, got, want.ToNamespacedPath)
		}
	}
	for key, want := range ref.Win32.BasenameExt {
		p, ext := decodePair(t, key)
		if got := pathBasename(p, ext, isPathSeparatorWin32, true); got != want {
			t.Errorf("basename(%q, %q) = %q, node says %q", p, ext, got, want)
		}
	}
	for key, want := range ref.Win32.Join {
		a, b := decodePair(t, key)
		if got := pathJoinWin32([]string{a, b}); got != want {
			t.Errorf("join(%q, %q) = %q, node says %q", a, b, got, want)
		}
	}
}

// TestPathRelativeMatchesNode holds relative to Node's answers. relative
// resolves both of its arguments against the working directory before comparing
// them, so the port is driven with the directory the reference was generated
// with rather than with this process's.
func TestPathRelativeMatchesNode(t *testing.T) {
	ref := loadNodePathRef(t)
	for key, want := range ref.Posix.Relative {
		from, to := decodePair(t, key)
		if got := pathRelativePosix(from, to, ref.Posix.Cwd); got != want {
			t.Errorf("posix relative(%q, %q) = %q, node says %q", from, to, got, want)
		}
	}
	for key, want := range ref.Win32.Relative {
		from, to := decodePair(t, key)
		if got := pathRelativeWin32(from, to, ref.Win32.Cwd, noDriveCwd); got != want {
			t.Errorf("win32 relative(%q, %q) = %q, node says %q", from, to, got, want)
		}
	}
}

// TestPathResolveMatchesNode holds resolve to Node's answers, including the
// argument lists that reach the working directory and the ones that name a drive
// without rooting it, which is the case Windows answers from a directory it keeps
// per drive.
func TestPathResolveMatchesNode(t *testing.T) {
	ref := loadNodePathRef(t)
	for key, want := range ref.Posix.Resolve {
		args := decodeArgs(t, key)
		if got := pathResolvePosix(args, ref.Posix.Cwd); got != want {
			t.Errorf("posix resolve(%q) = %q, node says %q", args, got, want)
		}
	}
	for key, want := range ref.Win32.Resolve {
		args := decodeArgs(t, key)
		if got := pathResolveWin32(args, ref.Win32.Cwd, noDriveCwd); got != want {
			t.Errorf("win32 resolve(%q) = %q, node says %q", args, got, want)
		}
	}
}

// TestPathRelativeWhereLowercasingChangesLength covers the branch the table
// cannot reach, because no path in it holds a character whose lowercase is longer
// than itself. Turkish dotted capital I is one: it lowercases to two characters,
// which moves every index after it, so Node compares those paths segment by
// segment rather than character by character. The answers are Node's own.
func TestPathRelativeWhereLowercasingChangesLength(t *testing.T) {
	const dotted = "\u0130"
	cases := []struct {
		from, to string
		want     string
	}{
		{`C:\` + dotted, `C:\` + dotted + `\a`, "a"},
		{`C:\` + dotted + `\a`, `C:\` + dotted, ".."},
		{`C:\` + dotted + `\a`, `C:\b`, `..\..\b`},
		{`C:\a\` + dotted, `C:\a\` + dotted + `\b`, "b"},
		{`C:\` + dotted, `D:\` + dotted, `D:\` + dotted},
	}
	for _, tc := range cases {
		if got := pathRelativeWin32(tc.from, tc.to, `C:\cwd`, noDriveCwd); got != tc.want {
			t.Errorf("relative(%q, %q) = %q, node says %q", tc.from, tc.to, got, tc.want)
		}
	}
}

// TestPathResolveReadsTheDriveWorkingDirectory covers the one input the reference
// cannot carry: Windows keeps a working directory per drive, and Node reads it out
// of the environment. A path that names a drive without rooting it, D:a, resolves
// against that directory, and against the drive's root when there is none.
//
// The two cases with no directory for the drive are Node's own answers, taken the
// same way as the reference. The first case is not, because the variable Node
// reads is named "=D:" and a machine with no drives will not let a name with an
// equals sign in it into the environment. It is read off Node's resolve instead,
// which falls through to the drive's directory when it names the same drive.
func TestPathResolveReadsTheDriveWorkingDirectory(t *testing.T) {
	drives := func(device string) string {
		if strings.EqualFold(device, "D:") {
			return `D:\work`
		}
		return ""
	}
	if got := pathResolveWin32([]string{"D:a"}, `C:\cwd`, drives); got != `D:\work\a` {
		t.Errorf("resolve(D:a) = %q, want D:\\work\\a when D: is at D:\\work", got)
	}
	if got := pathResolveWin32([]string{"E:a"}, `C:\cwd`, drives); got != `E:\a` {
		t.Errorf("resolve(E:a) = %q, want E:\\a when E: has no directory of its own", got)
	}
	if got := pathResolveWin32([]string{"C:a"}, `C:\cwd`, drives); got != `C:\cwd\a` {
		t.Errorf("resolve(C:a) = %q, want C:\\cwd\\a, the process directory being on C:", got)
	}
}

// TestPathSepAndDelimiterMatchNode pins the two constants against the reference,
// since a program that builds a path by hand reads them.
func TestPathSepAndDelimiterMatchNode(t *testing.T) {
	ref := loadNodePathRef(t)
	want := ref.Posix
	if onWindows {
		want = ref.Win32
	}
	if got := PathSep().ToGoString(); got != want.Sep {
		t.Errorf("sep = %q, node says %q for this platform", got, want.Sep)
	}
	if got := PathDelimiter().ToGoString(); got != want.Delimiter {
		t.Errorf("delimiter = %q, node says %q for this platform", got, want.Delimiter)
	}
}

// TestPathExportedHelpersFollowThePlatform proves the exported surface routes to
// the variant the host calls for, which is what makes a compiled program's answer
// the one Node would give on that machine.
func TestPathExportedHelpersFollowThePlatform(t *testing.T) {
	joined := PathJoin(FromGoString("a"), FromGoString("b")).ToGoString()
	want := "a/b"
	if onWindows {
		want = `a\b`
	}
	if joined != want {
		t.Errorf("PathJoin = %q, want %q on this platform", joined, want)
	}
	if got := PathBasename(FromGoString("a/b/c.txt")).ToGoString(); got != "c.txt" {
		t.Errorf("PathBasename = %q, want c.txt", got)
	}
	if got := PathBasenameSuffix(FromGoString("a/b/c.txt"), FromGoString(".txt")).ToGoString(); got != "c" {
		t.Errorf("PathBasenameSuffix = %q, want c", got)
	}
	if got := PathExtname(FromGoString("a/b/c.txt")).ToGoString(); got != ".txt" {
		t.Errorf("PathExtname = %q, want .txt", got)
	}
	if got := PathDirname(FromGoString("a/b/c.txt")).ToGoString(); got != "a/b" {
		t.Errorf("PathDirname = %q, want a/b", got)
	}
	if got := PathNormalize(FromGoString("a/./b/../c")).ToGoString(); got != want[:1]+"c" && got != "a/c" && got != `a\c` {
		t.Errorf("PathNormalize = %q, want a/c or a\\c", got)
	}
}
