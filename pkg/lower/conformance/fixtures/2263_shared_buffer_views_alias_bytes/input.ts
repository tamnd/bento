// A typed array and a DataView over one SharedArrayBuffer alias the same bytes, the
// same way views over an ArrayBuffer do: a write through one view shows through every
// other view of the buffer. An Int32Array and a Uint8Array of different element widths
// see the same run, so the four little-endian bytes of one 32-bit element read back as
// four Uint8 elements; a DataView reads and writes the same bytes at a chosen offset
// and endianness; and a write through any view is visible through the rest.
function run(): void {
  const sab = new SharedArrayBuffer(8);
  const i32 = new Int32Array(sab);
  const u8 = new Uint8Array(sab);
  const dv = new DataView(sab);

  i32[0] = 16909060; // 0x01020304
  console.log(u8[0]);
  console.log(u8[1]);
  console.log(u8[2]);
  console.log(u8[3]);
  console.log(dv.getInt32(0, true));

  dv.setInt32(4, 100, true);
  console.log(i32[1]);

  u8[0] = 255;
  console.log(i32[0]);
}

run();
