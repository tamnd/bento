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

func TestMapToGoMarshalsEachEntry(t *testing.T) {
	// A bento Map crosses to a Go map, entry by entry, through the key and value
	// conversions the caller supplies: here each bento string key becomes a Go string
	// and each number value a Go int.
	m := value.NewStringMap[float64]()
	m.Set(StringFromGo("a"), 1)
	m.Set(StringFromGo("b"), 2)
	got := MapToGo(m, StringToGo, func(v float64) int { return int(v) })
	if len(got) != 2 || got["a"] != 1 || got["b"] != 2 {
		t.Errorf("map = %v, want map[a:1 b:2]", got)
	}
}

func TestMapToGoNilMapIsNilMap(t *testing.T) {
	// A nil bento Map (which the map model never produces, but the crossing tolerates)
	// becomes a nil Go map, so a Go function that branches on nil sees it.
	var m *value.Map[value.BStr, float64]
	if got := MapToGo(m, StringToGo, func(v float64) int { return int(v) }); got != nil {
		t.Errorf("nil map crossed to %v, want a nil Go map", got)
	}
}

func TestMapFromGoFillsTheDestination(t *testing.T) {
	// A Go map crosses to the empty bento Map its key kind fixes, entry by entry,
	// through the conversions the caller supplies. The destination carries the string
	// key equality, so the two Go keys land as two distinct bento keys.
	got := MapFromGo(map[string]int{"a": 1, "b": 2}, value.NewStringMap[float64](), StringFromGo, func(n int) float64 { return float64(n) })
	if got.Size() != 2 {
		t.Fatalf("map size = %v, want 2", got.Size())
	}
	if v := got.Get(StringFromGo("a")); v.IsUndefined() || v.Get() != 1 {
		t.Errorf("get(a) = %+v, want present 1", v)
	}
	if v := got.Get(StringFromGo("b")); v.IsUndefined() || v.Get() != 2 {
		t.Errorf("get(b) = %+v, want present 2", v)
	}
}

func TestMapFromGoNilIsEmptyMap(t *testing.T) {
	// A nil Go map crosses as the empty destination map, because a bento Map has no nil,
	// so a Go function returning nil and one returning an empty map both hand back an
	// empty Map.
	got := MapFromGo(map[string]int(nil), value.NewStringMap[float64](), StringFromGo, func(n int) float64 { return float64(n) })
	if got == nil || got.Size() != 0 {
		t.Errorf("nil map crossed to %+v, want an empty Map", got)
	}
}

func TestMapRoundTripThroughGo(t *testing.T) {
	// A bento Map to a Go map and back is the identity on its entries, the shape a
	// map[K]V parameter and a map[K]V result share.
	m := value.NewStringMap[float64]()
	m.Set(StringFromGo("one"), 1)
	m.Set(StringFromGo("two"), 2)
	back := MapFromGo(MapToGo(m, StringToGo, func(v float64) int { return int(v) }), value.NewStringMap[float64](), StringFromGo, func(n int) float64 { return float64(n) })
	if back.Size() != 2 {
		t.Fatalf("round trip size = %v, want 2", back.Size())
	}
	if v := back.Get(StringFromGo("one")); v.Get() != 1 {
		t.Errorf("get(one) = %v, want 1", v.Get())
	}
	if v := back.Get(StringFromGo("two")); v.Get() != 2 {
		t.Errorf("get(two) = %v, want 2", v.Get())
	}
}

func TestBytesToGoCopiesTheBuffer(t *testing.T) {
	// The default crossing copies, so a Go callee that keeps and mutates the slice
	// cannot reach back into bento's buffer (section 7.3).
	a := value.Uint8ArrayOf(1, 2, 3)
	got := BytesToGo(a)
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("BytesToGo = %v, want [1 2 3]", got)
	}
	got[0] = 99
	if a.At(0) != 1 {
		t.Errorf("mutating the copy changed the buffer: At(0) = %v, want 1", a.At(0))
	}
}

func TestBytesToGoSharedAliasesTheBuffer(t *testing.T) {
	// The fast path hands over the backing slice itself, so a write the Go callee
	// makes within the call is visible in the buffer, which is exactly what the
	// zero-copy contract permits for an API that mutates in place.
	a := value.Uint8ArrayOf(1, 2, 3)
	got := BytesToGoShared(a)
	if len(got) != 3 {
		t.Fatalf("BytesToGoShared length = %d, want 3", len(got))
	}
	got[0] = 99
	if a.At(0) != 99 {
		t.Errorf("the shared slice did not alias the buffer: At(0) = %v, want 99", a.At(0))
	}
}

func TestBytesToGoNilArrayIsNilSlice(t *testing.T) {
	// A nil array crosses as a nil slice through both forms, so a Go function that
	// branches on nil sees it.
	if got := BytesToGo(nil); got != nil {
		t.Errorf("BytesToGo(nil) = %v, want a nil slice", got)
	}
	if got := BytesToGoShared(nil); got != nil {
		t.Errorf("BytesToGoShared(nil) = %v, want a nil slice", got)
	}
}

func TestBytesFromGoCopiesTheSlice(t *testing.T) {
	// The default result crossing copies, so a Go function that keeps the slice and
	// mutates it after return cannot change bytes bento now owns (section 7.3).
	b := []byte{10, 20, 30}
	got := BytesFromGo(b)
	if got.Len() != 3 || got.At(0) != 10 || got.At(2) != 30 {
		t.Fatalf("BytesFromGo produced %v %v %v, want 10 .. 30", got.At(0), got.At(1), got.At(2))
	}
	b[0] = 99
	if got.At(0) != 10 {
		t.Errorf("mutating the source changed the buffer: At(0) = %v, want 10", got.At(0))
	}
}

