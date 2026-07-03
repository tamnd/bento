// A Uint8Array built from a number list whose entries fall outside 0..255, so the
// initializer's per-element ToUint8 wrap runs: 300 becomes 44, -1 becomes 255, and
// 256 becomes 0. The bytes are summed so the wrapped values are compared against a
// real Uint8Array constructed from the same list.
export function fromList(): number {
  const buf = new Uint8Array([10, 300, -1, 256, 128]);
  let sum = 0;
  for (let i = 0; i < buf.length; i++) {
    sum += buf[i];
  }
  return sum;
}
