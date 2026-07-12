package value

import "testing"

// TestJSONStringifyIndentNum checks the numeric-space form against the exact text
// V8 produces: a nested object and array indent one gap deeper per level with a
// newline before each member, an empty object and array stay on one line, and a
// space of zero falls back to the compact form.
func TestJSONStringifyIndentNum(t *testing.T) {
	type Inner struct {
		D float64 `json:"d"`
	}
	type Rec struct {
		A float64         `json:"a"`
		B *Array[float64] `json:"b"`
		C Inner           `json:"c"`
	}
	rec := Rec{A: 1, B: NewArray(float64(2), float64(3)), C: Inner{D: 4}}
	wantRec := "{\n  \"a\": 1,\n  \"b\": [\n    2,\n    3\n  ],\n  \"c\": {\n    \"d\": 4\n  }\n}"
	if got := JSONStringifyIndentNum(rec, 2).ToGoString(); got != wantRec {
		t.Fatalf("indent num object =\n%s\nwant\n%s", got, wantRec)
	}
	arr := NewArray(float64(1), float64(2), float64(3))
	wantArr := "[\n  1,\n  2,\n  3\n]"
	if got := JSONStringifyIndentNum(arr, 2).ToGoString(); got != wantArr {
		t.Fatalf("indent num array = %q, want %q", got, wantArr)
	}
	if got := JSONStringifyIndentNum(NewArray[float64](), 2).ToGoString(); got != "[]" {
		t.Fatalf("indent empty array = %q, want []", got)
	}
	if got := JSONStringifyIndentNum(Inner{D: 0}, 0).ToGoString(); got != `{"d":0}` {
		t.Fatalf("zero space = %q, want compact", got)
	}
}

// TestJSONStringifyIndentNumClamp checks that a space above ten clamps to ten and
// a fractional space floors through ToInteger, matching the specification's gap.
func TestJSONStringifyIndentNumClamp(t *testing.T) {
	type Box struct {
		A float64 `json:"a"`
	}
	tenSpaces := "{\n" + "          " + "\"a\": 1\n}"
	if got := JSONStringifyIndentNum(Box{A: 1}, 20).ToGoString(); got != tenSpaces {
		t.Fatalf("space 20 = %q, want ten-space indent %q", got, tenSpaces)
	}
	twoSpaces := "{\n  \"a\": 1\n}"
	if got := JSONStringifyIndentNum(Box{A: 1}, 2.9).ToGoString(); got != twoSpaces {
		t.Fatalf("space 2.9 = %q, want two-space indent %q", got, twoSpaces)
	}
}

// TestJSONStringifyIndentStr checks the string-space form: the gap is the string
// itself (a tab here), an empty string falls back to the compact form, and a
// string longer than ten characters is truncated to its first ten.
func TestJSONStringifyIndentStr(t *testing.T) {
	type Box struct {
		A float64 `json:"a"`
	}
	wantTab := "{\n\t\"a\": 1\n}"
	if got := JSONStringifyIndentStr(Box{A: 1}, FromGoString("\t")).ToGoString(); got != wantTab {
		t.Fatalf("tab space = %q, want %q", got, wantTab)
	}
	if got := JSONStringifyIndentStr(Box{A: 1}, FromGoString("")).ToGoString(); got != `{"a":1}` {
		t.Fatalf("empty string space = %q, want compact", got)
	}
	wantTrunc := "{\n0123456789\"a\": 1\n}"
	if got := JSONStringifyIndentStr(Box{A: 1}, FromGoString("0123456789abc")).ToGoString(); got != wantTrunc {
		t.Fatalf("long string space = %q, want ten-char gap %q", got, wantTrunc)
	}
}

// TestJSONStringifyIndentBoxed checks the indented form over a boxed dynamic value,
// the shape a JSON.parse result takes flowing back through an indented stringify.
func TestJSONStringifyIndentBoxed(t *testing.T) {
	parsed := JSONParse(FromGoString(`{"a":1,"b":[2,3]}`))
	want := "{\n  \"a\": 1,\n  \"b\": [\n    2,\n    3\n  ]\n}"
	if got := JSONStringifyIndentNum(parsed, 2).ToGoString(); got != want {
		t.Fatalf("indent boxed =\n%s\nwant\n%s", got, want)
	}
}
