package value

import "testing"

// A boxed regexp is an object value: typeof is "object", it is truthy, it renders its
// literal form through ToString, and it answers its own accessors off the live regexp.
// Two boxes of the same regexp compare equal by identity, and a box of a distinct
// regexp does not, the reference semantics an object value carries.
func TestRegExpValueBox(t *testing.T) {
	re := NewRegExpLiteral("ab+c", "gi")
	box := RegExpValue(re)

	if got := box.TypeOf().ToGoString(); got != "object" {
		t.Fatalf("typeof boxed regexp = %q, want object", got)
	}
	if !ToBoolean(box) {
		t.Fatal("boxed regexp is falsy, want truthy")
	}
	if got := ToString(box).ToGoString(); got != "/ab+c/gi" {
		t.Fatalf("ToString(boxed regexp) = %q, want /ab+c/gi", got)
	}
	if got := box.asRegExp(); got != re {
		t.Fatalf("asRegExp did not return the boxed regexp")
	}
}

// The own-property reads off a boxed regexp read the live regexp: .source and .flags
// report the pattern and canonical flags, the flag booleans their own flag, and
// .lastIndex the resume offset, while a name that is not a regexp property reads
// undefined off the empty prototype chain.
func TestRegExpValueGet(t *testing.T) {
	box := RegExpValue(NewRegExpLiteral("ab+c", "gi"))
	cases := map[string]string{"source": "ab+c", "flags": "gi"}
	for name, want := range cases {
		if got := box.Get(FromGoString(name)).AsString().ToGoString(); got != want {
			t.Fatalf(".%s = %q, want %q", name, got, want)
		}
	}
	if !ToBoolean(box.Get(FromGoString("global"))) {
		t.Fatal(".global read false on a /g regexp")
	}
	if ToBoolean(box.Get(FromGoString("sticky"))) {
		t.Fatal(".sticky read true on a regexp without the y flag")
	}
	if got := ToNumber(box.Get(FromGoString("lastIndex"))); got != 0 {
		t.Fatalf(".lastIndex = %v, want 0", got)
	}
	if !box.Get(FromGoString("nope")).IsUndefined() {
		t.Fatal("a non-regexp property read something other than undefined")
	}
}

// A read of .test or .exec off the box yields a callable bound to the live regexp: a
// dynamic test reports the match boolean, an exec returns the match array with its
// captures and .index, and a global regexp advances its own lastIndex across calls
// because the closures share the one regexp.
func TestRegExpValueMethods(t *testing.T) {
	box := RegExpValue(NewRegExpLiteral("a(b+)", ""))
	if !ToBoolean(box.Get(FromGoString("test")).Call(StringValue(FromGoString("zabbbc")))) {
		t.Fatal("boxed .test missed a matching subject")
	}
	if ToBoolean(box.Get(FromGoString("test")).Call(StringValue(FromGoString("nope")))) {
		t.Fatal("boxed .test matched a non-matching subject")
	}
	res := box.Get(FromGoString("exec")).Call(StringValue(FromGoString("zabbbc")))
	if res.IsNull() {
		t.Fatal("boxed .exec returned null on a match")
	}
	if got := res.GetIndex(0).AsString().ToGoString(); got != "abbb" {
		t.Fatalf("exec[0] = %q, want abbb", got)
	}
	if got := res.GetIndex(1).AsString().ToGoString(); got != "bbb" {
		t.Fatalf("exec[1] = %q, want bbb", got)
	}

	g := RegExpValue(NewRegExpLiteral("a", "g"))
	testG := g.Get(FromGoString("test"))
	testG.Call(StringValue(FromGoString("aaa")))
	if got := ToNumber(g.Get(FromGoString("lastIndex"))); got != 1 {
		t.Fatalf("lastIndex after one global test = %v, want 1", got)
	}
	testG.Call(StringValue(FromGoString("aaa")))
	if got := ToNumber(g.Get(FromGoString("lastIndex"))); got != 2 {
		t.Fatalf("lastIndex after two global tests = %v, want 2 (the box shares one regexp)", got)
	}
}

// Identity: a regexp box compares equal to itself and unequal to a box of a distinct
// regexp, the reference semantics an object carries, so === over a boxed regexp holds
// only for the same box.
func TestRegExpValueIdentity(t *testing.T) {
	box := RegExpValue(NewRegExpLiteral("a", ""))
	if !StrictEquals(box, box) {
		t.Fatal("a regexp box is not === itself")
	}
	other := RegExpValue(NewRegExpLiteral("a", ""))
	if StrictEquals(box, other) {
		t.Fatal("two distinct regexp boxes compared ===, want distinct identities")
	}
}
