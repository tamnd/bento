package value

import (
	"math"
	"testing"
)

// The box a Date takes when it crosses into a dynamic slot is a view rather than a
// copy, which is most of what these check: a write through the box moves the same
// instant the generated code keeps reading, one date has one box however many times it
// crosses, and the two coercion hints split the way they do for no other built-in. The
// readings themselves are held against Node in nodedatestring_test.go; what is here is
// the behavior of the box around them.

// boxedEpochDate builds a date at the epoch and hands back both halves, so a test can
// write through one and read through the other.
func boxedEpochDate(t *testing.T) (*Date, Value) {
	t.Helper()
	d := NewDateFromMillis(0)
	return d, d.ToValue()
}

// TestABoxedDateReadsTheLiveDate covers the read half of the member surface: every
// family of method answers off the live date, and a name that is not a Date member
// reads undefined rather than raising.
func TestABoxedDateReadsTheLiveDate(t *testing.T) {
	d, box := boxedEpochDate(t)
	d.SetTime(1673751845006)

	if got := callMember(t, box, "getTime"); got.AsNumber() != 1673751845006 {
		t.Errorf("getTime() = %v, want 1673751845006", got.AsNumber())
	}
	if got := callMember(t, box, "valueOf"); got.AsNumber() != 1673751845006 {
		t.Errorf("valueOf() = %v, want 1673751845006", got.AsNumber())
	}
	if got := callMember(t, box, "getUTCFullYear"); got.AsNumber() != 2023 {
		t.Errorf("getUTCFullYear() = %v, want 2023", got.AsNumber())
	}
	if got := callMember(t, box, "getUTCMonth"); got.AsNumber() != 0 {
		t.Errorf("getUTCMonth() = %v, want 0", got.AsNumber())
	}
	if got := callMember(t, box, "toISOString"); got.AsString().ToGoString() != "2023-01-15T03:04:05.006Z" {
		t.Errorf("toISOString() = %q, want the ISO reading", got.AsString().ToGoString())
	}
	// The local formats are read against the date itself rather than against a literal,
	// since the suite runs in whatever zone the machine is in and the zone is not what
	// this test is about.
	if got := callMember(t, box, "toString"); got.AsString().ToGoString() != d.ToString().ToGoString() {
		t.Errorf("toString() = %q, want the local reading %q", got.AsString().ToGoString(), d.ToString().ToGoString())
	}
	if got := callMember(t, box, "toJSON"); got.AsString().ToGoString() != "2023-01-15T03:04:05.006Z" {
		t.Errorf("toJSON() = %q, want the ISO reading", got.AsString().ToGoString())
	}
	if got := box.Get(FromGoString("nope")); got.Kind() != KindUndefined {
		t.Errorf("a name that is not a Date member = %v, want undefined", got.Kind())
	}
	// The name rides each bound method, so logging one reads "[Function: getTime]" the
	// way Node prints a built-in method read off a date.
	if got := box.Get(FromGoString("getTime")).Get(FromGoString("name")); got.AsString().ToGoString() != "getTime" {
		t.Errorf("the bound method's name = %q, want getTime", got.AsString().ToGoString())
	}
}

// TestAWriteThroughTheBoxMovesTheTypedDate is the point of a view: the two sides are
// one date, so the typed code that keeps running after a date was logged or passed to
// an any parameter reads what the dynamic side did to it.
func TestAWriteThroughTheBoxMovesTheTypedDate(t *testing.T) {
	d, box := boxedEpochDate(t)

	if got := callMember(t, box, "setUTCFullYear", Number(2000)); got.AsNumber() != 946684800000 {
		t.Errorf("setUTCFullYear(2000) = %v, want the new time value", got.AsNumber())
	}
	if d.GetUTCFullYear() != 2000 {
		t.Errorf("the typed date reads %v, want the year written through the box", d.GetUTCFullYear())
	}
	if got := callMember(t, box, "setTime", Number(5)); got.AsNumber() != 5 || d.GetTime() != 5 {
		t.Errorf("setTime(5) left the date at %v", d.GetTime())
	}
	// A setter called with nothing to set reads its first field as ToNumber(undefined),
	// which is NaN, so the date becomes the Invalid Date rather than staying where it
	// was. That is what the specification does and what Node does.
	if got := callMember(t, box, "setMonth"); !math.IsNaN(got.AsNumber()) {
		t.Errorf("setMonth() = %v, want NaN", got.AsNumber())
	}
	if !math.IsNaN(d.GetTime()) {
		t.Errorf("the typed date reads %v after an empty setter, want the Invalid Date", d.GetTime())
	}
	// An argument that is not a number coerces the way every other numeric position
	// does, so a string of digits writes the day it names.
	d.SetTime(0)
	callMember(t, box, "setUTCDate", StringValue(FromGoString("15")))
	if d.GetUTCDate() != 15 {
		t.Errorf("the day after a string argument = %v, want 15", d.GetUTCDate())
	}
}

