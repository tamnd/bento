package build

import (
	"runtime"
	"testing"
)

// hostPathIsWin32 is whether require('path') on the machine running this test is the
// win32 variant, which is the choice Node's own module makes and the one the module
// here copies. The tests that call the host module rather than a named variant have
// to expect its answers, and the two variants disagree about ordinary input: a
// backslash separates segments on one and is a filename character on the other.
var hostPathIsWin32 = runtime.GOOS == "windows"

// wantForHost picks between the two variants' expected output. Both strings came from
// real Node, the posix one from the host module and the win32 one from
// require('path').win32, which is the same code Node runs as `path` on Windows.
func wantForHost(posix, win32 string) string {
	if hostPathIsWin32 {
		return win32
	}
	return posix
}

// require('path') answered a throw-on-use stub until this slice, so a compiled
// program that reached for the most-used Node module of all built successfully and
// then failed on its first call. A unit test on the value model cannot show that it
// is fixed for a compiled program, because what changed is what the built binary
// does rather than what the lowerer emits. These build real binaries.
//
// The algorithms themselves are held to real Node output in pkg/value. What these
// check is that a program gets them: that require resolves, that the module carries
// every member, that both variants are reachable, and that a bad argument fails the
// way Node fails it.

// TestRequiringPathAnswersTheRealModule is the reproducer. Every one of these calls
// used to throw "The built-in module 'path' is registered but not implemented".
func TestRequiringPathAnswersTheRealModule(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const path = require('path');\n"+
			"console.log(path.join('a', 'b', '..', 'c'));\n"+
			"console.log(path.dirname('/a/b/c.txt'), path.basename('/a/b/c.txt'), path.extname('/a/b/c.txt'));\n"+
			"console.log(path.basename('/a/b/c.txt', '.txt'));\n"+
			"console.log(path.normalize('a/./b//c/../d'));\n"+
			"console.log(path.isAbsolute('/a'), path.isAbsolute('a'));\n"+
			"console.log(path.relative('/a/b', '/a/c'));\n"+
			// resolve is checked by an identity rather than a literal because the win32
			// half of it prepends the working directory's drive, which is whatever drive
			// the test happens to run from. The posix answer is pinned outright on the
			// next line, where no working directory is involved.
			"console.log(path.resolve('/a', 'b', 'c') === path.resolve('/a/b/c'));\n"+
			"console.log(path.posix.resolve('/a', 'b', 'c'));\n"+
			"console.log(JSON.stringify(path.sep), JSON.stringify(path.delimiter));\n")
	want := wantForHost(
		"a/c\n"+
			"/a/b c.txt .txt\n"+
			"c\n"+
			"a/b/d\n"+
			"true false\n"+
			"../c\n"+
			"true\n"+
			"/a/b/c\n"+
			"\"/\" \":\"\n",
		"a\\c\n"+
			"/a/b c.txt .txt\n"+
			"c\n"+
			"a\\b\\d\n"+
			"true false\n"+
			"..\\c\n"+
			"true\n"+
			"/a/b/c\n"+
			"\"\\\\\" \";\"\n")
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestParseAndFormatWorkInABinary pins the two members the import half of node:path
// has never had, because they take and return objects and the lowerer cannot build
// one at a call site. They are the reason a program reaches for require('path')
// rather than the import: changing a file's extension is parse, edit, format.
func TestParseAndFormatWorkInABinary(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const path = require('path');\n"+
			"const p = path.parse('/home/user/dir/file.txt');\n"+
			"console.log(p.root, p.dir, p.base, p.ext, p.name);\n"+
			"console.log(JSON.stringify(path.parse('.bashrc')));\n"+
			"console.log(path.format({ dir: '/a/b', base: 'f.txt' }));\n"+
			"console.log(path.format({ root: '/', name: 'f', ext: '.js' }));\n"+
			"console.log(path.format({ name: 'f', ext: 'js' }));\n"+
			"p.base = undefined;\n"+
			"p.ext = '.md';\n"+
			"console.log(path.format(p));\n")
	// parse reads the same on both variants here, since a forward slash separates
	// segments on either. format differs, because it joins with the host separator.
	want := wantForHost(
		"/ /home/user/dir file.txt .txt file\n"+
			`{"root":"","dir":"","base":".bashrc","ext":"","name":".bashrc"}`+"\n"+
			"/a/b/f.txt\n"+
			"/f.js\n"+
			"f.js\n"+
			"/home/user/dir/file.md\n",
		"/ /home/user/dir file.txt .txt file\n"+
			`{"root":"","dir":"","base":".bashrc","ext":"","name":".bashrc"}`+"\n"+
			"/a/b\\f.txt\n"+
			"/f.js\n"+
			"f.js\n"+
			"/home/user/dir\\file.md\n")
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestBothPathVariantsAreInABinary pins that a program running on one platform can
// reason about the other's paths. A build tool that writes a Windows manifest from a
// Linux machine needs path.win32 to be the win32 algorithms and not the host's with
// a different separator, and the two disagree on ordinary input: a backslash
// separates segments on win32 and is a filename character on posix.
func TestBothPathVariantsAreInABinary(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const path = require('path');\n"+
			"console.log(JSON.stringify(path.win32.sep), JSON.stringify(path.win32.delimiter));\n"+
			"console.log(path.win32.join('C:\\\\a', 'b', '..', 'c'));\n"+
			"console.log(path.win32.dirname('C:\\\\a\\\\b'), path.posix.dirname('C:\\\\a\\\\b'));\n"+
			"console.log(path.win32.isAbsolute('C:\\\\a'), path.posix.isAbsolute('C:\\\\a'));\n"+
			"console.log(JSON.stringify(path.win32.parse('C:\\\\path\\\\dir\\\\file.txt')));\n"+
			"console.log(path.win32.format({ dir: 'C:\\\\a', base: 'f.txt' }));\n"+
			"console.log(path.posix.join('a', 'b'));\n")
	want := "\"\\\\\" \";\"\n" +
		"C:\\a\\c\n" +
		"C:\\a .\n" +
		"true false\n" +
		`{"root":"C:\\","dir":"C:\\path\\dir","base":"file.txt","ext":".txt","name":"file"}` + "\n" +
		"C:\\a\\f.txt\n" +
		"a/b\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestThePathIdentitiesHoldInABinary pins that the specifiers name modules rather
