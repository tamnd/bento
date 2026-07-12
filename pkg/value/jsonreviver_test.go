package value

import (
	"reflect"
	"testing"
)

// doubleNumbers is a reviver that doubles every number and passes any other value
// through, the shape a JSON.parse(text, reviver) arrow lowers to.
func doubleNumbers(_ BStr, v Value) Value {
	if v.kind == KindNumber {
		return Number(v.AsNumber() * 2)
	}
	return v
}

// TestJSONParseReviverReplace checks that a reviver's returned value replaces the
// property: doubling every number rebuilds the object with the doubled fields.
func TestJSONParseReviverReplace(t *testing.T) {
	got := JSONStringify(JSONParseReviver(FromGoString(`{"a":1,"b":2}`), doubleNumbers)).ToGoString()
	if got != `{"a":2,"b":4}` {
		t.Fatalf("reviver replace = %q, want %q", got, `{"a":2,"b":4}`)
	}
}

// TestJSONParseReviverArray checks that a reviver runs on array elements too,
// replacing each element with its returned value.
func TestJSONParseReviverArray(t *testing.T) {
	got := JSONStringify(JSONParseReviver(FromGoString(`[1,2,3]`), doubleNumbers)).ToGoString()
	if got != `[2,4,6]` {
		t.Fatalf("reviver array = %q, want %q", got, `[2,4,6]`)
	}
}

// TestJSONParseReviverDrop checks that a reviver returning undefined deletes the
// property, so the dropped key does not appear in the rebuilt object.
func TestJSONParseReviverDrop(t *testing.T) {
	drop := func(k BStr, v Value) Value {
		if k.ToGoString() == "secret" {
			return Undefined
		}
		return v
	}
	got := JSONStringify(JSONParseReviver(FromGoString(`{"secret":1,"keep":2}`), drop)).ToGoString()
	if got != `{"keep":2}` {
		t.Fatalf("reviver drop = %q, want %q", got, `{"keep":2}`)
	}
}

// TestJSONParseReviverBottomUp checks the walk order: a child is revived before
// its parent, and the root runs last under the empty key, so a nested document
// records its keys deepest-first.
func TestJSONParseReviverBottomUp(t *testing.T) {
	var order []string
	rec := func(k BStr, v Value) Value {
		order = append(order, k.ToGoString())
		return v
	}
	JSONParseReviver(FromGoString(`{"a":{"b":1}}`), rec)
	want := []string{"b", "a", ""}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("reviver visit order = %v, want %v", order, want)
	}
}

// TestJSONParseReviverReplaceRoot checks that the reviver can replace the whole
// document, since the root runs last under the empty key: returning a constant
// there discards the parsed tree.
func TestJSONParseReviverReplaceRoot(t *testing.T) {
	toName := func(k BStr, v Value) Value {
		if k.ToGoString() == "" {
			return StringValue(FromGoString("root"))
		}
		return v
	}
	got := JSONStringify(JSONParseReviver(FromGoString(`{"a":1}`), toName)).ToGoString()
	if got != `"root"` {
		t.Fatalf("reviver replace root = %q, want %q", got, `"root"`)
	}
}
