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

import "github.com/tamnd/bento/pkg/value"

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

// RangeError is the value a boundary range check raises (section 7.5). It is a
// distinct type from GoError so a catch, once throw lands, can tell a numeric
// overflow apart from a returned Go error, mirroring JavaScript's RangeError.
type RangeError struct{ Message string }

func (e RangeError) Error() string { return e.Message }
