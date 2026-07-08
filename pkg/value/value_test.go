package value

import "testing"

// TestJSONParseScalars checks that each leaf grammar production parses to the
// right boxed kind and value.
func TestJSONParseScalars(t *testing.T) {
	if v := JSONParse(FromGoString("true")); v.Kind() != KindBool || !v.AsBool() {
		t.Fatalf("true parsed to %v", v.Kind())
	}
	if v := JSONParse(FromGoString("false")); v.Kind() != KindBool || v.AsBool() {
		t.Fatalf("false parsed to %v", v.Kind())
	}
	if v := JSONParse(FromGoString("null")); v.Kind() != KindNull {
		t.Fatalf("null parsed to %v", v.Kind())
	}
	if v := JSONParse(FromGoString("42")); v.Kind() != KindNumber || v.AsNumber() != 42 {
		t.Fatalf("number parsed to %v %v", v.Kind(), v.AsNumber())
	}
	if v := JSONParse(FromGoString("-3.5e2")); v.AsNumber() != -350 {
		t.Fatalf("exponent number = %v", v.AsNumber())
	}
	if v := JSONParse(FromGoString(`"hi\nA"`)); ToString(v).ToGoString() != "hi\nA" {
		t.Fatalf("string parsed to %q", ToString(v).ToGoString())
	}
}

// TestJSONParseLength checks the property read the roundtrip workload depends on:
// the length of a parsed array and of a parsed string.
func TestJSONParseLength(t *testing.T) {
	arr := JSONParse(FromGoString(`[10, 20, 30]`))
	if arr.Kind() != KindArray {
		t.Fatalf("array parsed to %v", arr.Kind())
	}
	if n := arr.Get(FromGoString("length")).AsNumber(); n != 3 {
		t.Fatalf("array length = %v", n)
	}
	if e := arr.Get(FromGoString("1")).AsNumber(); e != 20 {
		t.Fatalf("array[1] = %v", e)
	}
	str := JSONParse(FromGoString(`"hello"`))
	if n := str.Get(FromGoString("length")).AsNumber(); n != 5 {
		t.Fatalf("string length = %v", n)
	}
}

// TestJSONParseObjectOrder checks that a parsed object keeps insertion order and
// looks its keys up, and that a missing key reads undefined.
func TestJSONParseObjectOrder(t *testing.T) {
	obj := JSONParse(FromGoString(`{"id":7,"name":"x","active":true}`))
	if obj.Kind() != KindObject {
		t.Fatalf("object parsed to %v", obj.Kind())
	}
	if obj.Get(FromGoString("id")).AsNumber() != 7 {
		t.Fatal("id")
	}
	if ToString(obj.Get(FromGoString("name"))).ToGoString() != "x" {
		t.Fatal("name")
	}
	if !obj.Get(FromGoString("active")).AsBool() {
		t.Fatal("active")
	}
	if !obj.Get(FromGoString("missing")).IsUndefined() {
		t.Fatal("missing key should be undefined")
	}
}

// TestJSONRoundTrip checks the property the roundtrip workload leans on: a value
// stringified and parsed and stringified again produces the identical text, so
// the parser and serializer are exact inverses over what the serializer emits.
func TestJSONRoundTrip(t *testing.T) {
	docs := []string{
		`[]`,
		`{}`,
		`[1,2,3]`,
		`{"id":0,"name":"item-0","active":true,"tags":["a","b","c","0"],"meta":{"created":0,"score":0,"nested":{"depth":0}}}`,
		`[{"a":1},{"a":2}]`,
		`"escapes:\"\\\n\t"`,
		`[true,false,null,1.5,-2,"s"]`,
	}
	for _, d := range docs {
		v := JSONParse(FromGoString(d))
		if got := JSONStringify(v).ToGoString(); got != d {
			t.Fatalf("round trip of %s produced %s", d, got)
		}
	}
}

// parseThrewSyntaxError runs JSONParse on doc and reports whether it threw a
// SyntaxError, recovering the panic the Throw path raises. It fails the test on a
// panic that is not a thrown error, the way an uncaught runtime fault should
// surface rather than be swallowed.
func parseThrewSyntaxError(t *testing.T, doc string) (threw bool) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		e, ok := r.(Thrown)
		if !ok {
			t.Fatalf("JSONParse(%q) panicked with a non-thrown value %v", doc, r)
		}
		threw = e.ErrorName() == "SyntaxError"
	}()
	JSONParse(FromGoString(doc))
	return false
}

