package lower

import (
	"strings"
	"testing"
)

// TestBigIntMixedOperandHandsBack pins that a bigint arithmetic operator whose operand is
// a mixed subexpression, an untyped value the checker widens to bigint through a literal,
// hands the program back rather than emit a new(big.Int) call over the float64 the number
// path lowers that operand to. The year-format Temporal tests hit this with
// (year - 1970n) * bigintConst where year is an untyped parameter every call passes a
// bigint; before the guard it emitted new(big.Int).Mul(<float64>, ...), which does not
// compile.
func TestBigIntMixedOperandHandsBack(t *testing.T) {
	const src = `function epochNsInYear(year: any) {
  const avgNsPerYear: bigint = 31556952000000000n;
  return (year - 1970n) * avgNsPerYear;
}
console.log(epochNsInYear(0n));
`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "big.Int") {
		t.Errorf("handback reason = %q, want it to name the big.Int operand mismatch", reason)
	}
}

// TestBigIntSoundCompoundOperandStillLowers pins that a pure-bigint compound operand still
// lowers to the *big.Int method form; the soundness guard fires only when a number sneaks
// into the subexpression, so a genuine (a - b) * c over bigints is untouched.
func TestBigIntSoundCompoundOperandStillLowers(t *testing.T) {
	const src = `const a: bigint = 6n;
const b: bigint = 4n;
const c: bigint = 2n;
console.log((a - b) * c);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "new(big.Int).Mul") {
		t.Errorf("pure-bigint compound did not lower to Mul:\n%s", source)
	}
}