// than build them. A program requires path more than once, and Node gives it the
// same object every time and the same object as the variant it matches, so a build
// that rebuilt per require would answer every call correctly and still fail a
// program that compared what it got.
func TestThePathIdentitiesHoldInABinary(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const path = require('path');\n"+
			"console.log(path === require('node:path'), path === require('path'));\n"+
			"console.log(require('path/posix') === path.posix, require('node:path/win32') === path.win32);\n"+
			"console.log(path.posix.posix === path.posix, path.win32.posix === path.posix);\n"+
			"console.log(path.posix !== path.win32);\n"+
			// The core module is one of the two variants rather than a third object, and
			// which one it is follows the host, the same choice Node's own module makes.
			"console.log(path === path.win32 ? 'win32' : path === path.posix ? 'posix' : 'neither');\n")
	want := "true true\ntrue true\ntrue true\ntrue\n" + wantForHost("posix\n", "win32\n")
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestEveryPathExportIsThereInABinary walks the whole module from inside a compiled
// program rather than checking a representative few. A program finds a missing
// member at run time, and the point of the slice is that none is missing, so the
// check is that every name Node has reads back as the right kind of value.
func TestEveryPathExportIsThereInABinary(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const path = require('path');\n"+
			"const fns = JSON.parse('[\"resolve\",\"normalize\",\"isAbsolute\",\"join\",\"relative\",\"toNamespacedPath\",\"dirname\",\"basename\",\"extname\",\"format\",\"parse\",\"matchesGlob\",\"_makeLong\"]');\n"+
			"let missing = '';\n"+
			"for (const n of fns) { if (typeof path[n] !== 'function') { missing = missing + n + ' '; } }\n"+
			"console.log(missing === '' ? 'none' : missing);\n"+
			"console.log(typeof path.sep, typeof path.delimiter, typeof path.win32, typeof path.posix);\n"+
			"console.log(Object.keys(path).length);\n"+
			"console.log(path._makeLong === path.toNamespacedPath);\n")
	want := "none\nstring string object object\n17\ntrue\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABadPathArgumentThrowsNodesError pins the refusal end to end. Node does not
// coerce a non-string here, and a bento that answered would turn a caught mistake
// into a wrong path that surfaces somewhere else entirely. The code is printed
// alongside the message because the code is what a program branches on.
func TestABadPathArgumentThrowsNodesError(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const path = require('path');\n"+
			"const bad = JSON.parse('[5, null, {}]');\n"+
			"for (const v of bad) {\n"+
			"  try { path.dirname(v); console.log('no throw'); }\n"+
			"  catch (e) { console.log(e.name, e.code, e.message); }\n"+
			"}\n"+
			"try { path.format('x'); } catch (e) { console.log(e.message); }\n"+
			"console.log(path.basename('a/f.txt', undefined));\n")
	want := "TypeError ERR_INVALID_ARG_TYPE The \"path\" argument must be of type string. Received type number (5)\n" +
		"TypeError ERR_INVALID_ARG_TYPE The \"path\" argument must be of type string. Received null\n" +
		"TypeError ERR_INVALID_ARG_TYPE The \"path\" argument must be of type string. Received an instance of Object\n" +
		"The \"pathObject\" argument must be of type object. Received type string ('x')\n" +
		"f.txt\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestMatchesGlobRefusesInABinary pins the one member that is present and not
// implemented. Glob matching is a matcher of its own, and a partial one would answer
// false for a pattern it did not understand, which is exactly the quiet wrong answer
// the rest of this module is arranged to avoid. Refusing by name is what tells a
// program's author that bento is missing this rather than that their pattern is
// wrong.
func TestMatchesGlobRefusesInABinary(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const path = require('path');\n"+
			"try { path.matchesGlob('a/b.js', 'a/*.js'); console.log('answered'); }\n"+
			"catch (e) { console.log(e.code, e.message); }\n")
	if want := "ERR_NOT_IMPLEMENTED path.matchesGlob is not implemented in bento yet\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
