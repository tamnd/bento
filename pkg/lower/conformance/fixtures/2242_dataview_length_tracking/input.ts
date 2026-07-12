// A DataView with no explicit length over a resizable buffer tracks the buffer's
// length: its byteLength follows a resize rather than freezing at construction, and a
// grow makes the new tail reachable through the same view. A view given an explicit
// length stays fixed and ignores the resize. A later shrink brings the tracking view's
// span back down.
const buf = new ArrayBuffer(8, { maxByteLength: 16 });
const dv = new DataView(buf);
console.log(dv.byteLength);
const fixed = new DataView(buf, 0, 4);
console.log(fixed.byteLength);

buf.resize(16);
console.log(dv.byteLength);
console.log(fixed.byteLength);

dv.setUint8(12, 200);
console.log(dv.getUint8(12));

buf.resize(4);
console.log(dv.byteLength);
