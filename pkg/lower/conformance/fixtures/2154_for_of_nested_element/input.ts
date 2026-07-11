const m: number[][][] = [[[1, 2], [3, 4]]];
for (const [[a, b], [c, d]] of m) {
  console.log(a + b + c + d);
}
