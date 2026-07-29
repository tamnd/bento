package value

import (
	"strings"
	"testing"
)

// require('path') used to hand back a throw-on-use stub, so a program that reached
// for the single most-used Node module compiled and then failed on its first call.
// These cases pin the module: that every export Node has is there, that both
// variants are, that the identities between them hold, and that each function
// answers what the ported algorithm answers rather than what a miswired module
// object would.
//
// The functions themselves are checked against real Node output in nodepath_test.go.
// What is checked here is the module: that resolve is wired to resolve and not to
// normalize, that win32's members run the win32 algorithms, and that a bad argument
// is refused the way Node refuses it.

// nodePathExports is every own property Node reports for require('path'), in the
// order Object.keys gives them on v24.18.0. Keeping the list here rather than
// deriving it from the builder is what makes it a check: a member dropped from the
// builder fails against Node's list rather than against a copy of itself.
var nodePathExports = []string{
	"resolve", "normalize", "isAbsolute", "join", "relative", "toNamespacedPath",
	"dirname", "basename", "extname", "format", "parse", "matchesGlob",
	"sep", "delimiter", "win32", "posix", "_makeLong",
}

// pathMod is the module under test, read through require the way a program gets it.
func pathMod(t *testing.T, specifier string) Value {
	t.Helper()
	m := RequireBuiltin(specifier)
	if m.Kind() != KindObject {
		t.Fatalf("require(%q) answered %v, want an object", specifier, m.Kind())
	}
	return m
}

// callPath reads a member off the module and calls it, the two steps a program's
// path.join(a, b) takes. A member that is missing fails at the call rather than at
// the read, which is the failure the stub used to produce, so both steps run.
func callPath(t *testing.T, mod Value, name string, args ...Value) Value {
	t.Helper()
	f := mod.Get(FromGoString(name))
	if f.Kind() != KindFunc {
		t.Fatalf("path.%s is %v, want a function", name, f.Kind())
	}
	return f.Call(args...)
}

// pstr builds a string argument, which is what nearly every path function takes.
func pstr(s string) Value { return StringValue(FromGoString(s)) }

// TestPathModuleHasEveryNodeExport pins the whole surface in both directions:
// nothing Node has is missing, and nothing bento invented is there. The order is
// pinned too, since Object.keys reports it and a program that prints the module
// should print what Node prints.
func TestPathModuleHasEveryNodeExport(t *testing.T) {
	for _, specifier := range []string{"path", "path/posix", "path/win32"} {
		mod := pathMod(t, specifier)
		keys := mod.OwnKeys()
		n := int(keys.Len())
		got := make([]string, n)
		for i := 0; i < n; i++ {
			got[i] = keys.AtI(i).ToGoString()
		}
		if strings.Join(got, ",") != strings.Join(nodePathExports, ",") {
			t.Errorf("require(%q) keys are\n%v\nwant\n%v", specifier, got, nodePathExports)
		}
	}
}

