// A return inside a generator completes it, carrying the returned value as the value
// of the final { value, done: true } result. A for...of drive discards that value,
// matching the JavaScript rule that for...of ignores the return, so the loop prints
// each yield and then stops when the generator completes.
function* g(): Generator<number> {
  yield 1;
  yield 2;
  return 99;
}

for (const x of g()) {
  console.log(String(x));
}