// TestADateHasOneBox pins the cached view. A JavaScript Date is a reference, so two
// crossings of one date must be one value: without the cache, `d === d` would be false
// for an any-typed binding read twice and console.log would print two objects.
func TestADateHasOneBox(t *testing.T) {
	d := NewDateFromMillis(0)
	if !StrictEquals(d.ToValue(), d.ToValue()) {
		t.Error("two boxes of one date are not identical, want one view")
	}
	if StrictEquals(d.ToValue(), NewDateFromMillis(0).ToValue()) {
		t.Error("two different dates share a box, want two values")
	}
}

// TestABoxedDateCoercesByHint is the split that makes a date unlike every other
// object: its Symbol.toPrimitive answers the string form for the default hint, so + on
// a boxed date concatenates, while a numeric or relational operator asks for the number
// hint and reads the instant.
func TestABoxedDateCoercesByHint(t *testing.T) {
	d, box := boxedEpochDate(t)
	d.SetTime(1000)

	if got := ToString(box).ToGoString(); got != d.ToString().ToGoString() {
		t.Errorf("String(box) = %q, want the local reading %q", got, d.ToString().ToGoString())
	}
	if got := ToNumber(box); got != 1000 {
		t.Errorf("Number(box) = %v, want the time value 1000", got)
	}
	// The default hint is the one a bare + takes, and for a date it is the string form,
	// which is why box + 1 reads the local time followed by a "1" rather than 1001.
	if got := toPrimitiveDefault(box); got.kind != KindString {
		t.Errorf("the default hint gave a %v, want a string", got.kind)
	}
	// The hook is callable in its own right, and the specification has it reject a hint
	// that is not one of the three rather than pick one.
	hook := box.GetElem(SymbolToPrimitive())
	if hook.Kind() != KindFunc {
		t.Fatalf("Symbol.toPrimitive off the box = %v, want a function", hook.Kind())
	}
	if got := hook.Call(StringValue(FromGoString("number"))); got.AsNumber() != 1000 {
		t.Errorf("the number hint gave %v, want 1000", got.AsNumber())
	}
	err := catchThrown(t, func() { hook.Call(StringValue(FromGoString("bogus"))) })
	if err != "TypeError: Invalid hint: bogus" {
		t.Errorf("an unknown hint threw %q, want the invalid-hint TypeError", err)
	}
}

// catchThrown runs fn and returns the message of whatever it threw, or the empty
// string when it returned. It reads the thrown value as an error the way a catch
// clause does, so a test can hold a runtime throw to its wording.
func catchThrown(t *testing.T, fn func()) (msg string) {
	t.Helper()
	defer func() {
		if rec := recover(); rec != nil {
			msg = ToString(Caught(rec).ToValue()).ToGoString()
		}
	}()
	fn()
	return ""
}

