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
	// The buffer length comes from a parameter, so the array is not a compile-time
	// fixed length and its accesses keep the checked At and SetAt rather than taking
	// the native slice path a proven fixed-length array does.
	const src = `function fill(n: number): number {
  const buf = new Int32Array(n);
  buf[0] = 65;
  buf[1] = buf[0] + 1;
  const lit = new Int32Array([9, 8, 7]);
  console.log(buf.length);
  console.log(buf[1]);
  console.log(lit.length);
  return buf[1];
}
console.log(fill(3));
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
// lowering yet (subarray builds a view) is a later slice. A bigint-element array
// (BigInt64Array) has no lowering, so its construction hands back rather than
// emitting wrong Go.
func TestTypedArrayHandsBackUnsupportedForms(t *testing.T) {
	handsBack(t, "const b = new Int32Array(3); b[0] += 1; console.log(b[0]);\n")
	handsBack(t, "const b = new Int32Array(3); const c = b.subarray(1); console.log(c.length);\n")
	handsBack(t, "const b = new BigInt64Array(3); console.log(b.length);\n")
}

// TestTypedArrayViewOverBufferLowering pins the ArrayBuffer overload of the
// constructor: a view over a buffer lowers to value.<Name>View, with the byte
// offset defaulting to a literal 0 when omitted and the length passed through when
// given. A Uint8Array view takes the same form over its own view constructor.
func TestTypedArrayViewOverBufferLowering(t *testing.T) {
	cases := map[string]string{
		`const buf = new ArrayBuffer(16); const b = new Int32Array(buf); console.log(b.length);`:       "value.Int32ArrayView(buf, 0)",
		`const buf = new ArrayBuffer(16); const b = new Int32Array(buf, 4); console.log(b.length);`:    "value.Int32ArrayView(buf, 4)",
		`const buf = new ArrayBuffer(16); const b = new Int32Array(buf, 4, 2); console.log(b.length);`: "value.Int32ArrayView(buf, 4, 2)",
		`const buf = new ArrayBuffer(8); const b = new Uint8Array(buf, 1, 4); console.log(b.length);`:  "value.Uint8ArrayView(buf, 1, 4)",
		`const buf = new ArrayBuffer(8); const b = new Float64Array(buf); console.log(b.length);`:      "value.Float64ArrayView(buf, 0)",
	}
	for src, want := range cases {
		source := renderProgram(t, src+"\n")
		if !strings.Contains(source, want) {
			t.Errorf("%q did not lower to %q:\n%s", src, want, source)
		}
	}
}

// TestTypedArrayFromSourceLowering pins the two copy sources: a number array value
// lowers to value.<Name>Of over the source's Elems spread, and another typed array
// lowers to value.<Name>Of over the source's Floats spread, each building a fresh
// buffer through the Of constructor. A plain array literal still lowers to the
// direct Of form rather than through a source read.
func TestTypedArrayFromSourceLowering(t *testing.T) {
	cases := map[string]string{
		`const src = [1, 2, 3]; const b = new Int32Array(src); console.log(b.length);`:                 "value.Int32ArrayOf(src.Elems()...)",
		`const src = new Int32Array([1, 2, 3]); const b = new Uint8Array(src); console.log(b.length);`: "value.Uint8ArrayOf(src.Floats()...)",
		`const src = new Uint8Array([1, 2]); const b = new Float64Array(src); console.log(b.length);`:  "value.Float64ArrayOf(src.Floats()...)",
	}
	for src, want := range cases {
		source := renderProgram(t, src+"\n")
		if !strings.Contains(source, want) {
			t.Errorf("%q did not lower to %q:\n%s", src, want, source)
		}
	}
}

// TestBytesPerElementLowering pins BYTES_PER_ELEMENT. The static read off the
// constructor folds to the element-width literal, since it has no receiver value.
// The instance read lowers to the view's BytesPerElement method instead, which keeps
// the receiver referenced so a binding read only through this stays used.
func TestBytesPerElementLowering(t *testing.T) {
	cases := map[string]string{
		`console.log(Int8Array.BYTES_PER_ELEMENT);`:                       "value.NumberToString(1)",
		`console.log(Int32Array.BYTES_PER_ELEMENT);`:                      "value.NumberToString(4)",
		`console.log(Float64Array.BYTES_PER_ELEMENT);`:                    "value.NumberToString(8)",
		`const b = new Uint16Array(2); console.log(b.BYTES_PER_ELEMENT);`: "b.BytesPerElement()",
		`const b = new Uint8Array(2); console.log(b.BYTES_PER_ELEMENT);`:  "b.BytesPerElement()",
	}
	for src, want := range cases {
		source := renderProgram(t, src+"\n")
		if !strings.Contains(source, want) {
			t.Errorf("%q did not lower to %q:\n%s", src, want, source)
		}
	}
}

// TestTypedArrayGeometryGetterLowering pins the instance geometry getters off the
// view: .buffer to Buffer, .byteOffset to ByteOffset, and .byteLength to
// ByteLength, each a method on the value.TypedArray or value.Uint8Array. A read of
// view.buffer.byteLength chains the two, the buffer getter then the ArrayBuffer's
// own ByteLength. The numeric family and Uint8Array share the shape.
func TestTypedArrayGeometryGetterLowering(t *testing.T) {
	cases := map[string]string{
		`const buf = new ArrayBuffer(16); const b = new Int32Array(buf, 4, 2); console.log(b.byteOffset);`:        "b.ByteOffset()",
		`const buf = new ArrayBuffer(16); const b = new Int32Array(buf, 4, 2); console.log(b.byteLength);`:        "b.ByteLength()",
		`const buf = new ArrayBuffer(16); const b = new Int32Array(buf, 4, 2); console.log(b.buffer.byteLength);`: "b.Buffer().ByteLength()",
		`const b = new Uint8Array(8); console.log(b.byteOffset);`:                                                 "b.ByteOffset()",
		`const b = new Uint8Array(8); console.log(b.byteLength);`:                                                 "b.ByteLength()",
	}
	for src, want := range cases {
		source := renderProgram(t, src+"\n")
		if !strings.Contains(source, want) {
			t.Errorf("%q did not lower to %q:\n%s", src, want, source)
		}
	}
}

// TestTypedArrayBoxedReadLowering pins that a typed-array element read flowing into a
// dynamic slot lowers through GetIndex rather than the numeric At. The numeric At
// returns a float64 and cannot answer undefined for an out-of-range index, so a read
// boxed into an any slot takes GetIndex, which returns a value.Value that is undefined
// for a non-canonical or out-of-range index the way the spec requires.
func TestTypedArrayBoxedReadLowering(t *testing.T) {
	const src = `const ta = new Int32Array([10, 20]);
