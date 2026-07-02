package valuelab

// The templates workload builds a row per iteration from a head, a number, a word
// from a small table, another number, and a boolean, then folds a couple of lengths
// and code units into a checksum. The build is where the allocation lives: the join
// form coerces each number and the boolean to its own String(x) string before
// joining, so a loop throws away four intermediate strings an iteration on top of
// the result. This file measures the same build two ways, the ConcatN join the
// lowerer used to emit and the reused StrBuilder it emits now, so the win the
// runtime change buys stays honestly benchmarked next to the whole loop body.

import (
	"testing"

	"github.com/tamnd/bento/pkg/value"
)

var (
	tmplHead = value.FromGoString("row ")
	tmplC1   = value.FromGoString(": ")
	tmplC2   = value.FromGoString(" = ")
	tmplC3   = value.FromGoString(" (")
	tmplTail = value.FromGoString(")")
	tmplWord = []value.BStr{
		value.FromGoString("alpha"),
		value.FromGoString("beta"),
		value.FromGoString("gamma"),
		value.FromGoString("delta"),
	}
)

// buildRowConcat builds the row through one ConcatN over the head, the shape the
// lowerer emitted before the StrBuilder change: every number and the boolean is
// coerced to its own String(x) BStr first, so the build allocates the four
// intermediate strings plus the result.
func buildRowConcat(i int32) value.BStr {
	return tmplHead.ConcatN(
		value.NumberToString(float64(i)),
		tmplC1,
		tmplWord[i&3],
		tmplC2,
		value.NumberToString(float64(i)*1.5),
		tmplC3,
		value.BoolToString((i&1) == 0),
		tmplTail,
	)
}

// buildRowBuilder builds the same row through a reused StrBuilder, the shape the
// lowerer emits now: each part appends straight into the builder's buffer with no
// intermediate coercion string, so the build allocates only its result and the
// builder's buffer is reused across calls.
func buildRowBuilder(sb *value.StrBuilder, i int32) value.BStr {
	return sb.Reset().
		Lit("row ", 4).
		Num(float64(i)).
		Lit(": ", 2).
		Str(tmplWord[i&3]).
		Lit(" = ", 3).
		Num(float64(i) * 1.5).
		Lit(" (", 2).
		Bool((i & 1) == 0).
		Lit(")", 1).
		Done()
}

// BenchmarkTemplateConcat times the join build alone, the four coercion strings and
// the result per call.
func BenchmarkTemplateConcat(b *testing.B) {
	var s value.BStr
	i := int32(1)
	for b.Loop() {
		s = buildRowConcat(i)
		i++
		if i >= 3000 {
			i = 1
		}
	}
	sinkStr = s
}

// BenchmarkTemplateBuilder times the builder build alone, one reused builder and the
// result per call, so the removed intermediate strings show up as the difference
// from the join.
func BenchmarkTemplateBuilder(b *testing.B) {
	var sb value.StrBuilder
	var s value.BStr
	i := int32(1)
	for b.Loop() {
		s = buildRowBuilder(&sb, i)
		i++
		if i >= 3000 {
			i = 1
		}
	}
	sinkStr = s
}

// BenchmarkTemplateLoopBody mirrors the whole workload inner loop over the builder
// build: build the row, then fold its length and the code units at both ends into
// the same int32 checksum, so the per-iteration cost the workload pays stays visible
// as one number.
func BenchmarkTemplateLoopBody(b *testing.B) {
	var sb value.StrBuilder
	var acc int32
	i := int32(1)
	for b.Loop() {
		s := buildRowBuilder(&sb, i)
		acc = value.ToInt32(float64(acc)*31 + s.Length())
		acc = value.ToInt32(float64(acc)*31 + s.CharCodeAt(0))
		acc = value.ToInt32(float64(acc)*31 + s.CharCodeAt(s.Length()-1))
		i++
		if i >= 3000 {
			i = 1
		}
	}
	sinkLen = int(acc)
}
