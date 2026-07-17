package value

import "testing"

// TestFromCharCodeValuesCoerces pins that FromCharCode over boxed arguments coerces each
// with ToNumber before taking the code unit, so a number, a numeric string, and a boolean
// all name their code unit the way String.fromCharCode.apply over a dynamic array does.
func TestFromCharCodeValuesCoerces(t *testing.T) {
	got := FromCharCodeValues(Number(104), StringValue(FromGoString("105")), Bool(true)).ToGoString()
	want := "hi\x01"
	if got != want {
		t.Fatalf("FromCharCodeValues coercion = %q, want %q", got, want)
	}
}

// TestFromCodePointValuesCoerces pins that FromCodePoint over boxed arguments coerces each
// with ToNumber, then encodes the code point, so a plain number and a numeric string both
// spell their character and an astral point becomes a surrogate pair.
func TestFromCodePointValuesCoerces(t *testing.T) {
	simple := FromCodePointValues(Number(104), StringValue(FromGoString("105"))).ToGoString()
	if simple != "hi" {
		t.Fatalf("FromCodePointValues coercion = %q, want %q", simple, "hi")
	}
	astral := FromCodePointValues(Number(0x1f600)).ToGoString()
	if len([]rune(astral)) != 1 {
		t.Fatalf("FromCodePointValues astral = %q, want a single rune", astral)
	}
}

// TestFromCodePointValuesRangeError pins that an out-of-range coerced element throws the
// same RangeError a direct String.fromCodePoint would, since the coercion delegates to the
// checked variadic constructor.
func TestFromCodePointValuesRangeError(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("FromCodePointValues(-1) did not throw")
		}
	}()
	FromCodePointValues(Number(-1))
}

// TestFromCharCodeValuesEmpty pins that no arguments yield the empty string, the apply-over-[]
// shape.
func TestFromCharCodeValuesEmpty(t *testing.T) {
	if FromCharCodeValues().ToGoString() != "" {
		t.Fatalf("FromCharCodeValues() over no args did not yield the empty string")
	}
}
