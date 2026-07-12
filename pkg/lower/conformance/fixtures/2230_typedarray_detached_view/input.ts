const buf = new ArrayBuffer(8);
const view = new Int32Array(buf);
const other = new Uint8Array(buf);
view[0] = 5;
view[1] = 6;
console.log(view.length);
console.log(view.byteLength);
console.log(other.length);

buf.transfer();

console.log(view.length);
console.log(view.byteLength);
console.log(other.length);

view[0] = 9;
other[0] = 1;
console.log(view.length);

const dyn: any = view[0];
console.log(dyn);
const dyn2: any = other[0];
console.log(dyn2);
