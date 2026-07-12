// A DataView writes each width back through the setter that names it, the mirror of
// the getters. setInt8 reduces its Number with ToInt8, so -1 stores the byte 255.
// setUint16 lays its two bytes down big-endian by default, so 0x1234 stores 0x12 then
// 0x34. setInt32 with the little-endian flag round-trips through getInt32 in the same
// order. setFloat64 stores a double that reads straight back. setBigUint64 of the
// all-ones value stores the bytes a signed 64-bit read returns as -1.
const buf = new ArrayBuffer(8);
const dv = new DataView(buf);
dv.setInt8(0, -1);
console.log(dv.getUint8(0));
dv.setUint16(0, 0x1234);
console.log(dv.getUint8(0), dv.getUint8(1));
dv.setInt32(0, -2, true);
console.log(dv.getInt32(0, true));
dv.setFloat64(0, 1.5);
console.log(dv.getFloat64(0));
dv.setBigUint64(0, 0xffffffffffffffffn, true);
console.log(dv.getBigInt64(0, true));
