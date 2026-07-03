// Package bridge is the runtime helper package a go: import's emitted code calls
// to marshal values across the interop boundary (16_go_interop.md section 9.4). It
// is compiled into the binary like the rest of bento's runtime, cgo-free, and it
// depends only on the value model, so importing it never drags the code-generation
// toolchain (go/packages, go/types) that the parent goimport package uses into the
// emitted program.
//
// A go: call is a plain Go call after lowering, so the crossing is thin: most of
// what this package does is the value model's own boxing, named here as a stable
// entry point per crossing type (section 7.1). This file covers the synchronous
// scalar crossings: strings, the 64-bit number range check, and the error-to-throw
// bridge. Bytes and typed arrays (section 7.3), callbacks (section 7.6), and
// channels (section 8) arrive with the value-model and event-loop machinery they
// depend on.
package bridge

import (
	"fmt"

	"github.com/tamnd/bento/pkg/value"
)

// StringToGo transcodes a bento string to the Go UTF-8 string a Go func parameter
// of type string expects (section 7.2). A bento string is UTF-16 code units and a
// Go string is UTF-8, so the crossing copies; an unpaired surrogate encodes to the
// replacement character, matching Go's own tolerance.
func StringToGo(s value.BStr) string { return s.ToGoString() }

// StringFromGo transcodes a Go UTF-8 string returned from a Go call to a bento
// string (section 7.2). Invalid UTF-8 decodes with the replacement character, the
// way JavaScript treats invalid input, so the crossing never fails.
func StringFromGo(s string) value.BStr { return value.FromGoString(s) }

// Int64ToNumber widens a Go int64 result to a bento number, checking the range a
// 64-bit integer can exceed (section 7.5). A symbol projected as number promises a
// value JavaScript can represent exactly, so an int64 outside the safe-integer
// range raises rather than return a silently truncated number; a symbol known to
// produce large values is projected as bigint instead and skips this path. The
// small Go integer types always fit, so they convert with a plain float64() in the
// emitted code and never call here.
func Int64ToNumber(n int64) float64 {
	if n > value.NumberMaxSafeInteger || n < value.NumberMinSafeInteger {
		panic(RangeError{Message: "go: integer result out of Number.MAX_SAFE_INTEGER range"})
	}
	return float64(n)
}

// Uint64ToNumber widens a Go uint64 result to a bento number with the same
// safe-integer check as Int64ToNumber; the lower bound is zero, so only the upper
// bound can trip.
func Uint64ToNumber(n uint64) float64 {
	if n > value.NumberMaxSafeInteger {
		panic(RangeError{Message: "go: unsigned integer result out of Number.MAX_SAFE_INTEGER range"})
	}
	return float64(n)
}

// SliceToGo marshals a bento array to a Go slice element by element, applying conv
// to each element to cross it to the Go element type (section 6.4). A nil array (a
// hole the array model never produces for a dense array) crosses as a nil slice, and
// an empty array as an empty non-nil slice, so a Go function that distinguishes nil
// from empty sees the array's own emptiness. The element conversion is the scalar
// crossing the emitted closure names, so a []string parameter reuses StringToGo per
// element and a number slice reuses the numeric conversion.
func SliceToGo[B any, G any](a *value.Array[B], conv func(B) G) []G {
	if a == nil {
		return nil
	}
	src := a.Elems()
	out := make([]G, len(src))
	for i, v := range src {
		out[i] = conv(v)
	}
	return out
}

// SliceFromGo marshals a Go slice returned from a go: call to a bento array, applying
// conv to each element to cross it back (section 6.4). A nil Go slice crosses as an
// empty array, because a bento array has no nil, so a Go function returning nil and a
// Go function returning an empty slice both hand back an empty array. Any per-element
// range check (a []int64 element through Int64ToNumber) runs inside conv, so the same
// safe-integer guarantee the scalar crossing gives applies to every element.
func SliceFromGo[G any, B any](s []G, conv func(G) B) *value.Array[B] {
	out := make([]B, len(s))
	for i, v := range s {
		out[i] = conv(v)
	}
	return value.NewArray(out...)
}

