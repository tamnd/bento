package lower

import (
	"strings"
	"testing"
)

// TestBigIntAsIntNLowers pins that BigInt.asIntN(bits, x) lowers to the value signed
// wrap helper, with the width as a number and the value as a bigint.
func TestBigIntAsIntNLowers(t *testing.T) {
	const src = "export function f(x: bigint): bigint { return BigInt.asIntN(8, x); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.BigIntAsIntN(8, x)") {
		t.Errorf("BigInt.asIntN did not lower to the value wrap helper:\n%s", source)
	}
}

// TestBigIntAsUintNLowers pins that BigInt.asUintN(bits, x) lowers to the value
// unsigned wrap helper.
func TestBigIntAsUintNLowers(t *testing.T) {
	const src = "export function f(x: bigint): bigint { return BigInt.asUintN(16, x); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.BigIntAsUintN(16, x)") {
		t.Errorf("BigInt.asUintN did not lower to the value wrap helper:\n%s", source)
	}
}

// TestBigIntWrapRuns builds and runs the generated Go, proving the signed wrap folds
// the top half of the range to negatives and the unsigned wrap keeps everything in
// range.
func TestBigIntWrapRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function run(): void {
  console.log(BigInt.asIntN(8, 255n));
  console.log(BigInt.asIntN(8, 128n));
  console.log(BigInt.asIntN(8, 127n));
  console.log(BigInt.asUintN(16, -1n));
  console.log(BigInt.asUintN(16, 70000n));
}
run();
`
	if got, want := runProgramGo(t, src), "-1n\n-128n\n127n\n65535n\n4464n\n"; got != want {
		t.Fatalf("bigint wrap printed %q, want %q", got, want)
	}
}

// TestBigIntValueOfIsIdentity pins that b.valueOf() lowers to the receiver with no
// call, since a bigint's valueOf returns the bigint itself.
func TestBigIntValueOfIsIdentity(t *testing.T) {
	const src = "export function f(x: bigint): bigint { return x.valueOf(); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "return x\n") {
		t.Errorf("bigint valueOf did not lower to the receiver identity:\n%s", source)
	}
}

// TestBigIntToStringLowers pins that b.toString() renders the decimal digits and
// b.toString(radix) renders the digits in the named base.
func TestBigIntToStringLowers(t *testing.T) {
	const decimal = "export function f(x: bigint): string { return x.toString(); }\n"
	if source := renderProgram(t, decimal); !strings.Contains(source, "value.BigIntToString(x)") {
		t.Errorf("bigint toString did not lower to the decimal render:\n%s", source)
	}
	const radix = "export function f(x: bigint): string { return x.toString(16); }\n"
	if source := renderProgram(t, radix); !strings.Contains(source, "value.BigIntToStringRadix(x, 16)") {
		t.Errorf("bigint toString(radix) did not lower to the radix render:\n%s", source)
	}
}

// TestBigIntStringRuns builds and runs the generated Go, proving toString renders
// the digits in the named base with the same lowercase digits and leading minus V8
// uses, and valueOf reads back the same value.
func TestBigIntStringRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function run(): void {
  console.log((255n).toString());
  console.log((255n).toString(16));
  console.log((255n).toString(2));
  console.log((-255n).toString(16));
  console.log((123n).valueOf());
}
run();
`
	if got, want := runProgramGo(t, src), "255\nff\n11111111\n-ff\n123n\n"; got != want {
		t.Fatalf("bigint string printed %q, want %q", got, want)
	}
}
