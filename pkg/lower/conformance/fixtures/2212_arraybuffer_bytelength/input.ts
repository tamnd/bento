const buf = new ArrayBuffer(8);
console.log(buf.byteLength);

const empty = new ArrayBuffer(0);
console.log(empty.byteLength);

function sizeOf(b: ArrayBuffer): number {
  return b.byteLength;
}
console.log(sizeOf(buf));

const wide = new ArrayBuffer(1024);
console.log(wide.byteLength);
