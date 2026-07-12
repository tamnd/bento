// A DataView over an ArrayBuffer records the buffer it aliases, the byte offset it
// starts at, and its byte length, independent of any element width. A view built
// with no explicit length runs from its offset to the end of the buffer; an offset
// alone shortens it, and an explicit length pins its span. The buffer it reports is
// the whole backing store, so buffer.byteLength answers the buffer's span, not the
// view's.
const buf = new ArrayBuffer(16);
const full = new DataView(buf);
console.log(full.byteOffset);
console.log(full.byteLength);

const part = new DataView(buf, 4);
console.log(part.byteOffset);
console.log(part.byteLength);

const win = new DataView(buf, 4, 6);
console.log(win.byteOffset);
console.log(win.byteLength);
console.log(win.buffer.byteLength);
