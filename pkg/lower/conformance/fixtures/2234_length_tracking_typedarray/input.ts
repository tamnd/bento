// A typed array constructed over a resizable buffer with no explicit length is a
// length-tracking view: its length follows the buffer's current size instead of
// staying pinned at the count it had when built. A grow lengthens the view and lets
// it reach the new tail; a shrink shortens it and drops the elements past the new
// end; a later grow brings the length back and reads the regrown tail as zero. The
// bytes already written stay put across every resize.
const buf = new ArrayBuffer(8, { maxByteLength: 16 });
const a = new Uint8Array(buf);
a[0] = 42;
console.log(a.length);

buf.resize(12);
console.log(a.length);
a[11] = 7;
console.log(a[11]);
console.log(a[0]);

buf.resize(4);
console.log(a.length);
console.log(a[0]);

buf.resize(8);
console.log(a.length);
console.log(a[0]);
console.log(a[7]);
