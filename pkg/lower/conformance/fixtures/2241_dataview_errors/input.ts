// A DataView surfaces the throws the spec raises, each caught and reported by name. A
// constructor byte offset past the buffer is a RangeError. A get whose element would
// run past the view's end is a RangeError. A negative byte offset is a RangeError from
// ToIndex, before any bounds check. A get through a view whose buffer has since been
// detached is a TypeError, the out-of-bounds report every access makes.
const buf = new ArrayBuffer(4);

try {
  new DataView(buf, 8);
} catch (e: any) {
  console.log(e.name);
}

const dv = new DataView(buf);

try {
  dv.getInt32(2);
} catch (e: any) {
  console.log(e.name);
}

try {
  dv.getInt8(-1);
} catch (e: any) {
  console.log(e.name);
}

const buf2 = new ArrayBuffer(8);
const dv2 = new DataView(buf2);
buf2.transfer();

try {
  dv2.getUint8(0);
} catch (e: any) {
  console.log(e.name);
}
