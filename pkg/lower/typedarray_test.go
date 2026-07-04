package lower

import (
	"strings"
	"testing"
)

// TestTypedArrayLoweringShape pins the Go a numeric typed-array program lowers to:
// a construction from a length is value.New<Name>, a construction from a number
// list is value.<Name>Of, an indexed read is At, an indexed write is SetAt,
// .length is Len, and a local that holds a buffer is typed *value.TypedArray[T]
// over the element's Go type. The element read and write and length shapes match
// the byte buffer's, so only the receiver Go type and the constructor names differ
// across the family; this test reads the emitted code directly so a change to the
// shape is visible in review without running the toolchain.
func TestTypedArrayLoweringShape(t *testing.T) {
	const src = `const buf = new Int32Array(3);
buf[0] = 65;
buf[1] = buf[0] + 1;
const lit = new Int32Array([9, 8, 7]);
console.log(buf.length);
console.log(buf[1]);
console.log(lit.length);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"buf := value.NewInt32Array(",
		"lit := value.Int32ArrayOf(",
		"buf.SetAt(",
		"buf.At(",
		"buf.Len()",
		"lit.Len()",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("Int32Array lowering missing %q:\n%s", want, source)
		}
	}
}

// TestTypedArrayElementTypes pins that each numeric family member lowers to a
// value.TypedArray over its own Go element type, so the store width the checker
// proved is the width the generated buffer holds. Uint8ClampedArray stores a uint8
// through the generic buffer with the clamp coercion, distinct from the Uint8Array
// byte buffer, so both spell a uint8 element but through different receivers.
func TestTypedArrayElementTypes(t *testing.T) {
	cases := map[string]string{
		"Int8Array":         "*value.TypedArray[int8]",
		"Uint8ClampedArray": "*value.TypedArray[uint8]",
		"Int16Array":        "*value.TypedArray[int16]",
		"Uint16Array":       "*value.TypedArray[uint16]",
		"Int32Array":        "*value.TypedArray[int32]",
		"Uint32Array":       "*value.TypedArray[uint32]",
		"Float32Array":      "*value.TypedArray[float32]",
		"Float64Array":      "*value.TypedArray[float64]",
	}
	for name, want := range cases {
		src := "function make(): " + name + " {\n  return new " + name + "(4);\n}\nconsole.log(make().length);\n"
		source := renderProgram(t, src)
		if !strings.Contains(source, want) {
			t.Errorf("%s did not lower its element type to %q:\n%s", name, want, source)
		}
		if !strings.Contains(source, "value.New"+name+"(") {
			t.Errorf("%s construction did not lower to value.New%s\n%s", name, name, source)
		}
	}
}

// TestTypedArrayConstructionForms pins the two covered constructor forms for a
// family member: a length allocates a zeroed buffer, a number list fills one. The
// list form passes each element straight through, so a value outside the element's
// range rides to the element kind's store coercion rather than being folded here.
func TestTypedArrayConstructionForms(t *testing.T) {
	cases := map[string]string{
		`const b = new Uint8ClampedArray(8); console.log(b.length);`:         "value.NewUint8ClampedArray(",
		`const b = new Uint8ClampedArray([300, -1]); console.log(b.length);`: "value.Uint8ClampedArrayOf(",
		`const b = new Float64Array([1.5, 2.5]); console.log(b.length);`:     "value.Float64ArrayOf(",
	}
	for src, want := range cases {
		source := renderProgram(t, src+"\n")
		if !strings.Contains(source, want) {
			t.Errorf("%q did not lower to %q:\n%s", src, want, source)
		}
	}
}

// TestTypedArrayHandsBackUnsupportedForms proves the typed-array lowering claims
// only the subset it can emit soundly and hands the rest back to the engine, the
// same boundaries the byte buffer keeps. A compound element write reads and writes
// the element, which the plain SetAt store does not model. A method that has no
// lowering yet (subarray builds a view) and the view-over-a-buffer constructor
// overload are later slices. A bigint-element array (BigInt64Array) has no
// lowering, so its construction hands back rather than emitting wrong Go.
func TestTypedArrayHandsBackUnsupportedForms(t *testing.T) {
	handsBack(t, "const b = new Int32Array(3); b[0] += 1; console.log(b[0]);\n")
	handsBack(t, "const b = new Int32Array(3); const c = b.subarray(1); console.log(c.length);\n")
	handsBack(t, "const buf = new ArrayBuffer(8); const b = new Int32Array(buf); console.log(b.length);\n")
	handsBack(t, "const b = new BigInt64Array(3); console.log(b.length);\n")
}

// TestTypedArrayIntIndexLowering pins the native-int index form. A typed-array read
// and write driven by a bounded for-counter lower through AtI and SetAtI with the
// index narrowed to a Go int, so the counter stays a native int32 and the float
// truncation At and SetAt run on a Number index is dropped. The index arithmetic a
// loop takes, b[i - 1], rides the same int form. A constant index keeps At, since it
// gains nothing from the int form, and a dynamic index that the checker cannot prove
// integer also keeps At.
func TestTypedArrayIntIndexLowering(t *testing.T) {
	const src = `const b = new Int32Array(8);
for (let i = 1; i < 8; i++) {
  b[i] = b[i - 1] + 1;
}
console.log(b[7]);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"var i int32 = 1",
		"b.SetAtI(int(i), b.AtI(int(i-1))+1)",
		"b.At(7)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("int-index lowering missing %q:\n%s", want, source)
		}
	}
	if strings.Contains(source, "b.SetAt(") {
		t.Errorf("counter-driven write should not use the float SetAt:\n%s", source)
	}
}

