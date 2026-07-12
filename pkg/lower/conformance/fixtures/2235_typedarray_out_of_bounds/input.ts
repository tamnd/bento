// A fixed-length typed array over a resizable buffer reacts to a shrink that puts its
// range past the buffer's new end: it goes out of bounds and reports a length of zero,
// the spec's out-of-bounds view behavior, rather than reading stale bytes. A later
// grow that brings the buffer back over the view's range restores its length and reach.
// The bytes inside the surviving prefix keep their values across the shrink and grow;
// the bytes the shrink dropped read back as zero once the grow re-adds them.
const buf = new ArrayBuffer(16, { maxByteLength: 16 });
const a = new Uint8Array(buf, 4, 4);
a[0] = 10;
a[3] = 20;
console.log(a.length);
console.log(a[0]);
console.log(a[3]);

buf.resize(6);
console.log(a.length);

buf.resize(8);
console.log(a.length);
console.log(a[0]);
console.log(a[3]);
