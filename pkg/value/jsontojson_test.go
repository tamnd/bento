package value

import "testing"

// money is a stand-in for a lowered class that declares a toJSON method: its
// ToJSON returns a string, so JSON.stringify serializes that string in place of
// the object's own fields.
type money struct {
	Amount float64 `json:"amount"`
}

func (m *money) ToJSON() BStr {
	return Concat(FromGoString("$"), NumberToString(m.Amount))
}

// TestJSONStringifyToJSON checks the toJSON hook across the three serializers: a
// top-level value serializes its toJSON return, a value nested in an object
// serializes it under the object's key, and a value nested in an array serializes
// it as the element, including under an indentation gap.
func TestJSONStringifyToJSON(t *testing.T) {
	m := &money{Amount: 5}
	if got := JSONStringify(m).ToGoString(); got != `"$5"` {
		t.Fatalf("top-level toJSON = %q, want %q", got, `"$5"`)
	}

	type wrap struct {
		Label BStr   `json:"label"`
		Cost  *money `json:"cost"`
	}
	w := wrap{Label: FromGoString("price"), Cost: m}
	if got := JSONStringify(w).ToGoString(); got != `{"label":"price","cost":"$5"}` {
		t.Fatalf("nested toJSON = %q, want %q", got, `{"label":"price","cost":"$5"}`)
	}

	arr := NewArray(m, m)
	wantArr := "[\n  \"$5\",\n  \"$5\"\n]"
	if got := JSONStringifyIndentNum(arr, 2).ToGoString(); got != wantArr {
		t.Fatalf("indented array toJSON = %q, want %q", got, wantArr)
	}
}

// TestJSONStringifyToJSONWithReplacer checks that the toJSON hook runs on the
// replacer path too: the lifted Value tree serializes the toJSON return, which the
// replacer then sees as the property's value.
func TestJSONStringifyToJSONWithReplacer(t *testing.T) {
	type wrap struct {
		Cost *money `json:"cost"`
	}
	w := wrap{Cost: &money{Amount: 7}}
	keep := func(_ BStr, v Value) Value { return v }
	if got := JSONStringifyReplacerFunc(w, keep, "").ToGoString(); got != `{"cost":"$7"}` {
		t.Fatalf("replacer with toJSON = %q, want %q", got, `{"cost":"$7"}`)
	}
}
