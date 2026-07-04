// A numeric typed array backs a fixed-width buffer per element kind, with a store
// coercion the write applies: an Int8Array wraps modulo 256 into signed range, a
// Uint16Array wraps modulo 65536, a Uint8ClampedArray clamps out-of-range values
// with round-half-to-even, and a Float64Array keeps the Number. Each lowers to a
// value.TypedArray[T] over the element's Go type, an indexed read to At, an indexed
// write to SetAt, and .length to Len, so the buffer is a slice index and a slice
// store with none of the per-element boxing an engine keeps. A construction from a
// length allocates a zeroed buffer; a construction from a number list fills one,
// coercing each element on the way in the same way an element write would.

function run(): void {
  const i8 = new Int8Array(3);
  i8[0] = 127;
  i8[1] = 128;
  i8[2] = -1;
  console.log(String(i8[0]));
  console.log(String(i8[1]));
  console.log(String(i8[2]));
  console.log(String(i8.length));

  const u16 = new Uint16Array([65535, 65536, 65537]);
  console.log(String(u16[0]));
  console.log(String(u16[1]));
  console.log(String(u16[2]));

  const clamped = new Uint8ClampedArray([300, -5, 1.5]);
  console.log(String(clamped[0]));
  console.log(String(clamped[1]));
  console.log(String(clamped[2]));

  const f = new Float64Array([0.5, 1.25]);
  console.log(String(f[0] + f[1]));
}

run();
