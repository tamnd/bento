package value

import "testing"

// TestJSONStringifyReplacerArray checks the array whitelist: only the listed keys
// are serialized, in the array's order, a listed key the object lacks is dropped,
// and the whitelist does not reach into array indices.
func TestJSONStringifyReplacerArray(t *testing.T) {
	type Rec struct {
		A float64 `json:"a"`
		B float64 `json:"b"`
		C float64 `json:"c"`
	}
	rec := Rec{A: 1, B: 2, C: 3}
	if got := JSONStringifyReplacerArray(rec, []BStr{FromGoString("a"), FromGoString("c")}, "").ToGoString(); got != `{"a":1,"c":3}` {
		t.Fatalf("array whitelist = %q, want %q", got, `{"a":1,"c":3}`)
	}
	// A listed key the object does not have is skipped rather than serialized as null.
	if got := JSONStringifyReplacerArray(rec, []BStr{FromGoString("a"), FromGoString("zzz")}, "").ToGoString(); got != `{"a":1}` {
		t.Fatalf("array whitelist with absent key = %q, want %q", got, `{"a":1}`)
	}
}

// TestJSONStringifyReplacerArrayNested checks the whitelist reaching a nested
// object under a gap: the listed keys filter every object at every depth, and the
// gap indents the surviving members.
func TestJSONStringifyReplacerArrayNested(t *testing.T) {
	type Inner struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	type Rec struct {
		A float64 `json:"a"`
		B Inner   `json:"b"`
		C float64 `json:"c"`
	}
	rec := Rec{A: 1, B: Inner{X: 1, Y: 2}, C: 3}
	keys := []BStr{FromGoString("a"), FromGoString("b"), FromGoString("x")}
	want := "{\n  \"a\": 1,\n  \"b\": {\n    \"x\": 1\n  }\n}"
	if got := JSONStringifyReplacerArray(rec, keys, JSONGapNum(2)).ToGoString(); got != want {
		t.Fatalf("nested whitelist =\n%s\nwant\n%s", got, want)
	}
}

// TestJSONStringifyReplacerFuncDrop checks a function replacer that returns
// undefined for one key: that key is omitted from an object and its element folds
// to null in an array, the two ways SerializeJSONProperty treats a dropped value.
func TestJSONStringifyReplacerFuncDrop(t *testing.T) {
	type Rec struct {
		A      float64 `json:"a"`
		Secret float64 `json:"secret"`
		B      float64 `json:"b"`
	}
	dropSecret := func(k BStr, v Value) Value {
		if k.ToGoString() == "secret" {
			return Undefined
		}
		return v
	}
	rec := Rec{A: 1, Secret: 2, B: 3}
	if got := JSONStringifyReplacerFunc(rec, dropSecret, "").ToGoString(); got != `{"a":1,"b":3}` {
		t.Fatalf("replacer drop = %q, want %q", got, `{"a":1,"b":3}`)
	}
	dropIndexOne := func(k BStr, v Value) Value {
		if k.ToGoString() == "1" {
			return Undefined
		}
		return v
	}
	arr := NewArray(float64(1), float64(2), float64(3))
	if got := JSONStringifyReplacerFunc(arr, dropIndexOne, "").ToGoString(); got != `[1,null,3]` {
		t.Fatalf("replacer drop array element = %q, want %q", got, `[1,null,3]`)
	}
}

// TestJSONStringifyReplacerFuncReplace checks a function replacer that substitutes
// a value: the returned value is serialized in place, and the replacer is called
// for the root under the empty key before the walk descends.
func TestJSONStringifyReplacerFuncReplace(t *testing.T) {
	type Rec struct {
		A float64 `json:"a"`
		B float64 `json:"b"`
	}
	setA := func(k BStr, v Value) Value {
		if k.ToGoString() == "a" {
			return Number(99)
		}
		return v
	}
	rec := Rec{A: 1, B: 2}
	if got := JSONStringifyReplacerFunc(rec, setA, "").ToGoString(); got != `{"a":99,"b":2}` {
		t.Fatalf("replacer replace = %q, want %q", got, `{"a":99,"b":2}`)
	}
	// A replacer that returns undefined for the root yields the empty string, the
	// value model's stand-in for the undefined JSON.stringify returns.
	dropRoot := func(k BStr, v Value) Value {
		if k.ToGoString() == "" {
			return Undefined
		}
		return v
	}
	if got := JSONStringifyReplacerFunc(rec, dropRoot, "").ToGoString(); got != "" {
		t.Fatalf("replacer dropping root = %q, want empty string", got)
	}
}
