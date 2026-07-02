package value

import "testing"

// BenchmarkStringsWorkload replicates the compute/strings.ts benchmark against
// the value runtime so a CPU profile shows which string operation dominates. It
// mirrors the workload exactly: build one big string with += in a loop, then
// split, join, and replace it a few times.
func BenchmarkStringsWorkload(b *testing.B) {
	const words = 8000
	const passes = 12
	space := FromGoString(" ")
	dash := FromGoString("-")
	word := FromGoString("word")
	searchWord := FromGoString("word")
	replW := FromGoString("W")
	for n := 0; n < b.N; n++ {
		s := FromGoString("")
		for i := 0; i < words; i++ {
			s = Concat(Concat(Concat(s, word), NumberToString(float64(i%100))), space)
		}
		count := 0.0
		for pass := 0; pass < passes; pass++ {
			parts := s.Split(space)
			count += parts.Len()
			joined := parts.Join(dash, func(x BStr) BStr { return x })
			count += joined.Length() - joined.ReplaceAll(searchWord, replW).Length()
		}
		if count == 0 {
			b.Fatal("count stayed zero")
		}
	}
}

// BenchmarkStringsBuild isolates the accumulation loop, the s += ... phase, so the
// rope's effect on the build is visible apart from the split/join/replace passes.
func BenchmarkStringsBuild(b *testing.B) {
	const words = 8000
	space := FromGoString(" ")
	word := FromGoString("word")
	for n := 0; n < b.N; n++ {
		s := FromGoString("")
		for i := 0; i < words; i++ {
			s = Concat(Concat(Concat(s, word), NumberToString(float64(i%100))), space)
		}
		if s.Length() == 0 {
			b.Fatal("empty")
		}
	}
}

// BenchmarkStringsPasses isolates the split/join/replace passes over a prebuilt
// string, so their cost is visible apart from the build.
func BenchmarkStringsPasses(b *testing.B) {
	const words = 8000
	const passes = 12
	space := FromGoString(" ")
	dash := FromGoString("-")
	word := FromGoString("word")
	searchWord := FromGoString("word")
	replW := FromGoString("W")
	s := FromGoString("")
	for i := 0; i < words; i++ {
		s = Concat(Concat(Concat(s, word), NumberToString(float64(i%100))), space)
	}
	s = s.flat()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		count := 0.0
		for pass := 0; pass < passes; pass++ {
			parts := s.Split(space)
			count += parts.Len()
			joined := parts.Join(dash, func(x BStr) BStr { return x })
			count += joined.Length() - joined.ReplaceAll(searchWord, replW).Length()
		}
		if count == 0 {
			b.Fatal("zero")
		}
	}
}
