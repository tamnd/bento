package lower

import (
	"strings"
	"testing"
)

// TestInt64SpecializationBigSum renders the big-sum kernel and pins the shape the
// int64 tier produces: the accumulator is a Go int64, the squared counter is a
// native int64 multiply over the counter read into the integer domain, and the
// write collapses to the compound += a developer would write. The sum tops out
// near 3.3e14, past int32 but inside the safe-integer range, which is exactly the
// value set this tier exists for.
func TestInt64SpecializationBigSum(t *testing.T) {
	src := `
let sum = 0;
for (let i = 1; i <= 100000; i++) {
  sum = sum + i * i;
}
console.log(sum);
`
	got := renderProgram(t, src)

	wants := []string{
		"var sum int64 = 0",
		"sum += int64(i) * int64(i)",
		"float64(sum)",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("emitted Go missing %q\n---\n%s", w, got)
		}
	}
}

// TestInt64SpecializationWideConstant proves the base case of the tier: a local
// initialized to an integer literal past the int32 range but inside the safe
// range declares as int64 with the literal folded straight in.
func TestInt64SpecializationWideConstant(t *testing.T) {
	src := `
let big = 5000000000;
big = big + 1;
console.log(big);
`
	got := renderProgram(t, src)
	if !strings.Contains(got, "var big int64 = 5000000000") {
		t.Errorf("emitted Go missing the int64 declaration\n---\n%s", got)
	}
	if !strings.Contains(got, "big++") {
		t.Errorf("emitted Go missing the native increment\n---\n%s", got)
	}
}

// TestInt64SpecializationRejectsOverflowRisk proves the drift proof is real: the
// same accumulator under a bound whose worst-case sum passes 2^53 stays a
// float64, because the double the language computes would round there and the
// int64 form would not.
func TestInt64SpecializationRejectsOverflowRisk(t *testing.T) {
	src := `
let sum = 0;
for (let i = 1; i <= 2000000; i++) {
  sum = sum + i * i * i;
}
console.log(sum);
`
	got := renderProgram(t, src)
	if strings.Contains(got, "var sum int64") {
		t.Errorf("an accumulator whose drift can pass 2^53 must stay float64\n---\n%s", got)
	}
	if !strings.Contains(got, "var sum float64 = 0") && !strings.Contains(got, "sum := 0.0") {
		t.Errorf("expected the float64 fallback declaration\n---\n%s", got)
	}
}

// TestInt64SpecializationRejectsUnboundedLoop proves an accumulator under a loop
// with no proven trip count is rejected: a while loop can run the write any
// number of times, so no drift bound exists.
func TestInt64SpecializationRejectsUnboundedLoop(t *testing.T) {
	src := `
let sum = 0;
let n = 0;
while (n < 10) {
  sum = sum + 5000000000;
  n = n + 1;
}
console.log(sum);
`
	got := renderProgram(t, src)
	if strings.Contains(got, "var sum int64") {
		t.Errorf("an accumulator under a while loop must stay float64\n---\n%s", got)
	}
}

// TestInt64SpecializationSkipsNarrowValues proves the tier does not claim locals
// the int32 range already covers: a small sum keeps its float64 lowering (or the
// int32 tier's, when eligible), so this tier only fires past 32 bits and the
// existing goldens do not churn.
func TestInt64SpecializationSkipsNarrowValues(t *testing.T) {
	src := `
let sum = 0;
for (let i = 1; i <= 100; i++) {
  sum = sum + i;
}
console.log(sum);
`
	got := renderProgram(t, src)
	if strings.Contains(got, "int64") {
		t.Errorf("a sum inside the int32 range must not take the int64 tier\n---\n%s", got)
	}
}

// TestInt64SpecializationRejectsFractional proves a write outside the integer
// domain disqualifies the local: a fractional delta has no interval in this
// analysis, so the local keeps the float64 the value genuinely needs.
func TestInt64SpecializationRejectsFractional(t *testing.T) {
	src := `
let x = 5000000000;
x = x + 0.5;
console.log(x);
`
	got := renderProgram(t, src)
	if strings.Contains(got, "var x int64") {
		t.Errorf("a local written a fraction must stay float64\n---\n%s", got)
	}
}
