package value

import "testing"

// TestJSONStringifyScalars checks the leaf encodings against the exact text V8
// produces, since these are the arms every larger shape is built from.
func TestJSONStringifyScalars(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"string", FromGoString("hi"), `"hi"`},
		{"emptyString", FromGoString(""), `""`},
		{"true", true, "true"},
		{"false", false, "false"},
		{"zero", float64(0), "0"},
		{"negZero", negZero(), "0"},
		{"int", float64(42), "42"},
		{"neg", float64(-7), "-7"},
		{"frac", float64(1) / float64(7), "0.14285714285714285"},
		{"nan", nan(), "null"},
		{"posInf", inf(1), "null"},
		{"negInf", inf(-1), "null"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := JSONStringify(c.in).ToGoString()
			if got != c.want {
				t.Fatalf("JSONStringify(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestJSONStringifyBoxedUndefinedElement proves a boxed JSON-undefined value, a
// symbol being the motivating one, folds to null as an array element and omits its
// key as an object property, the same as a bare Go func. A symbol[] element and an
// any-typed field both reach the static walk as a value.Value, so the fold must key
// off the boxed kind, not only the Go func kind.
func TestJSONStringifyBoxedUndefinedElement(t *testing.T) {
	sym := NewSymbol(FromGoString("desc"))
	if got := JSONStringify(NewArray[Value](sym)).ToGoString(); got != "[null]" {
		t.Fatalf("JSONStringify([sym]) = %q, want [null]", got)
	}
	if got := JSONStringify(NewArray[Value](Number(1), sym, Number(2))).ToGoString(); got != "[1,null,2]" {
		t.Fatalf("JSONStringify([1,sym,2]) = %q, want [1,null,2]", got)
	}
	obj := NewObject()
	obj.Set(FromGoString("key"), sym)
	if got := JSONStringify(obj).ToGoString(); got != "{}" {
		t.Fatalf("JSONStringify({key: sym}) = %q, want {}", got)
	}
}

// TestJSONStringifyStringEscapes checks that string escaping matches the
// specification's well-formed JSON.stringify: the two structural characters, the
// short control escapes, the \u form for the other control characters, and a
// lone surrogate escaped while a valid pair and a plain non-ASCII rune are left
// literal. V8 does not escape <, >, or &, so those stay literal too.
func TestJSONStringifyStringEscapes(t *testing.T) {
	cases := []struct {
		name string
		in   BStr
		want string
	}{
		{"quote", FromGoString("a\"b"), "\"a\\\"b\""},
		{"backslash", FromGoString("a\\b"), "\"a\\\\b\""},
		{"newline", FromGoString("a\nb"), "\"a\\nb\""},
		{"tab", FromGoString("a\tb"), "\"a\\tb\""},
		{"return", FromGoString("a\rb"), "\"a\\rb\""},
		{"backspace", FromGoString("a\bb"), "\"a\\bb\""},
		{"formfeed", FromGoString("a\fb"), "\"a\\fb\""},
		{"unitSep", FromGoString("a\x1fb"), "\"a\\u001fb\""},
		{"nul", FromGoString("a\x00b"), "\"a\\u0000b\""},
		{"angles", FromGoString("<a>&b"), "\"<a>&b\""},
		{"nonASCII", FromGoString("café"), "\"café\""},
		{"astral", FromGoString("😀"), "\"😀\""},
		{"loneHigh", FromUTF16([]uint16{'a', 0xD800, 'b'}), "\"a\\ud800b\""},
		{"loneLow", FromUTF16([]uint16{'a', 0xDC00, 'b'}), "\"a\\udc00b\""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := JSONStringify(c.in).ToGoString()
			if got != c.want {
				t.Fatalf("JSONStringify(%q) = %q, want %q", c.in.ToGoString(), got, c.want)
			}
		})
	}
}

// TestJSONStringifyArray checks that an array serializes to a bracketed list with
// no spaces, recursing into each element, so the array arm and the recursion are
// both exercised.
func TestJSONStringifyArray(t *testing.T) {
	arr := NewArray(FromGoString("a"), FromGoString("b"), FromGoString("c"))
	if got := JSONStringify(arr).ToGoString(); got != `["a","b","c"]` {
		t.Fatalf("string array = %q", got)
	}
	nums := NewArray(float64(1), float64(2), float64(3))
	if got := JSONStringify(nums).ToGoString(); got != `[1,2,3]` {
		t.Fatalf("number array = %q", got)
	}
	empty := NewArray[float64]()
	if got := JSONStringify(empty).ToGoString(); got != `[]` {
		t.Fatalf("empty array = %q", got)
	}
}

// TestJSONStringifyEmbeddedStruct checks the reflection walk over a lowered
// class struct: an anonymous embedded base flattens into the same object the
// way JavaScript sees every inherited field as an own property, base fields
// first, and an unexported field (the vtable pointer a virtual hierarchy
// carries) is compiler machinery with no JSON form and is skipped.
func TestJSONStringifyEmbeddedStruct(t *testing.T) {
	type Animal struct {
		vt   *struct{ f func() }
		Legs float64 `json:"legs"`
	}
	type Dog struct {
		Animal
		Name BStr `json:"name"`
	}
	_ = Dog{}.vt
	d := &Dog{Animal: Animal{Legs: 4}, Name: FromGoString("rex")}
	if got := JSONStringify(d).ToGoString(); got != `{"legs":4,"name":"rex"}` {
		t.Fatalf("embedded struct = %q", got)
	}
}

// TestJSONStringifyOptionalField checks the reflection walk over an optional
// property, the value.Opt[T] a shape's optional field lowers to: a present
// optional serializes the value it wraps, and an absent optional (the same empty
// Opt an explicit undefined lowers to) omits its key the way JSON.stringify drops
// a property whose value is undefined.
func TestJSONStringifyOptionalField(t *testing.T) {
	type Rec struct {
		A float64      `json:"a"`
		B Opt[float64] `json:"b"`
		C Opt[BStr]    `json:"c"`
	}
	present := Rec{A: 1, B: Some(float64(2)), C: Some(FromGoString("x"))}
	if got := JSONStringify(present).ToGoString(); got != `{"a":1,"b":2,"c":"x"}` {
		t.Fatalf("present optionals = %q", got)
	}
	absent := Rec{A: 1, B: None[float64](), C: None[BStr]()}
	if got := JSONStringify(absent).ToGoString(); got != `{"a":1}` {
		t.Fatalf("absent optionals = %q", got)
	}
	mixed := Rec{A: 1, B: Some(float64(0)), C: None[BStr]()}
	if got := JSONStringify(mixed).ToGoString(); got != `{"a":1,"b":0}` {
		t.Fatalf("mixed optionals = %q", got)
	}
}

func nan() float64 { z := float64(0); return z / z }

func inf(s int) float64 {
	z := float64(0)
	if s < 0 {
		return -1 / z
	}
	return 1 / z
}

func negZero() float64 { z := float64(0); return -z }

// TestJSONStringifyGoInteger proves a value the lowering leaves as a Go integer, an
// untyped-constant argument boxed straight into the any slot, serializes as a number
// rather than reaching the struct reflection walk, which would NumField-panic on it.
func TestJSONStringifyGoInteger(t *testing.T) {
	if got := JSONStringify(123).ToGoString(); got != "123" {
		t.Fatalf("JSONStringify(123) = %q, want 123", got)
	}
	if got := JSONStringify(int64(-7)).ToGoString(); got != "-7" {
		t.Fatalf("JSONStringify(int64(-7)) = %q, want -7", got)
	}
}
