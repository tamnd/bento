// A DataView reads integers of every width off the same bytes with an explicit byte
// order. The raw bytes are written through a Uint8Array over the buffer, then a
// big-endian and a little-endian read of the same offset return the two orderings,
// and the signed getters read the high bit as a sign the unsigned ones do not.
const buf = new ArrayBuffer(8);
const raw = new Uint8Array(buf);
raw[0] = 0x80;
raw[1] = 0x01;
raw[2] = 0x02;
raw[3] = 0x03;

const dv = new DataView(buf);
console.log(dv.getInt8(0));
console.log(dv.getUint8(0));
console.log(dv.getUint16(0));
console.log(dv.getUint16(0, true));
console.log(dv.getInt16(0));
console.log(dv.getUint32(0));
console.log(dv.getInt32(0));
console.log(dv.getUint32(0, true));
