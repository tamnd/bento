// A DataView reads floats off the same bytes with an explicit byte order. A double
// written through a Float64Array reads back through getFloat64 in the platform's
// little-endian order, and a single through getFloat32. A big-endian read of bytes
// laid out as an IEEE 754 single returns the value those bytes encode: 0x3f800000 is
// the bit pattern of 1.
const buf = new ArrayBuffer(8);
const f64 = new Float64Array(buf);
f64[0] = 1.5;
const dv = new DataView(buf);
console.log(dv.getFloat64(0, true));

const buf2 = new ArrayBuffer(4);
const f32 = new Float32Array(buf2);
f32[0] = 0.5;
const dv2 = new DataView(buf2);
console.log(dv2.getFloat32(0, true));

const raw = new Uint8Array(buf2);
raw[0] = 0x3f;
raw[1] = 0x80;
raw[2] = 0x00;
raw[3] = 0x00;
console.log(dv2.getFloat32(0));
