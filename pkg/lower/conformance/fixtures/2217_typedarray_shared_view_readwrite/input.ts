const buf = new ArrayBuffer(8);
const a = new Int32Array(buf);
const b = new Uint8Array(buf);
a[0] = 513;
console.log(b[0], b[1], b[2], b[3]);
b[4] = 255;
console.log(a[1]);
a[1] = 256;
console.log(b[4], b[5]);
