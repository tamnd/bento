package valuelab

// This is a labs prototype, not production code. It measures whether inline
// small-string storage (SSO) is worth adding to BStr. The open question is a
// tradeoff: SSO removes the heap allocation a short String.fromCharCode pays, but
// it grows the string value, which is passed by value everywhere, so every copy of
// a string costs more. These benchmarks model both sides on structs sized like the
// real BStr so the numbers transfer.
//
// plainBStr mirrors the real BStr's fields (a UTF-8 string header, a UTF-16 slice
// header, a length, and a rope pointer), so its size and copy cost match what
// bento ships today. ssoBStr adds an inline code-unit array and an inline length,
// the smallest SSO shape, so the delta between the two benchmarks is exactly what
// SSO would add to a copy and remove from a small construction.
//
// Result (Apple Silicon, go test -bench, non-inlinable constructors so the plain
// path heap-allocates like the real one):
//
//	PlainBuildRead    12.9 ns/op   1 alloc    heap slice
//	SSOBuildRead      14.5 ns/op   0 alloc    inline
//	PlainPassByValue   1.76 ns/op
//	SSOPassByValue     1.89 ns/op
//
// The verdict is that this bolt-on SSO does not pay. Adding the inline array grows
// the value from 56 to 80 bytes, and copying that larger struct on the constructor
// return costs more than the cheap eight-byte allocation it removes, so building is
// slower, not faster, even with the allocation gone. On top of that every pass by
// value gets about seven percent slower, and strings are passed by value all over
// the runtime. A small-string that actually wins would need a representation no
// larger than today's 56 bytes, a tagged union where the same bytes are either a
// pointer-length-cap or inline data, which Go cannot express without unsafe. That
// is a much larger and riskier redesign, so it is not worth doing on this evidence.

import "testing"

type ropeStub struct{ a, b, c uintptr }

// plainBStr is the shape bento ships: a short fromCharCode result lives in the
// heap-backed utf16 slice, so building one allocates.
type plainBStr struct {
	utf8      string
	utf16     []uint16
	lengthU16 int
	rope      *ropeStub
}

// Both constructors are marked non-inlinable (the directive below) so the built
// slice escapes across a call boundary the way value.FromCharCode does across the
// package boundary. Without it the benchmark inlines the whole build-and-read and
// escape analysis stack-allocates the slice, which is not the path a real program
// on the shipped BStr takes.
//
//go:noinline
func plainFromCodes(codes ...uint16) plainBStr {
	u := make([]uint16, len(codes))
	copy(u, codes)
	return plainBStr{utf16: u, lengthU16: len(u)}
}

func (s plainBStr) at(i int) uint16 { return s.utf16[i] }
func (s plainBStr) length() int     { return s.lengthU16 }

// ssoCap is the inline capacity in code units. Eight covers the fromcharcode
// result (four units) and most short strings while keeping the array small.
const ssoCap = 8

// ssoBStr is plainBStr plus inline storage. A result of ssoCap or fewer units
// lives in the value itself with no heap slice, so building one does not allocate;
// a larger one spills to the utf16 slice exactly as today.
type ssoBStr struct {
	utf8      string
	utf16     []uint16
	lengthU16 int
	rope      *ropeStub
	inline    [ssoCap]uint16
	inlineLen int8 // >=0 means the inline array holds the value
}

//go:noinline
func ssoFromCodes(codes ...uint16) ssoBStr {
	if len(codes) <= ssoCap {
		var s ssoBStr
		s.inlineLen = int8(len(codes))
		s.lengthU16 = len(codes)
		copy(s.inline[:], codes)
		return s
	}
	u := make([]uint16, len(codes))
	copy(u, codes)
	return ssoBStr{utf16: u, lengthU16: len(codes), inlineLen: -1}
}

func (s ssoBStr) at(i int) uint16 {
	if s.inlineLen >= 0 {
		return s.inline[i]
	}
	return s.utf16[i]
}
func (s ssoBStr) length() int { return s.lengthU16 }

// The four code units the fromcharcode workload builds each iteration.
func codesFor(i int) (uint16, uint16, uint16, uint16) {
	return uint16(65 + (i & 25)), uint16(97 + (i % 26)), uint16(65536 + (i & 127)), uint16(945 + (i % 24))
}

// BenchmarkPlainBuildRead is the fromcharcode construction-and-read on the shipped
// heap-backed shape: it allocates the slice every iteration.
func BenchmarkPlainBuildRead(b *testing.B) {
	var acc int
	i := 0
	for b.Loop() {
		a, c, d, e := codesFor(i)
		s := plainFromCodes(a, c, d, e)
		acc = acc*31 + int(s.at(0))
		acc = acc*31 + int(s.at(2))
		acc = acc*31 + s.length()
		i++
	}
	sinkLen = acc
}

// BenchmarkSSOBuildRead is the same construction-and-read on the inline shape: the
// four units live in the value, so no slice is allocated.
func BenchmarkSSOBuildRead(b *testing.B) {
	var acc int
	i := 0
	for b.Loop() {
		a, c, d, e := codesFor(i)
		s := ssoFromCodes(a, c, d, e)
		acc = acc*31 + int(s.at(0))
		acc = acc*31 + int(s.at(2))
		acc = acc*31 + s.length()
		i++
	}
	sinkLen = acc
}

//go:noinline
func consumePlain(s plainBStr) int { return s.lengthU16 }

//go:noinline
func consumeSSO(s ssoBStr) int { return s.lengthU16 }

// BenchmarkPlainPassByValue passes the shipped-size value across a call boundary in
// a tight loop, the cost SSO must not blow up: strings are handed to methods and
// helpers by value all over the runtime.
func BenchmarkPlainPassByValue(b *testing.B) {
	s := plainFromCodes(codesForSlice(64)...)
	var acc int
	for b.Loop() {
		acc += consumePlain(s)
	}
	sinkLen = acc
}

// BenchmarkSSOPassByValue passes the larger inline-bearing value the same way, so
// the gap to the plain benchmark is the per-copy tax SSO adds.
func BenchmarkSSOPassByValue(b *testing.B) {
	s := ssoFromCodes(codesForSlice(64)...)
	var acc int
	for b.Loop() {
		acc += consumeSSO(s)
	}
	sinkLen = acc
}

func codesForSlice(n int) []uint16 {
	u := make([]uint16, n)
	for i := range u {
		u[i] = uint16(65 + i)
	}
	return u
}
