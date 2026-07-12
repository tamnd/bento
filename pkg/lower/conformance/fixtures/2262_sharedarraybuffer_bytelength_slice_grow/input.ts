// A SharedArrayBuffer is the shared backing store Atomics coordinates over and a view
// aliases the same way it aliases an ArrayBuffer. Its own surface is byteLength, slice,
// and, for the growable form built with a maxByteLength, grow and growable. A
// fixed-length buffer reports its current length as its maximum and growable false;
// slice copies a byte span into a fresh shared buffer that does not alias the receiver;
// and grow enlarges the run within its maximum, which every view then sees.
function run(): void {
  const b = new SharedArrayBuffer(8);
  console.log(b.byteLength);
  console.log(b.maxByteLength);
  console.log(b.growable);

  const c = b.slice(2, 6);
  console.log(c.byteLength);
  console.log(b.byteLength);

  const g = new SharedArrayBuffer(8, { maxByteLength: 16 });
  console.log(g.byteLength);
  console.log(g.maxByteLength);
  console.log(g.growable);

  g.grow(12);
  console.log(g.byteLength);
  console.log(g.maxByteLength);
}

run();
