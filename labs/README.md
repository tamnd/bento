# labs

Micro-benchmarks for the value runtime.

The bento-bench suite measures whole programs end to end: it compiles a workload,
runs the binary, and reads the compute time the program prints. That is the right
tool for a headline number, but it is slow to iterate on and it mixes the cost of
one hot function in with everything else the workload does. labs is the finer
grained companion. Each file here isolates a single runtime path as a Go
`testing.B` benchmark so a change to a coercion or a Math method can be measured in
milliseconds, on the function alone, before it is wired through the lowerer and
re-run through the full harness.

The benchmarks come in pairs where a change is in flight: one benchmark mirrors
the Go the lowerer emits today, the other is the shape an optimization produces.
The distance between the two is the headroom, and a paired test asserts both
shapes still agree on the checksum the workload must keep. That way the library
records not just how fast a path is but how fast it could be and why the target
was chosen.

## Running

```
go test ./labs/... -bench . -benchtime 200x -run xxx
```

Add `-benchmem` for allocation counts, or name one benchmark with `-bench
BenchmarkMathbitsNative`. The `-run xxx` skips the correctness tests during a
timing run; drop it to check the kernels still produce the workload checksum.

## What is here

- `value/mathbits_bench_test.go` pairs the mathbits inner loop as emitted today
  (float64 held values, every bit op coerced through ToInt32 or ToUint32) against
  the same loop over int32. The lowered form is about twelve times slower, which
  is the case for teaching the lowerer to keep integer locals in int32.
- `value/coerce_bench_test.go` times the primitives the integer workloads lean on
  one call at a time: ToInt32, ToUint32, Imul, Clz32, and a native Clz32 for the
  floor.
- `value/fib_bench_test.go` pairs recursive fib over float64 against int32. The
  gap is small on purpose: fib is at the Go call ceiling, so it is the evidence
  that keeps the optimization effort on the integer-arithmetic workloads and off
  the call-bound ones.