// TestABoxedDateIsTaggedAndWalksEmpty covers what a date looks like from the outside:
// it names itself a Date to Object.prototype.toString through its brand, it carries its
// methods the way a real date carries them on its prototype, and its own property table
// is empty, so a key walk over one answers nothing the way Node's does.
func TestABoxedDateIsTaggedAndWalksEmpty(t *testing.T) {
	_, box := boxedEpochDate(t)

	if got := ClassTag(box).ToGoString(); got != "[object Date]" {
		t.Errorf("ClassTag(date) = %q, want [object Date]", got)
	}
	if got := box.TypeOf().ToGoString(); got != "object" {
		t.Errorf("typeof date = %q, want object", got)
	}
	if !ToBoolean(box) {
		t.Error("a boxed date is falsy, want truthy")
	}
	if got := box.OwnKeys().Len(); got != 0 {
		t.Errorf("the box has %v own keys, want none", got)
	}
	if !box.HasProperty(FromGoString("getTime")) {
		t.Error("'getTime' in date = false, want true")
	}
	if box.HasProperty(FromGoString("size")) {
		t.Error("'size' in date = true, want false")
	}
	// The tag comes off the brand rather than off a property, so it does not show up in
	// a symbol walk either: a real date carries no Symbol.toStringTag at all.
	if got := box.GetElem(SymbolToStringTag()); got.Kind() != KindUndefined {
		t.Errorf("date[Symbol.toStringTag] = %v, want undefined", got.Kind())
	}
}

// TestABoxedDatePrintsLikeNode holds the rendering to what Node prints: the ISO form
// for a date that names an instant, the human text for one that does not, and the
// properties after it for a date that was given some.
func TestABoxedDatePrintsLikeNode(t *testing.T) {
	_, box := boxedEpochDate(t)
	cases := []struct {
		name string
		v    Value
		want string
	}{
		{"a date", box, "1970-01-01T00:00:00.000Z"},
		{"an invalid date", NewDateFromMillis(math.NaN()).ToValue(), "Invalid Date"},
		{"nested in an object", objectOf("a", box), "{ a: 1970-01-01T00:00:00.000Z }"},
		{"nested in an array", NewArrayValue([]Value{box}), "[ 1970-01-01T00:00:00.000Z ]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NodeInspect(tc.v).ToGoString(); got != tc.want {
				t.Errorf("inspect = %q, want %q", got, tc.want)
			}
		})
	}

	// A date carrying properties of its own prints them after the instant, which is the
	// one shape that reaches the layout rather than returning the base text alone.
	withProps := NewDateFromMillis(0).ToValue()
	withProps.SetKey(FromGoString("x"), Number(5))
	if got := NodeInspect(withProps).ToGoString(); got != "1970-01-01T00:00:00.000Z { x: 5 }" {
		t.Errorf("a date with a property = %q, want the instant and the property", got)
	}
	// Past the depth limit a plain date still prints in full, since it has no entries to
	// recurse into, while one with properties is cut off by name.
	deep := objectOf("a", box)
	if got := NodeInspectArgs(deep, Undefined, Number(0)).ToGoString(); got != "{ a: 1970-01-01T00:00:00.000Z }" {
		t.Errorf("a date past the depth limit = %q, want the instant", got)
	}
	if got := NodeInspectArgs(objectOf("a", withProps), Undefined, Number(0)).ToGoString(); got != "{ a: [Date] }" {
		t.Errorf("a date with properties past the depth limit = %q, want [Date]", got)
	}
}

// objectOf builds a one-property object, the container the rendering tests nest a date
// in.
func objectOf(key string, v Value) Value {
	o := NewObject()
	o.SetKey(FromGoString(key), v)
	return o
}