// MapToGo marshals a bento Map to a Go map, applying keyConv and valConv to cross
// each entry to the Go key and value types (section 6.5). It iterates the bento map
// once through Range, so the single pass the crossing costs is the map's own
// iteration, and inserts each converted pair into a fresh Go map. A nil bento map (a
// shape the map model does not produce) crosses as a nil Go map, so a Go function
// that branches on nil sees it; an empty map crosses as an empty non-nil map. The
// key conversion is the scalar crossing the emitted closure names, so a map[string]V
// reuses StringToGo per key and a numeric-keyed map reuses the numeric conversion,
// exactly as a single key would cross. The Go key type is comparable because every
// key kind bento supports (string, number, boolean) is a comparable Go type.
func MapToGo[BK any, BV any, GK comparable, GV any](m *value.Map[BK, BV], keyConv func(BK) GK, valConv func(BV) GV) map[GK]GV {
	if m == nil {
		return nil
	}
	out := make(map[GK]GV, int(m.Size()))
	m.Range(func(k BK, v BV) {
		out[keyConv(k)] = valConv(v)
	})
	return out
}

// MapFromGo marshals a Go map returned from a go: call to a bento Map, applying
// keyConv and valConv to cross each entry back (section 6.5). The caller passes the
// empty bento map dst its key kind fixes (NewNumberMap, NewStringMap, or NewBoolMap),
// because the bento map carries a per-kind key equality the bridge cannot pick from
// the Go types alone, and this fills and returns it. A nil Go map crosses as the
// empty bento map, because a bento Map has no nil, so a Go function returning nil and
// one returning an empty map both hand back an empty map. Go map iteration order is
// unspecified, so the bento map's insertion order after the crossing is unspecified
// too, which section 6.5 fixes as the contract: a Map from a Go map has no promised
// order. Any per-entry range check (a map value through Int64ToNumber) runs inside
// valConv, so the same safe-integer guarantee the scalar crossing gives applies to
// every value.
func MapFromGo[GK comparable, GV any, BK any, BV any](m map[GK]GV, dst *value.Map[BK, BV], keyConv func(GK) BK, valConv func(GV) BV) *value.Map[BK, BV] {
	for k, v := range m {
		dst.Set(keyConv(k), valConv(v))
	}
	return dst
}

// BytesToGo marshals a bento Uint8Array to a Go []byte for a parameter of type
// []byte, the copy-on-uncertainty default of section 7.3. It copies the buffer's
// bytes into a fresh Go slice, so a Go function that retains or mutates the slice
// past the call can never alias bento's buffer, which is the safe crossing when the
// callee's retention is not known. The lowerer emits this by default and reaches for
// BytesToGoShared only where it can prove the callee reads the bytes within the call
// and does not keep them. A nil array (a shape the buffer model does not produce)
// crosses as a nil slice, so a Go function that branches on nil sees it; an empty
// buffer crosses as an empty non-nil slice.
func BytesToGo(a *value.Uint8Array) []byte {
	if a == nil {
		return nil
	}
	src := a.Bytes()
	out := make([]byte, len(src))
	copy(out, src)
	return out
}

// BytesToGoShared marshals a bento Uint8Array to a Go []byte by handing over the
// buffer's own backing slice with no copy, the zero-copy fast path of section 7.3.
// It is sound only when the Go callee reads the bytes within the call and does not
// retain or mutate them past return, the large set of read-only byte APIs
// (sha256.Sum256, an io.Writer.Write that copies), so the lowerer emits it in place
// of BytesToGo only where it can prove that. A nil array crosses as a nil slice, the
// same as the copying form.
func BytesToGoShared(a *value.Uint8Array) []byte {
	if a == nil {
		return nil
	}
	return a.Bytes()
}

// BytesFromGo marshals a Go []byte returned from a go: call to a bento Uint8Array,
// the copy-on-uncertainty default of section 7.3. It copies the Go slice into a
// fresh buffer, so a Go function that keeps the slice and mutates it after return
// cannot change bytes the bento program now owns. A nil Go slice crosses as an empty
// buffer, because a bento Uint8Array has no nil, so a Go function returning nil and
// one returning an empty slice both hand back an empty buffer.
func BytesFromGo(b []byte) *value.Uint8Array {
	out := make([]byte, len(b))
	copy(out, b)
	return value.Uint8ArrayFromGo(out)
}

