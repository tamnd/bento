// Spreading a Map used directly, or any Map or Set entries() call, into an array
// literal splices its [key, value] pairs, a Set's entries() the member twice. Each
// spliced element is the interned positional tuple, so [...m] and [...m.entries()]
// both build a [key, value][] the reads index through. Insertion order is preserved,
// the same walk a for...of over the entries takes, and destructuring off the spread
// result reads each pair through the tuple.
function run(): void {
  const m = new Map<string, number>();
  m.set("a", 1);
  m.set("b", 2);
  m.set("c", 3);

  // The default iterator and entries() both yield [key, value] pairs.
  const pairs = [...m];
  console.log(pairs.length);
  console.log(pairs[0][0] + "=" + pairs[0][1]);
  console.log(pairs[2][0] + "=" + pairs[2][1]);

  const ents = [...m.entries()];
  console.log(ents[1][0] + ":" + ents[1][1]);

  // A Set's entries pair is the member twice, [v, v].
  const s = new Set<number>();
  s.add(10);
  s.add(20);
  const svs = [...s.entries()];
  console.log(svs.length);
  console.log(svs[0][0] + "," + svs[0][1]);

  // Destructuring off the spread result reads each pair through the tuple.
  for (const [k, v] of [...m]) {
    console.log(k + "->" + v);
  }
}

run();