const x: any = ta[5];
console.log(x === undefined);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "ta.GetIndex(5)") {
		t.Errorf("boxed typed-array read did not lower to GetIndex:\n%s", source)
	}
	if strings.Contains(source, "value.Number(ta.At(") {
		t.Errorf("boxed read should not go through the numeric At:\n%s", source)
	}
}

// TestTypedArrayIntIndexLowering pins the native-int index form. A typed-array read
// and write driven by a bounded for-counter lower through AtI and SetAtI with the
// index narrowed to a Go int, so the counter stays a native int32 and the float
// truncation At and SetAt run on a Number index is dropped. The index arithmetic a
// loop takes, b[i - 1], rides the same int form. A constant index keeps At, since it
// gains nothing from the int form, and a dynamic index that the checker cannot prove
// integer also keeps At.
func TestTypedArrayIntIndexLowering(t *testing.T) {
	// The buffer length comes from a parameter, so the counter's range cannot be
	// proven inside it and the access keeps the integer-index AtI and SetAtI, which
	// still narrow the index to a Go int but keep the bounds check.
	const src = `function run(n: number): number {
  const b = new Int32Array(n);
  for (let i = 1; i < 8; i++) {
    b[i] = b[i - 1] + 1;
  }
  return b[7];
}
console.log(run(8));
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

// TestTypedArrayNativeSliceLowering pins the native slice form a fixed-length integer
// typed array takes under a proven-in-range counter. The array is a const with a
// literal length, so an access the counter keeps inside it reads and writes the
// backing slice directly through Data, dropping the At and SetAt method call, the
// bounds branch, and the store coercion: an Int32Array store is a plain slice
// assignment and its read is a plain index. The read-modify-write body b[i] =
// b[i - 1] + i stays entirely in native int32, and a constant in-range index takes
// the same slice form.
func TestTypedArrayNativeSliceLowering(t *testing.T) {
	const src = `const b = new Int32Array(8);
b[0] = 1;
for (let i = 1; i < 8; i++) {
  b[i] = b[i - 1] + i;
}
console.log(b[7]);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"b.Data()[0] = 1",
		"b.Data()[i] = b.Data()[i-1] + i",
		"float64(b.Data()[7])",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("native slice lowering missing %q:\n%s", want, source)
		}
	}
	for _, bad := range []string{"b.SetAt", "b.At(", "b.AtI", "b.SetAtI"} {
		if strings.Contains(source, bad) {
			t.Errorf("proven fixed-length access should not use %q:\n%s", bad, source)
		}
	}
}

