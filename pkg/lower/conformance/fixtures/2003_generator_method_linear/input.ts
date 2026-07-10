class Seq {
  *values(): Generator<number> {
    yield 10;
    yield 20;
    yield 30;
  }
}
const s = new Seq();
let sum = 0;
for (const v of s.values()) {
  sum += v;
}
console.log(String(sum));
for (const v of s.values()) {
  console.log(String(v));
}
