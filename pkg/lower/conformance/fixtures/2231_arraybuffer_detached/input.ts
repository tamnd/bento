// ArrayBuffer.prototype.detached reports whether the buffer has lost its storage. It
// reads false on a fresh buffer and true once a transfer moves the bytes out and
// detaches this one, and the byteLength collapses to zero alongside it. The
// $DETACHBUFFER harness hook reaches the same detached state through a plain transfer
// in the ported prelude, so this fixture pins the getter against the transfer route.
const buf = new ArrayBuffer(8);
console.log(buf.detached);
console.log(buf.byteLength);
buf.transfer();
console.log(buf.detached);
console.log(buf.byteLength);

const buf2 = new ArrayBuffer(4);
console.log(buf2.detached);
buf2.transferToFixedLength();
console.log(buf2.detached);
console.log(buf2.byteLength);
