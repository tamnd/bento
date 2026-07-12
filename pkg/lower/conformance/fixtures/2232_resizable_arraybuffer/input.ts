// A resizable ArrayBuffer is built with a maxByteLength option and grows or shrinks
// with resize, within that maximum. A grow keeps the bytes already written and zeroes
// the new tail; a shrink drops the bytes past the new end, and a later grow brings
// them back zeroed. Fresh views read the buffer at its current size after each resize.
const buf = new ArrayBuffer(8, { maxByteLength: 16 });
const a = new Uint8Array(buf);
a[0] = 42;
a[7] = 99;
console.log(buf.byteLength);

buf.resize(12);
console.log(buf.byteLength);
const b = new Uint8Array(buf);
console.log(b[0]);
console.log(b[7]);
console.log(b[8]);

buf.resize(4);
console.log(buf.byteLength);
buf.resize(8);
console.log(buf.byteLength);
const c = new Uint8Array(buf);
console.log(c[0]);
console.log(c[7]);
