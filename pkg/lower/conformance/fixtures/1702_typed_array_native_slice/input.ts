// A fixed-length integer typed array indexed by a counter proven to stay inside it
// reads and writes its backing slice directly, dropping the At and SetAt method
// call, the bounds branch, and the store coercion. An Int32Array store is a plain
// slice assignment and its read a plain index; a narrower element wraps to its width
// through a Go conversion, folded to the wrapped value when the store is constant.
// The bounds check the checked methods carry is kept only where the counter can leave
// the array, so an out-of-range write is still dropped rather than panicking a slice
// index. Each loop uses its own counter name, since a name declared by more than one
// loop is not specialized.

function run(): void {
  // Read-modify-write in native int32: b[i] = b[i - 1] + i with the index and the
  // arithmetic all in registers.
  const b = new Int32Array(8);
  b[0] = 1;
  for (let i = 1; i < 8; i++) {
    b[i] = b[i - 1] + i;
  }
  console.log(String(b[7]));

  // Narrower stores wrap to the element width. A constant folds to the value it wraps
  // to (200 into an Int8Array is -56, 256 is 0); a value the loop computes takes a
  // runtime conversion to the element type.
  const s = new Int8Array(4);
  s[0] = 200;
  s[1] = 1 << 8;
  for (let j = 0; j < 4; j++) {
    s[j] = (j << 6) - 1;
  }
  console.log(String(s[0]));
  console.log(String(s[1]));
  console.log(String(s[2]));
  console.log(String(s[3]));

  // A Uint16Array wraps modulo 65536, again folded when the store is constant.
  const u = new Uint16Array(2);
  u[0] = 70000;
  u[1] = 65535 + 2;
  console.log(String(u[0]));
  console.log(String(u[1]));

  // A counter that walks one past the end is not proven in range, so its store keeps
  // the checked path and the out-of-range write is dropped, leaving the length and the
  // in-range elements unchanged.
  const t = new Int32Array(3);
  for (let k = 0; k <= 3; k++) {
    t[k] = 5;
  }
  console.log(String(t[2]));
  console.log(String(t.length));

  // A native read feeds an ordinary float accumulator, so the read widens to a Number
  // where a float consumer needs it.
  const xs = new Int32Array(4);
  for (let m = 0; m < 4; m++) {
    xs[m] = m + 1;
  }
  let sum = 0;
  for (let p = 0; p < 4; p++) {
    sum = sum + xs[p];
  }
  console.log(String(sum));
}

run();
