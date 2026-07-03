package lower

import (
	"strings"
	"testing"
)

// TestInt32SpecializationMathbits renders the mathbits kernel and pins the shape
// the specialization produces: the accumulator and the loop counter are Go int32,
// the imul mix is a native multiply against the folded ToInt32 of the constant, the
// clz32 of the counter is Clz32U over a uint32 reinterpret with no float round trip,
// and the identity | 0 is dropped. If any of these regressed, the integer workload
// would fall back to the coercing float64 lowering and lose the speedup.
func TestInt32SpecializationMathbits(t *testing.T) {
	src := `
let acc = 0;
for (let i = 1; i < 5000; i++) {
  acc = Math.imul(acc ^ i, 2654435761);
  acc = (acc + Math.clz32(i)) | 0;
  const f = Math.fround(i * 1.1);
  acc = (acc + Math.clz32(f)) | 0;
}
console.log(acc | 0);
`
	got := renderProgram(t, src)

	wants := []string{
		"var acc int32 = 0",
		"var i int32 = 1",
		"acc = (acc ^ i) * -1640531535",
		"value.Clz32U(uint32(i))",
		"value.Clz32U(value.ToUint32(f))",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("emitted Go missing %q\n---\n%s", w, got)
		}
	}

	// The specialized writes must not reach for the non-inlined float64 helpers the
	// coercing lowering would emit, since those calls are the overhead the whole
	// change removes.
	notWants := []string{
		"value.Imul(",
		"value.Clz32(", // the float64 clz32, distinct from Clz32U
	}
	for _, nw := range notWants {
		if strings.Contains(got, nw) {
			t.Errorf("emitted Go still contains %q, the coercing form the specialization replaces\n---\n%s", nw, got)
		}
	}

	// The read side stays transparent: acc is read back as a float64 wherever a number
	// is expected, so the trailing print sees the same value model as before.
	if !strings.Contains(got, "float64(acc)") {
		t.Errorf("emitted Go does not read acc back as float64(acc)\n---\n%s", got)
	}
}

// TestInt32SpecializationSkipsFractional proves the analysis is conservative: a
// local that is ever written a fractional value cannot be int32, so it keeps its
// float64 type even though it is also used in a bitwise operator. Specializing it
// would truncate the fraction and diverge from the engine.
func TestInt32SpecializationSkipsFractional(t *testing.T) {
	src := `
let x = 0;
x = 1.5;
const y = x | 0;
console.log(y);
`
	got := renderProgram(t, src)
	// x := 0.0 is the float64 short declaration the readability fold emits for a plain
	// number local, so seeing it proves x kept its float64 type rather than being
	// specialized to int32.
	if !strings.Contains(got, "x := 0.0") {
		t.Errorf("a local written a fractional value should stay float64\n---\n%s", got)
	}
	if strings.Contains(got, "var x int32") || strings.Contains(got, "x := int32") {
		t.Errorf("a local written a fractional value was wrongly specialized to int32\n---\n%s", got)
	}
}

// TestInt32SpecializationNeedsIntegerUse proves a plain number local with no
// integer use is left a float64. Specializing a value that only ever feeds float
// arithmetic would buy nothing and only add the read-side wrap, so the analysis
// requires at least one bitwise or Math.imul/clz32 use before it commits.
func TestInt32SpecializationNeedsIntegerUse(t *testing.T) {
	src := `
let sum = 0;
for (let k = 0; k < 10; k++) {
  sum = sum + k;
}
console.log(sum);
`
	got := renderProgram(t, src)
	if strings.Contains(got, "var sum int32") || strings.Contains(got, "var k int32") {
		t.Errorf("a local with no integer use should not be specialized to int32\n---\n%s", got)
	}
}