// TestTypedArrayNativeSliceStoreCoercion pins that a store into a narrower integer
// typed array wraps to the element width the way the store coercion does. A constant
// value is folded to the element it wraps to, so an Int8Array store of 200 emits the
// wrapped -56 rather than an int8(200) Go rejects; an out-of-element-range write into
// a Uint16Array folds modulo 65536. A value the checker cannot fold, the counter
// itself, takes a runtime conversion to the element type.
func TestTypedArrayNativeSliceStoreCoercion(t *testing.T) {
	const src = `const i8 = new Int8Array(4);
i8[0] = 200;
i8[1] = 1 << 8;
const u16 = new Uint16Array(4);
u16[0] = 70000;
for (let i = 0; i < 4; i++) {
  i8[i] = i | 0;
}
console.log(i8[0], u16[0]);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"i8.Data()[0] = -56",
		"i8.Data()[1] = 0",
		"u16.Data()[0] = 4464",
		"i8.Data()[i] = int8(i)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("native store coercion missing %q:\n%s", want, source)
		}
	}
}

// TestTypedArrayNativeSliceKeepsCheckedOutOfRange pins that an access the counter can
// leave out of bounds keeps the checked store, so its out-of-range write is dropped
// rather than panicking a native slice index. A loop that walks one past the end (k
// up to and including the length) is not proven in range, so it stays on SetAtI.
func TestTypedArrayNativeSliceKeepsCheckedOutOfRange(t *testing.T) {
	const src = `const t = new Int32Array(3);
for (let k = 0; k <= 3; k++) {
  t[k] = 7;
}
console.log(t[0]);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "t.SetAtI(int(k), 7)") {
		t.Errorf("a counter that can exceed the length should keep the checked SetAtI:\n%s", source)
	}
	if strings.Contains(source, "t.Data()[k]") {
		t.Errorf("an unproven index must not take the native slice store:\n%s", source)
	}
}

// TestTypedArrayConstLengthNativeSlice pins that a const bound to an integer literal
// works as a typed-array length and a loop bound the same way a written literal does.
// An idiomatic const N sizes the array and bounds the counter, so the counter
// specializes to a Go int32 and the array's accesses take the native slice path: the
// length and the counter range are both resolved from N, the index is proven inside
// the array, and the read-modify-write body lowers to a plain slice assignment with no
// checked method call.
func TestTypedArrayConstLengthNativeSlice(t *testing.T) {
	const src = `const N = 6;
const b = new Int32Array(N);
b[0] = 1;
for (let i = 1; i < N; i++) {
  b[i] = b[i - 1] + i;
}
console.log(b[5]);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"var i int32 = 1",
		"b.Data()[i] = b.Data()[i-1] + i",
		"float64(b.Data()[5])",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("const-length native slice missing %q:\n%s", want, source)
		}
	}
	for _, bad := range []string{"b.SetAt", "b.At(", "b.AtI", "b.SetAtI"} {
		if strings.Contains(source, bad) {
			t.Errorf("a const-length array under a const bound should not use %q:\n%s", bad, source)
		}
	}
}

// TestConstIntBoundSpecializesCounter pins that a for-counter bounded by a const
// integer specializes to a Go int32, the step that lets the const-length array's body
// stay native. A let counter reassigned a fractional value never specializes, and a
// const bound does not change that, so the reused-name safeguard the analysis keeps is
// unaffected by resolving the bound.
func TestConstIntBoundSpecializesCounter(t *testing.T) {
	const src = `const LIMIT = 100;
let acc = 0;
for (let i = 0; i < LIMIT; i++) {
  acc = acc ^ i;
}
console.log(acc);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "var i int32 = 0") {
		t.Errorf("a counter bounded by a const int should specialize to int32:\n%s", source)
	}
}

// TestConstIntKeepsCheckedWhenReassigned pins that a name that looks like a const but
// is a reassigned let does not resolve to its initializer. A let N written after its
// declaration is not a constant, so the array it sizes is not proven fixed-length and
// its accesses keep the checked path.
func TestConstIntKeepsCheckedWhenReassigned(t *testing.T) {
	const src = `let n = 8;
n = n + 1;
const b = new Int32Array(n);
for (let i = 0; i < 4; i++) {
  b[i] = i;
}
console.log(b[0]);
`
	source := renderProgram(t, src)
	if strings.Contains(source, "b.Data()[i]") {
		t.Errorf("an array sized by a reassigned let must not take the native slice store:\n%s", source)
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