func TestBytesFromGoSharedAdoptsTheSlice(t *testing.T) {
	// The fast path adopts the returned slice, so it reads back the same bytes with no
	// copy; the lowerer emits this only when Go will not mutate after return.
	b := []byte{10, 20, 30}
	got := BytesFromGoShared(b)
	if got.Len() != 3 || got.At(1) != 20 {
		t.Errorf("BytesFromGoShared produced length %v At(1) %v, want 3 and 20", got.Len(), got.At(1))
	}
}

func TestBytesFromGoNilIsEmptyBuffer(t *testing.T) {
	// A nil Go slice crosses as an empty buffer, because a bento Uint8Array has no nil.
	if got := BytesFromGo(nil); got == nil || got.Len() != 0 {
		t.Errorf("BytesFromGo(nil) = %v, want an empty buffer", got)
	}
}

func TestBytesRoundTripThroughGo(t *testing.T) {
	// A Uint8Array to a Go slice and back is the identity on its bytes, the shape a
	// []byte parameter and a []byte result share.
	a := value.Uint8ArrayOf(4, 5, 6)
	back := BytesFromGo(BytesToGo(a))
	if back.Len() != 3 || back.At(0) != 4 || back.At(1) != 5 || back.At(2) != 6 {
		t.Errorf("round trip = %v %v %v, want 4 5 6", back.At(0), back.At(1), back.At(2))
	}
}

func TestAnyToGoUnwrapsScalars(t *testing.T) {
	// A bento scalar unwraps to the Go native a Go function inspecting the any expects,
	// so a type switch on the crossed value matches the concrete case (section 6.12).
	if got := AnyToGo(value.Null); got != nil {
		t.Errorf("null crossed to %v, want a nil interface", got)
	}
	if got := AnyToGo(value.Undefined); got != nil {
		t.Errorf("undefined crossed to %v, want a nil interface", got)
	}
	if got := AnyToGo(value.Bool(true)); got != true {
		t.Errorf("bool crossed to %v, want true", got)
	}
	if got := AnyToGo(value.Number(42)); got != float64(42) {
		t.Errorf("number crossed to %v, want float64(42)", got)
	}
	if got := AnyToGo(value.StringValue(value.FromGoString("hi"))); got != "hi" {
		t.Errorf("string crossed to %v, want the Go string hi", got)
	}
}

func TestAnyRoundTripThroughGo(t *testing.T) {
	// Every scalar bento kind survives a round trip through a Go any, the identity a
	// generic container gives when it stores and returns a value untouched.
	for _, v := range []value.Value{value.Null, value.Bool(false), value.Number(-7), value.StringValue(value.FromGoString("x"))} {
		back := AnyFromGo(AnyToGo(v))
		if back.Kind() != v.Kind() {
			t.Errorf("round trip of kind %v produced kind %v", v.Kind(), back.Kind())
		}
	}
	if back := AnyFromGo(AnyToGo(value.Number(3.5))); back.AsNumber() != 3.5 {
		t.Errorf("number round trip = %v, want 3.5", back.AsNumber())
	}
}

func TestAnyFromGoUnboxesGoNatives(t *testing.T) {
	// A Go native returned as any unboxes to the bento kind its dynamic Go type maps to,
	// so a Go library that builds a fresh value hands back a usable bento value.
	if got := AnyFromGo("go"); got.Kind() != value.KindString {
		t.Errorf("Go string unboxed to kind %v, want string", got.Kind())
	}
	if got := AnyFromGo(int64(9)); got.Kind() != value.KindNumber || got.AsNumber() != 9 {
		t.Errorf("Go int64 unboxed to %v, want the number 9", got)
	}
	if got := AnyFromGo(true); got.Kind() != value.KindBool || !got.AsBool() {
		t.Errorf("Go bool unboxed to %v, want true", got)
	}
	if got := AnyFromGo(nil); got.Kind() != value.KindNull {
		t.Errorf("nil unboxed to kind %v, want null", got.Kind())
	}
}

func TestAnyFromGoReferenceKeepsIdentity(t *testing.T) {
	// A bento array handed to Go as any crosses as its value.Value box, not a Go native,
	// so returning it through AnyFromGo yields the same reference value (section 7.4).
	arr := value.NewArrayValue([]value.Value{value.Number(1), value.Number(2)})
	crossed := AnyToGo(arr)
	if _, ok := crossed.(value.Value); !ok {
		t.Fatalf("a reference value crossed as %T, want its value.Value box", crossed)
	}
	back := AnyFromGo(crossed)
	if back.Kind() != value.KindArray {
		t.Errorf("reference round trip produced kind %v, want array", back.Kind())
	}
	if got := back.Get(value.FromGoString("length")); got.AsNumber() != 2 {
		t.Errorf("round-tripped array length = %v, want 2", got.AsNumber())
	}
}

func TestAnyFromGoUnprojectedTypeRaises(t *testing.T) {
	// A Go value of a type the value model cannot represent has no dynamic box, so the
	// crossing raises a boundary GoError rather than corrupt the value (section 6.12).
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("an unprojected Go type crossed as any did not raise")
		}
		if _, ok := r.(GoError); !ok {
			t.Errorf("raised %T, want GoError", r)
		}
	}()
	type foreign struct{ n int }
	AnyFromGo(foreign{n: 1})
}
