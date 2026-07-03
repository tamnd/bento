package bridge

import (
	"errors"
	"testing"

	"github.com/tamnd/bento/pkg/value"
)

func TestStringRoundTrip(t *testing.T) {
	// An ASCII string survives both crossings unchanged.
	got := StringToGo(StringFromGo("hello"))
	if got != "hello" {
		t.Errorf("round trip = %q, want hello", got)
	}
	// A non-ASCII string transcodes UTF-16 to UTF-8 and back.
	const s = "café 中文"
	if got := StringToGo(StringFromGo(s)); got != s {
		t.Errorf("non-ascii round trip = %q, want %q", got, s)
	}
}

func TestStringFromGoIsBentoString(t *testing.T) {
	// The Go-to-bento crossing produces a real bento string whose length is code
	// units, not bytes: a two-byte UTF-8 rune is one UTF-16 code unit.
	if got := StringFromGo("é").Length(); got != 1 {
		t.Errorf("length of é = %v, want 1 code unit", got)
	}
}

func TestInt64ToNumberInRange(t *testing.T) {
	for _, n := range []int64{0, 1, -1, value.NumberMaxSafeInteger, value.NumberMinSafeInteger} {
		if got := Int64ToNumber(n); got != float64(n) {
			t.Errorf("Int64ToNumber(%d) = %v, want %v", n, got, float64(n))
		}
	}
}

func TestInt64ToNumberOutOfRangeRaises(t *testing.T) {
	for _, n := range []int64{value.NumberMaxSafeInteger + 1, value.NumberMinSafeInteger - 1} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("Int64ToNumber(%d) did not raise", n)
					return
				}
				if _, ok := r.(RangeError); !ok {
					t.Errorf("Int64ToNumber(%d) raised %T, want RangeError", n, r)
				}
			}()
			Int64ToNumber(n)
		}()
	}
}

func TestUint64ToNumberOutOfRangeRaises(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Uint64ToNumber past the safe range did not raise")
		} else if _, ok := r.(RangeError); !ok {
			t.Errorf("raised %T, want RangeError", r)
		}
	}()
	Uint64ToNumber(value.NumberMaxSafeInteger + 1)
}

func TestMustReturnsValueWhenNoError(t *testing.T) {
	if got := Must(42, nil); got != 42 {
		t.Errorf("Must(42, nil) = %d, want 42", got)
	}
}

func TestMustRaisesGoErrorCarryingTheError(t *testing.T) {
	sentinel := errors.New("boom")
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Must with an error did not raise")
		}
		ge, ok := r.(GoError)
		if !ok {
			t.Fatalf("raised %T, want GoError", r)
		}
		// The original error is reachable through Unwrap, so errors.Is still works
		// across the boundary.
		if !errors.Is(ge, sentinel) {
			t.Errorf("wrapped error does not unwrap to the original")
		}
		if ge.Error() != "boom" {
			t.Errorf("GoError message = %q, want boom", ge.Error())
		}
	}()
	Must(0, sentinel)
}

func TestCheckRaisesOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Check(non-nil) did not raise")
		}
	}()
	Check(errors.New("nope"))
}

func TestCheckIsQuietOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Check(nil) raised %v", r)
		}
	}()
	Check(nil)
}
