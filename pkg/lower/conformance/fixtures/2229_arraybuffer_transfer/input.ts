const buf = new ArrayBuffer(8);
const view = new Int32Array(buf);
view[0] = 11;
view[1] = 22;

const moved = buf.transfer();
const movedView = new Int32Array(moved);
console.log(movedView[0]);
console.log(movedView[1]);
console.log(moved.byteLength);
console.log(buf.byteLength);

const src = new ArrayBuffer(8);
const srcView = new Int32Array(src);
srcView[0] = 7;
srcView[1] = 8;

const grown = src.transferToFixedLength(16);
const grownView = new Int32Array(grown);
console.log(grown.byteLength);
console.log(grownView[0]);
console.log(grownView[1]);
console.log(grownView[2]);
console.log(src.byteLength);

const shrunk = new ArrayBuffer(8).transfer(4);
console.log(shrunk.byteLength);
