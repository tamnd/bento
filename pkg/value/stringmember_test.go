package value

import "testing"

// callStringMethod reads a member off a boxed string and invokes it, the two steps a
// dynamic s.slice(1) takes: Get answers the bound method and Call runs it.
func callStringMethod(t *testing.T, s string, name string, args ...Value) Value {
	t.Helper()
	recv := StringValue(FromGoString(s))
	m := recv.Get(FromGoString(name))
	if m.kind != KindFunc {
		t.Fatalf("%q.%s read %v, want a callable", s, name, m)
	}
	return m.Call(args...)
}

// wantString runs a dynamic string method and compares its result rendered as a string,
// which covers a string result directly and a number or boolean one through the same
// coercion a print of it would take.
func wantString(t *testing.T, s, name string, args []Value, want string) {
	t.Helper()
	got := ToString(callStringMethod(t, s, name, args...)).ToGoString()
	if got != want {
		t.Errorf("dynamic %q.%s(%v) = %q, want %q", s, name, args, got, want)
	}
}

// TestStringMethodsOnADynamicReceiver pins the surface this file adds: every method the
// statically typed path lowers is reachable through a dynamic Get, and each answers what
// the lowered call answers. Before this the read found nothing and the call threw
// "undefined is not a function", so a program holding a string in an any-typed slot
// compiled and could not run.
func TestStringMethodsOnADynamicReceiver(t *testing.T) {
	cases := []struct {
		recv string
		name string
		args []Value
		want string
	}{
		{"Hello", "toUpperCase", nil, "HELLO"},
		{"Hello", "toLowerCase", nil, "hello"},
		{"Hello", "toString", nil, "Hello"},
		{"Hello", "valueOf", nil, "Hello"},
		{"Hello", "at", []Value{Number(-1)}, "o"},
		{"Hello", "charAt", []Value{Number(1)}, "e"},
		{"Hello", "charCodeAt", []Value{Number(0)}, "72"},
		{"Hello", "codePointAt", []Value{Number(0)}, "72"},
		{"Hello", "indexOf", []Value{str("l")}, "2"},
		{"Hello", "indexOf", []Value{str("l"), Number(3)}, "3"},
		{"Hello", "lastIndexOf", []Value{str("l")}, "3"},
		{"Hello", "includes", []Value{str("ell")}, "true"},
		{"Hello", "startsWith", []Value{str("He")}, "true"},
		{"Hello", "endsWith", []Value{str("lo")}, "true"},
		{"Hello", "slice", []Value{Number(1)}, "ello"},
		{"Hello", "slice", []Value{Number(1), Number(3)}, "el"},
		{"Hello", "substring", []Value{Number(3), Number(1)}, "el"},
		{"Hello", "substr", []Value{Number(1), Number(2)}, "el"},
		{"  hi  ", "trim", nil, "hi"},
		{"  hi", "trimStart", nil, "hi"},
		{"hi  ", "trimEnd", nil, "hi"},
		{"ab", "repeat", []Value{Number(3)}, "ababab"},
		{"ab", "concat", []Value{str("c"), str("d")}, "abcd"},
		{"7", "padStart", []Value{Number(3), str("0")}, "007"},
		{"7", "padEnd", []Value{Number(3), str("0")}, "700"},
		{"ab", "normalize", nil, "ab"},
		{"ab", "isWellFormed", nil, "true"},
		{"ab", "toWellFormed", nil, "ab"},
		{"a,b", "replace", []Value{str(","), str("-")}, "a-b"},
		{"a,b,c", "replaceAll", []Value{str(","), str("-")}, "a-b-c"},
		{"a,b", "split", []Value{str(",")}, "a,b"},
	}
	for _, c := range cases {
		wantString(t, c.recv, c.name, c.args, c.want)
	}
}

// TestStringOptionalArgumentDefaults pins that an undefined trailing argument reads as
// omitted rather than as NaN: "abc".slice(1, undefined) is the tail, not the empty
// string a NaN end would cut, and padStart with no pad uses its own space.
func TestStringOptionalArgumentDefaults(t *testing.T) {
	wantString(t, "abc", "slice", []Value{Number(1), Undefined}, "bc")
	wantString(t, "abc", "slice", []Value{Undefined, Number(2)}, "ab")
	wantString(t, "7", "padStart", []Value{Number(3)}, "  7")
	wantString(t, "abc", "indexOf", []Value{str("b"), Undefined}, "1")
}

