// A const bound to an integer literal, used as a typed-array length and as a loop
// bound, resolves to that literal, so an idiomatic const N sized array walked by a
// counter bounded by N takes the same native slice path a written literal length and
// bound do. The length and the counter range are both known from N, so the index is
// proven inside the array and the access reads and writes the backing slice directly.
// The counter specializes to a Go int32 too, since its bound is now a known int32
// constant rather than a variable.

const N = 6;
const b = new Int32Array(N);
b[0] = 1;
for (let i = 1; i < N; i++) {
  b[i] = b[i - 1] + i;
}
let sum = 0;
for (let j = 0; j < N; j++) {
  sum = sum + b[j];
}
console.log(String(b[5]));
console.log(String(sum));
