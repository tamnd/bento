package valuelab

// Recursive Fibonacci is the call-throughput benchmark: it does almost no
// arithmetic per call and almost no allocation, so it measures the cost of the
// call itself. The lowerer already emits fib as a plain Go function over float64,
// which is why bento leads the workload. These two benchmarks record what integer
// type specialization would and would not buy here: fibInt is the same recursion
// over int32. The gap is small, which is the point. fib is at the Go call ceiling,
// so it is not where specialization pays off, and this benchmark is the evidence
// that keeps the effort pointed at the integer-arithmetic workloads instead.

import "testing"

func fibFloat(n float64) float64 {
	if n < 2 {
		return n
	}
	return fibFloat(n-1) + fibFloat(n-2)
}

func fibInt(n int32) int32 {
	if n < 2 {
		return n
	}
	return fibInt(n-1) + fibInt(n-2)
}

func BenchmarkFibFloat(b *testing.B) {
	var acc float64
	for b.Loop() {
		acc += fibFloat(32)
	}
	sinkF64 = acc
}

func BenchmarkFibInt(b *testing.B) {
	var acc int32
	for b.Loop() {
		acc += fibInt(32)
	}
	sinkI32 = acc
}