// TestThePathModuleIsTheHostVariant pins which of the two variants require('path')
// answers with. It is not a copy of one of them, it is one of them: Node's path
// module is the host's variant with the other hung off it, so path.posix === path
// holds on a posix machine, and a program that compares them sees that.
func TestThePathModuleIsTheHostVariant(t *testing.T) {
	mod := pathMod(t, "path")
	wantSep, wantSelf := "/", "posix"
	if onWindows {
		wantSep, wantSelf = `\`, "win32"
	}
	if got := ToString(mod.Get(FromGoString("sep"))).ToGoString(); got != wantSep {
		t.Errorf("path.sep = %q, want %q on this platform", got, wantSep)
	}
	if !StrictEquals(mod, mod.Get(FromGoString(wantSelf))) {
		t.Errorf("path is not path.%s, which is the identity Node has", wantSelf)
	}
}

// TestBothPathVariantsAreReachable pins the reason for building two: a program
// running on Linux that has to reason about a Windows path reaches path.win32 and
// gets the win32 algorithms, not the host's with a different separator glued on.
func TestBothPathVariantsAreReachable(t *testing.T) {
	mod := pathMod(t, "path")
	win32 := mod.Get(FromGoString("win32"))
	posix := mod.Get(FromGoString("posix"))
	if got := ToString(win32.Get(FromGoString("sep"))).ToGoString(); got != `\` {
		t.Errorf("path.win32.sep = %q, want a backslash", got)
	}
	if got := ToString(posix.Get(FromGoString("sep"))).ToGoString(); got != "/" {
		t.Errorf("path.posix.sep = %q, want a slash", got)
	}
	if got := ToString(win32.Get(FromGoString("delimiter"))).ToGoString(); got != ";" {
		t.Errorf("path.win32.delimiter = %q, want a semicolon", got)
	}
	// The separator is the visible difference; these are the algorithmic ones. A
	// backslash is a separator on win32 and an ordinary filename character on posix,
	// and a drive letter is a root only on win32.
	if got := ToString(callPath(t, win32, "dirname", pstr(`C:\a\b`))).ToGoString(); got != `C:\a` {
		t.Errorf(`win32.dirname("C:\a\b") = %q, want "C:\a"`, got)
	}
	if got := ToString(callPath(t, posix, "dirname", pstr(`C:\a\b`))).ToGoString(); got != "." {
		t.Errorf(`posix.dirname("C:\a\b") = %q, want "."`, got)
	}
	if ToBoolean(callPath(t, posix, "isAbsolute", pstr(`C:\a`))) {
		t.Error(`posix.isAbsolute("C:\a") is true, want false: posix has no drive roots`)
	}
	if !ToBoolean(callPath(t, win32, "isAbsolute", pstr(`C:\a`))) {
		t.Error(`win32.isAbsolute("C:\a") is false, want true`)
	}
}

// TestEverySpecifierNamesTheSameTwoModules pins the identities. A program that
// requires path twice, or requires it under both spellings, compares what it got,
// and a module rebuilt per specifier would fail that comparison while answering
// every call correctly, which is the kind of bug a functional test misses.
func TestEverySpecifierNamesTheSameTwoModules(t *testing.T) {
	posix := pathMod(t, "path/posix")
	win32 := pathMod(t, "path/win32")
	for _, c := range []struct {
		a, b string
		want Value
	}{
		{"path/posix", "node:path/posix", posix},
		{"path/win32", "node:path/win32", win32},
		{"path", "node:path", pathMod(t, "path")},
	} {
		if !StrictEquals(pathMod(t, c.a), pathMod(t, c.b)) {
			t.Errorf("require(%q) is not require(%q)", c.a, c.b)
		}
	}
	if !StrictEquals(posix.Get(FromGoString("posix")), posix) {
		t.Error("path.posix.posix is not path.posix")
	}
	if !StrictEquals(posix.Get(FromGoString("win32")), win32) {
		t.Error("path.posix.win32 is not path.win32")
	}
	if !StrictEquals(win32.Get(FromGoString("posix")), posix) {
		t.Error("path.win32.posix is not path.posix")
	}
	if StrictEquals(posix, win32) {
		t.Error("the two variants are one object, so one of them runs the wrong algorithms")
	}
}

// TestTheModuleAnswersWhatThePortAnswers drives every member of both variants over
// the same path list the port is checked against and compares the module's answer to
// the algorithm's. That is what catches a crossed wire: a module whose extname was
// bound to basename answers strings for every call and passes any test that only
// asks whether it answered.
//
// resolve, relative and toNamespacedPath read the working directory, so they are
// compared to the port driven with the same directory rather than to the reference,
// which was generated with a stub one.
func TestTheModuleAnswersWhatThePortAnswers(t *testing.T) {
	ref := loadNodePathRef(t)
	for _, v := range []struct {
		specifier string
		flavor    pathFlavor
		cases     map[string]nodeParsedPath
	}{
		{"path/posix", posixPathFlavor(), ref.Posix.Parse},
		{"path/win32", win32PathFlavor(), ref.Win32.Parse},
	} {
		mod := pathMod(t, v.specifier)
		for in := range v.cases {
			p := pstr(in)
			for name, want := range map[string]string{
				"normalize":        v.flavor.normalize(in),
				"dirname":          v.flavor.dirname(in),
				"basename":         v.flavor.basename(in, ""),
				"extname":          v.flavor.extname(in),
				"resolve":          v.flavor.resolve([]string{in}),
				"toNamespacedPath": v.flavor.toNamespaced(in),
			} {
				if got := ToString(callPath(t, mod, name, p)).ToGoString(); got != want {
					t.Errorf("%s.%s(%q) = %q, the port says %q", v.specifier, name, in, got, want)
				}
			}
			if got, want := ToBoolean(callPath(t, mod, "isAbsolute", p)), v.flavor.isAbsolute(in); got != want {
				t.Errorf("%s.isAbsolute(%q) = %v, the port says %v", v.specifier, in, got, want)
			}
			if got, want := ToString(callPath(t, mod, "join", p, pstr("b"))).ToGoString(), v.flavor.join([]string{in, "b"}); got != want {
				t.Errorf("%s.join(%q, \"b\") = %q, the port says %q", v.specifier, in, got, want)
			}
			if got, want := ToString(callPath(t, mod, "relative", p, pstr("b"))).ToGoString(), v.flavor.relative(in, "b"); got != want {
				t.Errorf("%s.relative(%q, \"b\") = %q, the port says %q", v.specifier, in, got, want)
			}
			if got, want := ToString(callPath(t, mod, "basename", p, pstr(".txt"))).ToGoString(), v.flavor.basename(in, ".txt"); got != want {
				t.Errorf("%s.basename(%q, \".txt\") = %q, the port says %q", v.specifier, in, got, want)
			}
		}
	}
}

// TestParseAnswersAllFiveFields pins the object parse hands back over every case in
// the reference. All five keys are always present even when empty, so a program that
// reads .ext off a path with no extension gets the empty string rather than
// undefined, which is what it would then concatenate into a filename.
func TestParseAnswersAllFiveFields(t *testing.T) {
	ref := loadNodePathRef(t)
	for _, v := range []struct {
		specifier string
		cases     map[string]nodeParsedPath
	}{
		{"path/posix", ref.Posix.Parse},
		{"path/win32", ref.Win32.Parse},
	} {
		mod := pathMod(t, v.specifier)
		for in, want := range v.cases {
			got := callPath(t, mod, "parse", pstr(in))
			if got.Kind() != KindObject {
				t.Fatalf("%s.parse(%q) answered %v, want an object", v.specifier, in, got.Kind())
			}
			for field, wantField := range map[string]string{
				"root": want.Root, "dir": want.Dir, "base": want.Base,
				"ext": want.Ext, "name": want.Name,
			} {
				f := got.Get(FromGoString(field))
				if f.Kind() != KindString {
					t.Errorf("%s.parse(%q).%s is %v, want a string even when empty", v.specifier, in, field, f.Kind())
					continue
				}
				if f.AsString().ToGoString() != wantField {
					t.Errorf("%s.parse(%q).%s = %q, node says %q", v.specifier, in, field, f.AsString().ToGoString(), wantField)
				}
			}
		}
	}
}

// TestFormatReadsTheObjectsFields pins format through the module, including the two
// precedence rules a program relies on without stating them: base wins over the
// name and ext pair, and dir wins over root. Those are what let a program parse a
// path, change one field, and format it back without clearing another.
func TestFormatReadsTheObjectsFields(t *testing.T) {
	mod := pathMod(t, "path/posix")
	obj := func(kv ...string) Value {
		o := NewObject()
		for i := 0; i < len(kv); i += 2 {
			o.Set(FromGoString(kv[i]), pstr(kv[i+1]))
		}
		return o
	}
	for _, c := range []struct {
		in   Value
		want string
	}{
		{obj(), ""},
		{obj("dir", "/a", "base", "f.txt"), "/a/f.txt"},
		{obj("dir", "/a", "name", "f", "ext", ".txt"), "/a/f.txt"},
		{obj("dir", "/a", "name", "f", "ext", "txt"), "/a/f.txt"},
		{obj("root", "/", "base", "f.txt"), "/f.txt"},
		{obj("dir", "/a", "root", "/", "base", "f.txt"), "/a/f.txt"},
		{obj("dir", "/a", "base", "b.txt", "name", "f", "ext", ".js"), "/a/b.txt"},
	} {
		if got := ToString(callPath(t, mod, "format", c.in)).ToGoString(); got != c.want {
			t.Errorf("format answered %q, want %q", got, c.want)
		}
	}
	// A round trip through the module is what a program actually writes when it
	// changes a file's extension, so it is pinned as one call rather than two facts.
	p := callPath(t, mod, "parse", pstr("/a/b/c.txt"))
	p.Set(FromGoString("base"), Undefined)
	p.Set(FromGoString("ext"), pstr(".js"))
	if got := ToString(callPath(t, mod, "format", p)).ToGoString(); got != "/a/b/c.js" {
		t.Errorf("changing the extension gave %q, want /a/b/c.js", got)
	}
}

// TestAPathFunctionRefusesANonString pins the refusal. Node does not coerce here,
// and that is the load-bearing part: path.dirname(5) throwing is a caught mistake,
// while answering "." for it is a wrong path that surfaces somewhere else later. The
// code is checked alongside the message because a program branches on the code.
func TestAPathFunctionRefusesANonString(t *testing.T) {
	mod := pathMod(t, "path/posix")
	for _, c := range []struct {
		name    string
		args    []Value
		message string
	}{
		{"dirname", []Value{Number(5)}, `The "path" argument must be of type string. Received type number (5)`},
		{"normalize", []Value{Null}, `The "path" argument must be of type string. Received null`},
		{"extname", []Value{Undefined}, `The "path" argument must be of type string. Received undefined`},
		{"basename", []Value{pstr("a"), Number(5)}, `The "suffix" argument must be of type string. Received type number (5)`},
		{"join", []Value{pstr("a"), True}, `The "path" argument must be of type string. Received type boolean (true)`},
		{"relative", []Value{Number(1), pstr("b")}, `The "from" argument must be of type string. Received type number (1)`},
		{"relative", []Value{pstr("a"), Number(2)}, `The "to" argument must be of type string. Received type number (2)`},
		{"format", []Value{pstr("x")}, `The "pathObject" argument must be of type object. Received type string ('x')`},
		{"parse", []Value{NewArrayValue(nil)}, `The "path" argument must be of type string. Received an instance of Array`},
		{"isAbsolute", []Value{NewObject()}, `The "path" argument must be of type string. Received an instance of Object`},
	} {
		t.Run(c.name+"/"+c.message, func(t *testing.T) {
			defer func() {
				rec := recover()
				if rec == nil {
					t.Fatalf("path.%s did not throw on a bad argument", c.name)
				}
				e, ok := rec.(*Error)
				if !ok {
					t.Fatalf("panicked with %T, want a thrown JavaScript error", rec)
				}
				if e.ErrorName() != "TypeError" || e.ErrorMessage() != c.message {
					t.Errorf("threw %s: %s\nwant TypeError: %s", e.ErrorName(), e.ErrorMessage(), c.message)
				}
				code, has := e.Code()
				if !has || code.ToGoString() != "ERR_INVALID_ARG_TYPE" {
					t.Errorf("thrown code is %q (present %v), want ERR_INVALID_ARG_TYPE", code.ToGoString(), has)
				}
			}()
			callPath(t, mod, c.name, c.args...)
		})
	}
}

// TestBasenameSuffixStaysOptional pins the one argument Node checks only when it is
// present. basename(p) and basename(p, undefined) are the same call there, because
// the second is what a forwarded optional argument looks like, and refusing it would
// break every wrapper.
func TestBasenameSuffixStaysOptional(t *testing.T) {
	mod := pathMod(t, "path/posix")
	for _, args := range [][]Value{{pstr("a/f.txt")}, {pstr("a/f.txt"), Undefined}} {
		if got := ToString(callPath(t, mod, "basename", args...)).ToGoString(); got != "f.txt" {
			t.Errorf("basename with %d arguments = %q, want f.txt", len(args), got)
		}
	}
	if got := ToString(callPath(t, mod, "basename", pstr("a/f.txt"), pstr(".txt"))).ToGoString(); got != "f" {
		t.Errorf("basename with a suffix = %q, want f", got)
	}
}

// TestToNamespacedPathHandsBackANonString pins the one member that does not refuse.
// Node lets anything that is not a string through untouched, because it is called on
// paths that have already been through the module and a throw here would report a
// failure that was already raised somewhere more useful.
func TestToNamespacedPathHandsBackANonString(t *testing.T) {
	mod := pathMod(t, "path/win32")
	if got := callPath(t, mod, "toNamespacedPath", Number(5)); got.Kind() != KindNumber || got.AsNumber() != 5 {
		t.Errorf("toNamespacedPath(5) = %v, want the 5 back", got.Kind())
	}
	if got := callPath(t, mod, "toNamespacedPath", Undefined); got.Kind() != KindUndefined {
		t.Errorf("toNamespacedPath(undefined) = %v, want undefined back", got.Kind())
	}
}

// TestMakeLongIsToNamespacedPath pins the alias. Node keeps _makeLong as the same
// function object rather than a second one wrapping it, and published packages still
// call it, so it is here and it is the same object.
func TestMakeLongIsToNamespacedPath(t *testing.T) {
	mod := pathMod(t, "path/win32")
	if !StrictEquals(mod.Get(FromGoString("_makeLong")), mod.Get(FromGoString("toNamespacedPath"))) {
		t.Error("path._makeLong is not path.toNamespacedPath")
	}
}

// TestMatchesGlobRefusesRatherThanAnswering pins the one member that is present but
// not implemented. Glob matching is a matcher of its own and a partial one would
// answer false for a pattern it did not understand, which is the quiet wrong answer
// this module is otherwise arranged to avoid. Leaving the member out entirely would
// be worse still: the program would call undefined and get a message about that
// rather than about globs.
func TestMatchesGlobRefusesRatherThanAnswering(t *testing.T) {
	mod := pathMod(t, "path/posix")
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("matchesGlob answered, but it is not implemented")
		}
		e, ok := rec.(*Error)
		if !ok {
			t.Fatalf("panicked with %T, want a thrown JavaScript error", rec)
		}
		if !strings.Contains(e.ErrorMessage(), "matchesGlob") {
			t.Errorf("threw %q, want a message naming matchesGlob", e.ErrorMessage())
		}
		if code, has := e.Code(); !has || code.ToGoString() != "ERR_NOT_IMPLEMENTED" {
			t.Errorf("thrown code is %q, want ERR_NOT_IMPLEMENTED", code.ToGoString())
		}
	}()
	callPath(t, mod, "matchesGlob", pstr("a/b.js"), pstr("a/*.js"))
}

// TestMatchesGlobStillChecksItsArguments pins that the refusal comes after the
// validation. A program that passed the wrong type learns that first, because that
// is a mistake in the program rather than a gap in bento.
func TestMatchesGlobStillChecksItsArguments(t *testing.T) {
	mod := pathMod(t, "path/posix")
	defer func() {
		rec := recover()
		e, ok := rec.(*Error)
		if !ok {
			t.Fatalf("panicked with %T, want a thrown JavaScript error", rec)
		}
		if e.ErrorName() != "TypeError" || !strings.Contains(e.ErrorMessage(), `"pattern"`) {
			t.Errorf("threw %s: %s, want a TypeError naming the pattern argument", e.ErrorName(), e.ErrorMessage())
		}
	}()
	callPath(t, mod, "matchesGlob", pstr("a"), Number(5))
}

// TestPathIsNoLongerAStub pins the thing this slice changed. The stub answered a
// live value for require and threw on the first member read, so a test that only
// required the module could not tell the two apart.
func TestPathIsNoLongerAStub(t *testing.T) {
	for _, specifier := range []string{"path", "node:path", "path/posix", "path/win32"} {
		mod := pathMod(t, specifier)
		if got := ToString(mod.Get(FromGoString("join")).Call(pstr("a"), pstr("b"))).ToGoString(); got == "" {
			t.Errorf("require(%q).join answered nothing", specifier)
		}
	}
}
