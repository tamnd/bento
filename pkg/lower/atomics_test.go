package lower

import (
	"strings"
	"testing"
)

// TestAtomicsReadModifyWriteLoweringShape pins the Go the read, write, and
// read-modify-write Atomics operations lower to: each picks the matching value function
// over the typed array it takes, with the index and value operands passed through.
func TestAtomicsReadModifyWriteLoweringShape(t *testing.T) {
	const src = `const ta = new Int32Array(new SharedArrayBuffer(16));
Atomics.store(ta, 0, 10);
console.log(Atomics.load(ta, 0));
console.log(Atomics.add(ta, 0, 5));
console.log(Atomics.sub(ta, 0, 2));
console.log(Atomics.and(ta, 0, 12));
console.log(Atomics.or(ta, 0, 1));
console.log(Atomics.xor(ta, 0, 3));
console.log(Atomics.exchange(ta, 0, 99));
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"value.AtomicStore(ta, 0, 10)",
		"value.AtomicLoad(ta, 0)",
		"value.AtomicAdd(ta, 0, 5)",
		"value.AtomicSub(ta, 0, 2)",
		"value.AtomicAnd(ta, 0, 12)",
		"value.AtomicOr(ta, 0, 1)",
		"value.AtomicXor(ta, 0, 3)",
		"value.AtomicExchange(ta, 0, 99)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("Atomics lowering missing %q:\n%s", want, source)
		}
	}
}

// TestAtomicsHandsBackUnsupportedForms proves the Atomics lowering claims only the
// subset it can emit soundly. A bigint typed array stores a *big.Int outside the float
// AtomicView, and a non-number index is not the numeric operand the value functions
// take, so each hands back rather than emitting wrong Go.
func TestAtomicsHandsBackUnsupportedForms(t *testing.T) {
	handsBack(t, "const ta = new BigInt64Array(new SharedArrayBuffer(16)); console.log(Atomics.add(ta, 0, 1n));\n")
	handsBack(t, "const ta = new Int32Array(new SharedArrayBuffer(16)); const i: any = 0; console.log(Atomics.load(ta, i));\n")
}
