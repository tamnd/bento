package valuelab

// The replace workload builds a short row, replaces the first separator with a
// pattern that carries $&, and rewrites every separator with a plain string, then
// folds a couple of lengths and code units into a checksum. Each of those steps
// allocates: the row is one built string, the two replace results are two more,
// and the number coercions inside the row add two. This file measures the pieces
// the runtime optimization moved, so the byte-wise $ substitution and the single
// ConcatN row build stay honestly benchmarked next to the whole loop body.

import (
	"testing"

	"github.com/tamnd/bento/pkg/value"
)

var (
	colon = value.FromGoString(":")
	amp   = value.FromGoString(" [$&] ")
	dash  = value.FromGoString("-")
	head  = value.FromGoString("a:")
	midB  = value.FromGoString(":b:")
	tailC = value.FromGoString(":c")
)

// buildRow builds "a:" + (i%97) + ":b:" + (i%13) + ":c" through one ConcatN, the
// shape the lowerer emits for a chain of three or more string + operands, so the
// row is one strings.Builder pass rather than four pairwise Concat allocations.
func buildRow(i int32) value.BStr {
	return head.ConcatN(
		value.NumberToString(float64(i%97)),
		midB,
		value.NumberToString(float64(i%13)),
		tailC,
	)
}

// BenchmarkReplaceDollarSub times a single Replace whose replacement carries the
// $& pattern, the case that used to fall off the UTF-8 fast path into a full
// utf16.Encode of the receiver. It should now splice on bytes with no code-unit
// materialization.
func BenchmarkReplaceDollarSub(b *testing.B) {
	row := buildRow(1234)
	var s value.BStr
	for b.Loop() {
		s = row.Replace(colon, amp)
	}
	sinkStr = s
}

// BenchmarkReplaceAllPlain times a ReplaceAll with a plain replacement, the fast
// path that rewrites every separator on bytes.
func BenchmarkReplaceAllPlain(b *testing.B) {
	row := buildRow(1234)
	var s value.BStr
	for b.Loop() {
		s = row.ReplaceAll(colon, dash)
	}
	sinkStr = s
}

// BenchmarkReplaceLoopBody mirrors the whole workload inner loop: build the row,
// run both replaces, and fold the lengths and sampled code units into the same
// int32 checksum, so the per-iteration allocation total the workload pays stays
// visible as one number.
func BenchmarkReplaceLoopBody(b *testing.B) {
	var acc int32
	i := int32(1)
	for b.Loop() {
		row := buildRow(i)
		first := row.Replace(colon, amp)
		all := row.ReplaceAll(colon, dash)
		acc = value.ToInt32(float64(acc)*31 + first.Length())
		acc = value.ToInt32(float64(acc)*31 + all.Length())
		acc = value.ToInt32(float64(acc)*31 + all.CharCodeAt(0))
		acc = value.ToInt32(float64(acc)*31 + first.CharCodeAt(first.Length()-1))
		i++
		if i >= 3000 {
			i = 1
		}
	}
	sinkLen = int(acc)
}