// TestJSONParseThrowsOnMalformed checks that a value that does not parse, and
// non-whitespace content after the one top-level value, throw the SyntaxError
// JavaScript raises rather than returning undefined.
func TestJSONParseThrowsOnMalformed(t *testing.T) {
	malformed := []string{
		`{ bad }`,
		`12 34`,
		`12` + "\t\r\n " + `34`,
		`[1, 2,]`,
		`"unterminated`,
		``,
		`nul`,
		`{"a":1`,
	}
	for _, d := range malformed {
		if !parseThrewSyntaxError(t, d) {
			t.Errorf("JSONParse(%q) did not throw a SyntaxError", d)
		}
	}
}

// TestJSONParseWellFormedDoesNotThrow checks the throw path stays off valid input,
// including a document padded with the JSON whitespace set, so the SyntaxError is
// raised only on malformed text.
func TestJSONParseWellFormedDoesNotThrow(t *testing.T) {
	wellFormed := []string{
		`42`,
		` [1, 2, 3] `,
		"\t\r\n {\"a\": 1} \t",
		`"hi"`,
		`null`,
	}
	for _, d := range wellFormed {
		if parseThrewSyntaxError(t, d) {
			t.Errorf("JSONParse(%q) threw on well-formed input", d)
		}
	}
}

// numOrStrArm stands in for a generated tagged-sum union in this test: a value that
// carries one member and hands it back through JSONArm, the hook the JSON walk reads
// so it serializes the active member rather than reflecting the struct's unexported
// fields into an empty object.
type numOrStrArm struct {
	isStr bool
	num   float64
	str   BStr
}

func (u numOrStrArm) JSONArm() any {
	if u.isStr {
		return u.str
	}
	return u.num
}

// TestJSONStringifyUnionArm checks that a value exposing JSONArm serializes as its
// active member: a number arm renders as the number and a string arm as the quoted
// string, and an array of such values renders each element by its arm rather than as
// the empty object a reflected union struct would produce.
func TestJSONStringifyUnionArm(t *testing.T) {
	if got := JSONStringify(numOrStrArm{num: 42}).ToGoString(); got != "42" {
		t.Fatalf("number arm stringified to %q", got)
	}
	if got := JSONStringify(numOrStrArm{isStr: true, str: FromGoString("hi")}).ToGoString(); got != `"hi"` {
		t.Fatalf("string arm stringified to %q", got)
	}
	arr := NewArray(
		numOrStrArm{isStr: true, str: FromGoString("-0")},
		numOrStrArm{num: 0},
		numOrStrArm{num: negZero()},
	)
	if got := JSONStringify(arr).ToGoString(); got != `["-0",0,0]` {
		t.Fatalf("union array stringified to %q", got)
	}
}

// TestValueCoercions checks the ToNumber, ToString, ToBoolean, and Add operations
// the dynamic arithmetic path uses, against the JavaScript results.
func TestValueCoercions(t *testing.T) {
	if ToNumber(True) != 1 || ToNumber(False) != 0 || ToNumber(Null) != 0 {
		t.Fatal("bool/null to number")
	}
	if ToNumber(StringValue(FromGoString("  12 "))) != 12 {
		t.Fatal("string to number trims")
	}
	if !isNaN(ToNumber(Undefined)) {
		t.Fatal("undefined to number is NaN")
	}
	if ToString(Number(1.5)).ToGoString() != "1.5" {
		t.Fatal("number to string")
	}
	if ToString(Null).ToGoString() != "null" {
		t.Fatal("null to string")
	}
	if ToBoolean(Number(0)) || !ToBoolean(Number(1)) || ToBoolean(FromEmpty()) {
		t.Fatal("truthiness")
	}
	// number + number adds; a string operand concatenates.
	if sum := Add(Number(2000), Number(9)); sum.Kind() != KindNumber || sum.AsNumber() != 2009 {
		t.Fatalf("number add = %v", sum.AsNumber())
	}
	if cat := Add(StringValue(FromGoString("a")), Number(1)); ToString(cat).ToGoString() != "a1" {
		t.Fatalf("string concat = %q", ToString(cat).ToGoString())
	}
}

func isNaN(f float64) bool { return f != f }

// FromEmpty is a tiny test helper for the empty-string boolean case, kept local so
// the test reads without a raw literal.
func FromEmpty() Value { return StringValue(FromGoString("")) }
