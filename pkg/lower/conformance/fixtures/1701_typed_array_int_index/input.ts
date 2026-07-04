// A bounded for-counter that indexes an array or typed array lowers to the native
// int index form: the counter specializes to a Go int32 and each access reads
// through AtI or writes through SetAtI with the index already narrowed, dropping
// the float truncation the Number-indexed At and SetAt run on every access. The
// bounds check and the out-of-range behavior are unchanged, so a write past the end
// is still dropped. The index arithmetic a loop takes, a[i - 1], rides the same int
// form. Each loop uses its own counter name, since a name declared by more than one
// loop is not specialized (the analysis keys on the flat name and cannot tell the
// two scopes apart).

function run(): void {
  const b = new Int32Array(6);
  b[0] = 1;
  for (let i = 1; i < 6; i++) {
    b[i] = b[i - 1] + i;
  }
  for (let j = 0; j < 6; j++) {
    console.log(String(b[j]));
  }

  // A counter that walks one past the end drops the out-of-range write the same way
  // the float-indexed form does, so the in-range elements are set and the length is
  // unchanged. The dropped write is what SetAtI shares with SetAt.
  const t = new Int32Array(3);
  for (let k = 0; k <= 3; k++) {
    t[k] = 7;
  }
  console.log(String(t[2]));
  console.log(String(t.length));

  // A dense array read under a counter uses the native int index too.
  const xs: number[] = [10, 20, 30, 40];
  let sum = 0;
  for (let m = 0; m < 4; m++) {
    sum = sum + xs[m];
  }
  console.log(String(sum));
}

run();
