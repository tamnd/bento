// A DataView reads 64-bit integers off a buffer as bigints, since a Number cannot hold
// the full width without loss. A value written through a BigUint64Array in the
// platform's little-endian order reads back through getBigUint64 in that order, and
// getBigInt64 over the same bytes agrees while the high bit is clear. Eight 0xff bytes
// read big-endian are -1 as a signed 64-bit integer and the largest unsigned 64-bit
// value as an unsigned one, the high bit now telling the two readings apart.
const buf = new ArrayBuffer(8);
const u = new BigUint64Array(buf);
u[0] = 0x0102030405060708n;
const dv = new DataView(buf);
console.log(dv.getBigUint64(0, true));
console.log(dv.getBigInt64(0, true));

const raw = new Uint8Array(buf);
raw[0] = 0xff;
raw[1] = 0xff;
raw[2] = 0xff;
raw[3] = 0xff;
raw[4] = 0xff;
raw[5] = 0xff;
raw[6] = 0xff;
raw[7] = 0xff;
console.log(dv.getBigInt64(0));
console.log(dv.getBigUint64(0));