// TestStringSplitAnswersABoxedArray pins that split hands back a real array value the
// dynamic path can index and read a length off, not a Go slice with no box.
func TestStringSplitAnswersABoxedArray(t *testing.T) {
	got := callStringMethod(t, "a,b,c", "split", str(","))
	if got.kind != KindArray {
		t.Fatalf("dynamic split answered %v, want an array", got)
	}
	if n := ToNumber(got.Get(FromGoString("length"))); n != 3 {
		t.Errorf("dynamic split length = %v, want 3", n)
	}
	if s := ToString(got.Get(FromGoString("1"))).ToGoString(); s != "b" {
		t.Errorf("dynamic split element 1 = %q, want \"b\"", s)
	}
}

// TestStringRegExpMethodsOnADynamicReceiver pins the four methods that take a regexp.
// The pattern argument is used live when it is a regexp box and compiled as a pattern
// otherwise, the ToRegExp step the specification makes.
func TestStringRegExpMethodsOnADynamicReceiver(t *testing.T) {
	re := RegExpValue(NewRegExpLiteral("l+", ""))
	wantString(t, "Hello", "search", []Value{re}, "2")
	wantString(t, "Hello", "replace", []Value{re, str("L")}, "HeLo")
	wantString(t, "Hello", "match", []Value{re}, "ll")
	wantString(t, "Hello", "search", []Value{str("l+")}, "2")

	global := RegExpValue(NewRegExpLiteral("l", "g"))
	wantString(t, "Hello", "replaceAll", []Value{global, str("L")}, "HeLLo")
	wantString(t, "a1b2", "split", []Value{RegExpValue(NewRegExpLiteral("[0-9]", ""))}, "a,b,")
}

// TestStringReplaceWithAReplacerFunction pins the callable replacement, the form a
// dynamic receiver reaches that the lowered path has no shape for. The replacer gets the
// whole argument list the specification defines, so a capture group, the match offset,
// and the subject all reach a callback with no declared arity.
func TestStringReplaceWithAReplacerFunction(t *testing.T) {
	seen := []Value{}
	fn := NewFunc(func(args []Value) Value {
		seen = args
		return StringValue(FromGoString("<" + ToString(Arg(args, 1)).ToGoString() + ">"))
	})
	re := RegExpValue(NewRegExpLiteral("l(l)", ""))
	wantString(t, "Hello", "replace", []Value{re, fn}, "He<l>o")
	if len(seen) != 4 {
		t.Fatalf("replacer got %d arguments, want 4 (match, group, offset, subject)", len(seen))
	}
	if got := ToNumber(seen[2]); got != 2 {
		t.Errorf("replacer offset = %v, want 2", got)
	}
	if got := ToString(seen[3]).ToGoString(); got != "Hello" {
		t.Errorf("replacer subject = %q, want \"Hello\"", got)
	}
}

// TestStringLiteralReplaceWithAReplacerFunction pins the same callable form over a
// literal search string, which walks the code units rather than the regexp engine. The
// replacer sees the matched text, its offset, and the subject.
func TestStringLiteralReplaceWithAReplacerFunction(t *testing.T) {
	fn := NewFunc(func(args []Value) Value {
		return StringValue(FromGoString(ToString(Arg(args, 0)).ToGoString() + ToString(Arg(args, 1)).ToGoString()))
	})
	wantString(t, "a-b-c", "replace", []Value{str("-"), fn}, "a-1b-c")
	wantString(t, "a-b-c", "replaceAll", []Value{str("-"), fn}, "a-1b-3c")
	wantString(t, "ab", "replaceAll", []Value{str(""), fn}, "0a1b2")
}

// TestStringMemberMissReadsUndefined pins that a name outside the surface reads as
// undefined the way a miss on any other receiver does, rather than answering a callable
// that would give a wrong result. localeCompare and the toLocale case mappings sit here
// because bento carries no locale data.
func TestStringMemberMissReadsUndefined(t *testing.T) {
	for _, name := range []string{"nope", "localeCompare", "toLocaleUpperCase", "matchAll"} {
		if got := StringValue(FromGoString("x")).Get(FromGoString(name)); !got.IsUndefined() {
			t.Errorf("dynamic read of .%s = %v, want undefined", name, got)
		}
	}
}
