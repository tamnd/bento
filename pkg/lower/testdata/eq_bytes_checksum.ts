// A Uint8Array allocated at a length, written past the byte range so each store
// wraps through ToUint8, then read back and summed. The write value i * 7 + 3
// climbs past 255, so the wrap must match a real Uint8Array element assignment.
export function checksum(n: number): number {
  const buf = new Uint8Array(n);
  for (let i = 0; i < buf.length; i++) {
    buf[i] = i * 7 + 3;
  }
  let sum = 0;
  for (let i = 0; i < buf.length; i++) {
    sum += buf[i];
  }
  return sum;
}
