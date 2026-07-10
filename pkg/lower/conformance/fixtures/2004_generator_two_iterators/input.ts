class Seq {
  *values(): Generator<number> {
    yield 1;
    yield 2;
    yield 3;
  }
}
const s = new Seq();
let out = "";
for (const a of s.values()) {
  for (const b of s.values()) {
    out += String(a) + String(b) + " ";
  }
}
console.log(out.trim());
