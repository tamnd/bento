// A for...of over a Map, its entries() spelling, or a Set's entries() with a single
// binding yields one [key, value] pair each turn, which the body reads through the
// bound name rather than a destructuring pattern. The pair has to exist as a value, so
// it materializes the interned positional tuple: e[0] reads the key and e[1] the value.
// A Map pairs each key with its own value in insertion order; a Set's entries pair is
// the member twice. A loop that never reads its binding, the counting idiom, drives off
// one snapshot with no tuple built so the generated Go compiles.
function run(): void {
  const m = new Map<string, number>();
  m.set("a", 1);
  m.set("b", 2);
  m.set("c", 3);

  // The default iterator yields the same [key, value] pair entries() does.
  for (const e of m) {
    console.log(e[0] + "=" + e[1]);
  }

  for (const e of m.entries()) {
    console.log(e[0] + ":" + e[1]);
  }

  const s = new Set<number>();
  s.add(10);
  s.add(20);
  // A Set's entries pair is the member twice, [v, v].
  for (const p of s.entries()) {
    console.log(p[0] + "," + p[1]);
  }

  let count = 0;
  for (const e of m) {
    count = count + 1;
  }
  console.log(count);
}

run();
