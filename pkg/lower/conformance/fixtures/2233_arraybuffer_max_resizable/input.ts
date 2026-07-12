// maxByteLength and resizable report a buffer's resize geometry. A buffer built with
// a maxByteLength option reads resizable true and reports that maximum, which a resize
// within it does not change. A fixed-length buffer reads resizable false and reports
// its own byte length as the maximum, the fallback the getter applies.
const r = new ArrayBuffer(8, { maxByteLength: 16 });
console.log(r.resizable);
console.log(r.maxByteLength);

const f = new ArrayBuffer(8);
console.log(f.resizable);
console.log(f.maxByteLength);

r.resize(16);
console.log(r.maxByteLength);
console.log(r.byteLength);