// TestABoxedDateSerializesThroughToJSON covers JSON.stringify over a box in each of the
// three serializers. A date has no properties, so without the toJSON hook every one of
// them would write the empty object; with it they write the ISO string, and null for a
// date that names no instant, which is the whole reason Date.prototype.toJSON exists
// separately from toISOString.
func TestABoxedDateSerializesThroughToJSON(t *testing.T) {
	_, box := boxedEpochDate(t)
	invalid := NewDateFromMillis(math.NaN()).ToValue()

	if got := JSONStringify(box).ToGoString(); got != `"1970-01-01T00:00:00.000Z"` {
		t.Errorf("stringify(date) = %s, want the ISO string", got)
	}
	if got := JSONStringify(objectOf("when", box)).ToGoString(); got != `{"when":"1970-01-01T00:00:00.000Z"}` {
		t.Errorf("stringify({when: date}) = %s, want the ISO string", got)
	}
	if got := JSONStringify(NewArrayValue([]Value{box})).ToGoString(); got != `["1970-01-01T00:00:00.000Z"]` {
		t.Errorf("stringify([date]) = %s, want the ISO string", got)
	}
	if got := JSONStringify(invalid).ToGoString(); got != "null" {
		t.Errorf("stringify(invalid date) = %s, want null", got)
	}
	if got := JSONStringify(objectOf("x", invalid)).ToGoString(); got != `{"x":null}` {
		t.Errorf("stringify({x: invalid date}) = %s, want null", got)
	}
	want := "{\n  \"when\": \"1970-01-01T00:00:00.000Z\"\n}"
	if got := JSONStringifyIndentNum(objectOf("when", box), 2).ToGoString(); got != want {
		t.Errorf("indented stringify = %q, want %q", got, want)
	}
	// The hook runs before the replacer, so a replacer sees the ISO string rather than
	// the date, which is the order SerializeJSONProperty takes the two steps in.
	seen := ""
	trim := func(key BStr, v Value) Value {
		if v.kind == KindString {
			seen = v.str().ToGoString()
			return StringValue(FromGoString(v.str().ToGoString()[:4]))
		}
		return v
	}
	if got := JSONStringifyReplacerFunc(box, trim, "").ToGoString(); got != `"1970"` {
		t.Errorf("stringify(date, replacer) = %s, want the replaced string", got)
	}
	if seen != "1970-01-01T00:00:00.000Z" {
		t.Errorf("the replacer saw %q, want the ISO string the hook returned", seen)
	}
}

// TestTwoDatesAreDeeplyEqualByTheirInstant holds the comparison assert makes. A date is
// its moment, so two dates built apart are equal when they name the same one, and two
// that name none are equal to each other, which node compares as values rather than as
// the NaN time values they hold.
func TestTwoDatesAreDeeplyEqualByTheirInstant(t *testing.T) {
	cases := []struct {
		name string
		a, b Value
		want bool
	}{
		{"the same instant", NewDateFromMillis(0).ToValue(), NewDateFromMillis(0).ToValue(), true},
		{"different instants", NewDateFromMillis(0).ToValue(), NewDateFromMillis(1).ToValue(), false},
		{"two invalid dates", NewDateFromMillis(math.NaN()).ToValue(), NewDateFromMillis(math.NaN()).ToValue(), true},
		{"a date and an empty object", NewDateFromMillis(0).ToValue(), NewObject(), false},
		{"a date and a number", NewDateFromMillis(0).ToValue(), Number(0), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeepStrictEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("deepStrictEqual = %v, want %v", got, tc.want)
			}
		})
	}
	// A property added to one of them is compared too: the instant is what makes two
	// dates comparable at all, not all there is to compare.
	a, b := NewDateFromMillis(0).ToValue(), NewDateFromMillis(0).ToValue()
	a.SetKey(FromGoString("x"), Number(1))
	if DeepStrictEqual(a, b) {
		t.Error("a date with a property is deeply equal to one without, want unequal")
	}
	b.SetKey(FromGoString("x"), Number(1))
	if !DeepStrictEqual(a, b) {
		t.Error("two dates with the same property and instant are unequal, want equal")
	}
}

// TestADateInACollectionBoxIsItsOwnView covers the element bridge: a Map or a Set of
// dates presents each member through the date's own box, so a date read out of a boxed
// collection is the date the typed side holds rather than a copy of it.
func TestADateInACollectionBoxIsItsOwnView(t *testing.T) {
	d := NewDateFromMillis(0)
	m := NewStringMap[*Date]()
	m.Set(FromGoString("k"), d)
	box := m.ToValue()

	got := callMember(t, box, "get", StringValue(FromGoString("k")))
	if !StrictEquals(got, d.ToValue()) {
		t.Fatal("the date read out of the box is not the date the map holds")
	}
	if want := "Map(1) { 'k' => 1970-01-01T00:00:00.000Z }"; NodeInspect(box).ToGoString() != want {
		t.Errorf("inspect = %q, want %q", NodeInspect(box).ToGoString(), want)
	}
	// A write through the box has to reach the same date, which is what a view means:
	// the map holds the date, not a picture of it.
	callMember(t, got, "setTime", Number(1000))
	if d.GetTime() != 1000 {
		t.Errorf("the map's date reads %v, want the instant written through the box", d.GetTime())
	}
}
