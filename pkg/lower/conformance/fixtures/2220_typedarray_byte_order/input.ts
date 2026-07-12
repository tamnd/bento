// A wide view and a byte view over one buffer see the same bytes in the platform
// little-endian order the tests assume, for the integer, float, and bigint families.
const buf = new ArrayBuffer(8);
const bytes = new Uint8Array(buf);

const i32 = new Int32Array(buf);
i32[0] = 0x01020304;
console.log(bytes[0], bytes[1], bytes[2], bytes[3]);

const u16 = new Uint16Array(buf);
u16[0] = 0xabcd;
console.log(bytes[0], bytes[1]);

const f64 = new Float64Array(buf);
f64[0] = 2;
console.log(bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5], bytes[6], bytes[7]);

const big = new BigInt64Array(buf);
big[0] = 1n;
console.log(bytes[0], bytes[7]);

// The unpacking is the same layout in reverse: bytes written low to high read back
// as one little-endian value.
bytes[0] = 0xff;
bytes[1] = 0;
bytes[2] = 0;
bytes[3] = 0;
const u32 = new Uint32Array(buf);
console.log(u32[0]);