// BytesFromGoShared marshals a Go []byte returned from a go: call to a bento
// Uint8Array by adopting the Go slice with no copy, the zero-copy fast path of
// section 7.3. It is sound only when Go will not mutate the slice after return, so
// the lowerer emits it in place of BytesFromGo only where it can prove that; the
// adopted slice is then bento's to own and the tracing GC keeps it alive. A nil Go
// slice crosses as an empty buffer, the same as the copying form.
func BytesFromGoShared(b []byte) *value.Uint8Array {
	return value.Uint8ArrayFromGo(b)
}

// Opaque is a token for a Go value the bridge does not project (section 6.13). It
// holds the real Go value the bento side received from one go: call and hands to
// another, never dereferenced: an option value, an unexported concrete type behind
// an interface, or a struct with no exported surface. Holding it as a Go interface
// keeps the value reachable, so the garbage collector keeps it alive as long as the
// bento program holds the token, which is the value model's GC integration the
// handle depends on (section 7.8). Every opaque handle crosses as this one type, so
// a bento local that holds a token has a single stable Go type whatever the foreign
// type is; the concrete type is recovered only where the token crosses back into Go.
type Opaque struct {
	v any
}

// OpaqueFromGo boxes a Go value returned from a go: call as an opaque token, the
// result crossing for a type the bridge does not project (section 6.13). The value
// is stored as-is and never inspected, so any Go type crosses.
func OpaqueFromGo[T any](v T) Opaque {
	return Opaque{v: v}
}

// OpaqueToGo recovers the Go value an opaque token holds for the crossing back into
// Go, where the emitted call names the concrete type as the type argument (section
// 6.13). The token only ever holds the type the signature says it does, because a
// bento program cannot construct one or change what it holds, so the assertion is
// sound.
func OpaqueToGo[T any](o Opaque) T {
	return o.v.(T)
}

// AnyToGo marshals a bento value into a Go any parameter, the empty-interface crossing
// of section 6.12. A scalar unwraps to the Go native a Go function inspecting the any
// expects: a nullish value to a nil interface, a boolean to a Go bool, a number to a
// float64, and a string to a Go string, so a type switch in the Go library matches the
// concrete case. A reference value (an object, an array, a function) has no native Go
// form, so it crosses as the value.Value box itself, which keeps its identity when a Go
// container stores it and hands it back through AnyFromGo (section 7.4). This is the
// value model's own boxing, named here as the boundary entry point.
func AnyToGo(v value.Value) any {
	switch v.Kind() {
	case value.KindUndefined, value.KindNull:
		return nil
	case value.KindBool:
		return v.AsBool()
	case value.KindNumber:
		return v.AsNumber()
	case value.KindString:
		return value.ToString(v).ToGoString()
	default:
		return v
	}
}

// AnyFromGo marshals a Go any result back to a bento value, the inverse of AnyToGo
// (section 6.12). A value the value model represents unboxes to its bento kind: a nil
// interface to null, a Go bool to a boolean, every Go integer and float to a number
// (the 64-bit widths through the same safe-integer check a static number result takes,
// section 7.5), and a Go string to a bento string. A value.Value that a bento value
// round-trips through a Go container passes through unchanged, so an object handed to Go
// as any and returned keeps its identity. A Go value of a type the value model cannot
// represent raises a boundary error rather than cross a shape bento has no box for: the
// opaque handle that would hold it (section 6.13) lives in a static go: result typed
// GoOpaque, not in a dynamic any, so a dynamic crossing has no token slot for it.
func AnyFromGo(v any) value.Value {
	switch x := v.(type) {
	case nil:
		return value.Null
	case value.Value:
		return x
	case value.BStr:
		return value.StringValue(x)
	case bool:
		return value.Bool(x)
	case string:
		return value.StringValue(value.FromGoString(x))
	case int:
		return value.Number(Int64ToNumber(int64(x)))
	case int8:
		return value.Number(float64(x))
	case int16:
		return value.Number(float64(x))
	case int32:
		return value.Number(float64(x))
	case int64:
		return value.Number(Int64ToNumber(x))
	case uint:
		return value.Number(Uint64ToNumber(uint64(x)))
	case uint8:
		return value.Number(float64(x))
	case uint16:
		return value.Number(float64(x))
	case uint32:
		return value.Number(float64(x))
	case uint64:
		return value.Number(Uint64ToNumber(x))
	case uintptr:
		return value.Number(Uint64ToNumber(uint64(x)))
	case float32:
		return value.Number(float64(x))
	case float64:
		return value.Number(x)
	default:
		panic(GoError{Err: fmt.Errorf("go: value of Go type %T returned as any has no bento projection", v)})
	}
}