// TestTypedArrayIntIndexKeepsFloatForm pins that an index the checker does not prove
// integer keeps the float At. A loop counter written a fractional value never
// specializes, so its index stays a Number and the read truncates it through At.
func TestTypedArrayIntIndexKeepsFloatForm(t *testing.T) {
	const src = `const b = new Int32Array(8);
let x = 0.5;
console.log(b[x]);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "b.At(") {
		t.Errorf("a non-integer index should read through the float At:\n%s", source)
	}
	if strings.Contains(source, "b.AtI(") {
		t.Errorf("a non-integer index should not use the native-int AtI:\n%s", source)
	}
}

// TestArrayIntIndexReadLowering pins that a dense array read driven by a bounded
// for-counter lowers through AtI too, since the same float truncation is dead work
// there. The dense-array write keeps Set, whose out-of-range store grows the array
// the way a JavaScript sparse assignment does, so it is not part of this slice.
func TestArrayIntIndexReadLowering(t *testing.T) {
	const src = `const a: number[] = [1, 2, 3, 4];
let s = 0;
for (let i = 0; i < 4; i++) {
  s = s + a[i];
}
console.log(s);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "a.AtI(int(i))") {
		t.Errorf("dense-array read under a counter should use AtI:\n%s", source)
	}
}

// TestTypedArrayRuns builds and runs a program over several family members and
// checks the output matches the JavaScript store semantics: an Int8Array write
// wraps modulo 256 into signed range, a Uint8ClampedArray clamps out-of-range
// values, and a Float64Array keeps the Number exactly.
func TestTypedArrayRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const i8 = new Int8Array(2);
i8[0] = 200;
i8[1] = -1;
const clamped = new Uint8ClampedArray([300, -5, 128]);
const f = new Float64Array([0.5, 1.25]);
console.log(i8[0]);
console.log(i8[1]);
console.log(clamped[0]);
console.log(clamped[1]);
console.log(clamped[2]);
console.log(f[0] + f[1]);
`
	out := runProgramGo(t, src)
	const want = "-56\n-1\n255\n0\n128\n1.75\n"
	if out != want {
		t.Fatalf("typed-array run output = %q, want %q", out, want)
	}
}
