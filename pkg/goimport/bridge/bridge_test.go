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

// TestGuardReturnsValueWhenNoPanic proves the boundary guard is transparent on the
// happy path: a go: call that returns normally returns its value unchanged, so the
// guard adds no behavior when nothing panics.
func TestGuardReturnsValueWhenNoPanic(t *testing.T) {
	if got := Guard(func() int { return 42 }); got != 42 {
		t.Errorf("Guard of a returning call = %d, want 42", got)
	}
}

// TestGuardConvertsGoPanicToThrow proves a Go panic that escapes a go: call is
// converted to a thrown GoError, the section 12.3 guarantee that a Go panic becomes
// a catchable JavaScript exception. A panic with an error value keeps the error
// reachable through Unwrap, and a panic with a non-error value carries its string
// form.
func TestGuardConvertsGoPanicToThrow(t *testing.T) {
	sentinel := errors.New("boom")
	func() {
		defer func() {
			r := recover()
			ge, ok := r.(GoError)
			if !ok {
				t.Fatalf("guard of an error panic raised %T, want GoError", r)
			}
			if !errors.Is(ge, sentinel) {
				t.Errorf("converted GoError does not unwrap to the panicked error")
			}
		}()
		Guard(func() int { panic(sentinel) })
	}()

	func() {
		defer func() {
			r := recover()
			ge, ok := r.(GoError)
			if !ok {
				t.Fatalf("guard of a string panic raised %T, want GoError", r)
			}
			if ge.Error() != "go: call panicked: kaboom" {
				t.Errorf("converted GoError message = %q, want the panic's string form", ge.Error())
			}
		}()
		Guard(func() int { panic("kaboom") })
	}()
}

// TestGuardPassesThrownThrough proves a deliberate bento throw crossing the guard
// is left to keep unwinding, not reclassified: a RangeError raised by the number
// check inside a go: call surfaces as the RangeError it is, so instanceof narrowing
// still tells a numeric overflow apart from a Go panic.
func TestGuardPassesThrownThrough(t *testing.T) {
	defer func() {
		r := recover()
		if _, ok := r.(RangeError); !ok {
			t.Fatalf("guard reclassified a bento throw to %T, want the RangeError unchanged", r)
		}
	}()
	Guard(func() float64 { panic(RangeError{Message: "overflow"}) })
}

// TestGuard0GuardsVoidCall proves the void form guards a go: call that returns
// nothing the same way, converting a Go panic to a thrown GoError.
func TestGuard0GuardsVoidCall(t *testing.T) {
	defer func() {
		if _, ok := recover().(GoError); !ok {
			t.Fatal("Guard0 did not convert a Go panic to a GoError")
		}
	}()
	Guard0(func() { panic("void boom") })
}

func TestSliceFromGoMarshalsEachElement(t *testing.T) {
	// A Go slice crosses to a bento array, element by element, through the conversion
	// the caller supplies: here each Go string becomes a bento string.
	got := SliceFromGo([]string{"a", "bb"}, StringFromGo)
	if got.Len() != 2 {
		t.Fatalf("array length = %v, want 2", got.Len())
	}
	if StringToGo(got.At(0)) != "a" || StringToGo(got.At(1)) != "bb" {
		t.Errorf("array elements = %q %q, want a bb", StringToGo(got.At(0)), StringToGo(got.At(1)))
	}
}

func TestSliceFromGoNilIsEmptyArray(t *testing.T) {
	// A nil Go slice crosses as an empty array, because a bento array has no nil.
	got := SliceFromGo([]int(nil), func(n int) float64 { return float64(n) })
	if got == nil || got.Len() != 0 {
		t.Errorf("nil slice crossed to %v, want an empty array", got)
	}
}

func TestSliceToGoMarshalsEachElement(t *testing.T) {
	// A bento array crosses to a Go slice, element by element, through the conversion
	// the caller supplies: here each bento string becomes a Go string.
	arr := value.NewArray(StringFromGo("x"), StringFromGo("yy"))
	got := SliceToGo(arr, StringToGo)
	if len(got) != 2 || got[0] != "x" || got[1] != "yy" {
		t.Errorf("slice = %q, want [x yy]", got)
	}
}

func TestSliceToGoNilArrayIsNilSlice(t *testing.T) {
	// A nil array (which a dense array never is, but the crossing tolerates) becomes a
	// nil Go slice, so a Go function that branches on nil sees it.
	var arr *value.Array[value.BStr]
	if got := SliceToGo(arr, StringToGo); got != nil {
		t.Errorf("nil array crossed to %v, want a nil slice", got)
	}
}

func TestSliceRoundTripThroughGo(t *testing.T) {
	// A bento array to a Go slice and back is the identity on its elements, the shape a
	// []T parameter and a []T result share.
	arr := value.NewArray(StringFromGo("one"), StringFromGo("two"))
	back := SliceFromGo(SliceToGo(arr, StringToGo), StringFromGo)
	if back.Len() != 2 || StringToGo(back.At(0)) != "one" || StringToGo(back.At(1)) != "two" {
		t.Errorf("round trip = %q %q, want one two", StringToGo(back.At(0)), StringToGo(back.At(1)))
	}
}
