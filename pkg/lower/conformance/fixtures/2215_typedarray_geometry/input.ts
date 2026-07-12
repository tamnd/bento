const buf = new ArrayBuffer(16);
const view = new Int32Array(buf, 4, 2);
console.log(view.length);
console.log(view.byteOffset);
console.log(view.byteLength);
console.log(view.buffer.byteLength);

const u = new Uint8Array(8);
console.log(u.length, u.byteOffset, u.byteLength);
