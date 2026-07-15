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

// TestJSONStringifyFunctionValue checks that a function value, which has no JSON
// form, serializes the way SerializeJSONProperty dictates: a function element in an
// array renders as null, a function-valued struct field is omitted, and a top-level
// function produces the empty string the typed call site spells for undefined. None
// of these reflect the func into NumField, the fault the encoder used to take.
func TestJSONStringifyFunctionValue(t *testing.T) {
	fn := func() {}
	if got := JSONStringify(fn).ToGoString(); got != "" {
		t.Fatalf("top-level function stringified to %q, want empty", got)
	}
	arr := NewArray[any](float64(1), fn, float64(2))
	if got := JSONStringify(arr).ToGoString(); got != "[1,null,2]" {
		t.Fatalf("function array element stringified to %q, want [1,null,2]", got)
	}
	type withFunc struct {
		A float64 `json:"a"`
		F func()  `json:"f"`
		B float64 `json:"b"`
	}
	if got := JSONStringify(withFunc{A: 1, F: fn, B: 2}).ToGoString(); got != `{"a":1,"b":2}` {
		t.Fatalf("function field stringified to %q, want {\"a\":1,\"b\":2}", got)
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

// TestHasProperty checks the in operator across the kinds the AOT path produces:
// an object reports its own keys (including one holding undefined), an array reports
// length and in-range indices, a string reports length and in-range character
// indices, and a primitive receiver throws a TypeError the way JavaScript does.
func TestHasProperty(t *testing.T) {
	obj := NewObject()
	obj.Set(FromGoString("name"), StringValue(FromGoString("bento")))
	obj.Set(FromGoString("missing"), Undefined)
	if !obj.HasProperty(FromGoString("name")) {
		t.Fatalf("object should carry own key name")
	}
	if !obj.HasProperty(FromGoString("missing")) {
		t.Fatalf("a key present with an undefined value should report true")
	}
	if obj.HasProperty(FromGoString("nope")) {
		t.Fatalf("object should not carry an absent key")
	}

	arr := NewArrayValue([]Value{Number(10), Number(20)})
	if !arr.HasProperty(FromGoString("length")) || !arr.HasProperty(FromGoString("1")) {
		t.Fatalf("array should carry length and an in-range index")
	}
	if arr.HasProperty(FromGoString("2")) {
		t.Fatalf("array should not carry an out-of-range index")
	}

	s := StringValue(FromGoString("hi"))
	if !s.HasProperty(FromGoString("length")) || !s.HasProperty(FromGoString("0")) {
		t.Fatalf("string should carry length and an in-range char index")
	}
	if s.HasProperty(FromGoString("2")) {
		t.Fatalf("string should not carry an out-of-range char index")
	}

	if !hasPropertyThrewTypeError(t, Number(1), FromGoString("x")) {
		t.Fatalf("in on a number should throw a TypeError")
	}
}

// hasPropertyThrewTypeError runs v.HasProperty(key) and reports whether it threw a
// TypeError, recovering the panic the Throw path raises so the test can assert on
// the non-object case the way JavaScript's in operator signals it.
func hasPropertyThrewTypeError(t *testing.T, v Value, key BStr) (threw bool) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		e, ok := r.(Thrown)
		if !ok {
			t.Fatalf("HasProperty panicked with a non-thrown value %v", r)
		}
		threw = e.ErrorName() == "TypeError"
	}()
	v.HasProperty(key)
	return false
}

// TestInOperator pins the general in operator: a string key reads own existence, a
// numeric key coerces to its property-key string, a symbol key is probed by identity,
// and a primitive receiver throws a TypeError the way key in primitive does in
// JavaScript. The prototype-chain walk is covered end to end in the lower tests, which
// run against a program with the global prototypes installed.
func TestInOperator(t *testing.T) {
	obj := NewObject()
	obj.Set(FromGoString("a"), Number(1))
	obj.Set(FromGoString("2"), Number(9))
	if !InOperator(StringValue(FromGoString("a")), obj) {
		t.Fatalf("in should report a present string key")
	}
	if InOperator(StringValue(FromGoString("b")), obj) {
		t.Fatalf("in should report an absent string key false")
	}
	if !InOperator(Number(2), obj) {
		t.Fatalf("a numeric key should coerce to its property-key string and read the slot")
	}

	sym := NewSymbol(FromGoString("k"))
	obj.SetElem(sym, Number(3))
	if !InOperator(sym, obj) {
		t.Fatalf("in should probe a symbol key by identity")
	}
	if InOperator(NewSymbol(FromGoString("k")), obj) {
		t.Fatalf("a distinct symbol with the same description should report absent")
	}

	if !inOperatorThrewTypeError(t, StringValue(FromGoString("x")), Number(5)) {
		t.Fatalf("in on a primitive receiver should throw a TypeError")
	}
}

// inOperatorThrewTypeError runs InOperator(key, obj) and reports whether it threw a
// TypeError, recovering the panic the Throw path raises so the test can assert on the
// non-object receiver the way JavaScript's in operator signals it.
func inOperatorThrewTypeError(t *testing.T, key, obj Value) (threw bool) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		e, ok := r.(Thrown)
		if !ok {
			t.Fatalf("InOperator panicked with a non-thrown value %v", r)
		}
		threw = e.ErrorName() == "TypeError"
	}()
	InOperator(key, obj)
	return false
}

func isNaN(f float64) bool { return f != f }

// FromEmpty is a tiny test helper for the empty-string boolean case, kept local so
// the test reads without a raw literal.
func FromEmpty() Value { return StringValue(FromGoString("")) }