// Must returns v, or raises when err is non-nil: the throw-mode bridge for a Go
// (T, error) result projected as a T that throws (section 6.6). A go: call whose
// Go signature ends in error lowers to a call to this, so the TypeScript author
// writes a call that returns the value and handles failure as a thrown error,
// exactly as they would for a JavaScript API.
func Must[T any](v T, err error) T {
	if err != nil {
		panic(GoError{Err: err})
	}
	return v
}

// Check raises when err is non-nil: the throw-mode bridge for a Go func that
// returns only error, the no-result sibling of Must.
func Check(err error) {
	if err != nil {
		panic(GoError{Err: err})
	}
}

// Guard runs a go: call and converts a Go panic that escapes the call into a
// thrown GoError, so a panic from the Go library the call entered becomes a
// catchable JavaScript exception (section 12.3). A value.Thrown panic (a deliberate
// bento throw, including the GoError the (T, error) bridge raises and the RangeError
// the number check raises) is left to keep unwinding, so a bento throw is not
// reclassified and a genuine bento runtime bug still surfaces as itself; only a
// panic that originates in the Go call is converted. It is the generic value-result
// form; Guard0 is the sibling for a call that returns nothing.
func Guard[T any](fn func() T) T {
	defer repanic()
	return fn()
}

// Guard0 is the void-result form of Guard, for a go: call whose Go function returns
// nothing and lowers to a statement.
func Guard0(fn func()) {
	defer repanic()
	fn()
}

// repanic is the deferred recover at the go: boundary: it lets a bento throw
// through unchanged and converts any other Go panic into a thrown GoError whose
// message is the panic's string form, so the loss of the call is reported the way
// every other boundary failure is. It calls recover directly, as a deferred
// function must, so Guard and Guard0 only name it in their defer.
func repanic() {
	r := recover()
	if r == nil {
		return
	}
	if _, ok := r.(value.Thrown); ok {
		panic(r)
	}
	if err, ok := r.(error); ok {
		panic(GoError{Err: err})
	}
	panic(GoError{Err: fmt.Errorf("go: call panicked: %v", r)})
}

// GoError is the value a failed go: call raises. It carries the original Go error,
// so once throw lowering lands (section 6.6) a bento catch surfaces it as the
// GoError of section 7.7 with errors.Is and errors.As still usable through Unwrap.
// Until then it is an ordinary panic whose message is the Go error string, matching
// how the rest of the runtime raises today.
type GoError struct{ Err error }

func (e GoError) Error() string { return e.Err.Error() }

// Unwrap exposes the wrapped Go error so errors.Is and errors.As reach it, which is
// what keeps Go's identity-based error handling usable across the boundary (section
// 7.7).
func (e GoError) Unwrap() error { return e.Err }

// ErrorName and ErrorMessage make a failed go: call a value.Thrown, so the same
// top-level handler that reports a program throw reports a boundary failure, and a
// catch tells it apart by name. A GoError projects as a JavaScript Error carrying
// the Go error's string (section 7.7).
func (e GoError) ErrorName() string    { return "Error" }
func (e GoError) ErrorMessage() string { return e.Err.Error() }

// RangeError is the value a boundary range check raises (section 7.5). It is a
// distinct type from GoError so a catch, once throw lands, can tell a numeric
// overflow apart from a returned Go error, mirroring JavaScript's RangeError.
type RangeError struct{ Message string }

func (e RangeError) Error() string { return e.Message }

// ErrorName and ErrorMessage make a boundary range check a value.Thrown, reported
// and caught as the RangeError it mirrors.
func (e RangeError) ErrorName() string    { return "RangeError" }
func (e RangeError) ErrorMessage() string { return e.Message }
