// A for...in over a dynamic object enumerates the keys the spec visits: the object's
// own enumerable string keys first, integer indices ascending then the remaining keys
// in insertion order, followed by the enumerable keys it inherits through the
// prototype chain, each name yielded once. An own key shadows the same name a
// prototype supplies, so the inherited one is not revisited. A loop that never reads
// its binding, the counting idiom, ranges with no loop variable so the generated Go
// compiles. The integer keys go in after the named ones here yet still enumerate
// first and ascending, which is the reordering the spec applies to for...in.
function run(): void {
  const o: any = {};
  o.b = 2;
  o.a = 1;
  o[2] = 20;
  o[0] = 0;
  const pairs: string[] = [];
  for (const k in o) {
    pairs.push(k + "=" + o[k]);
  }
  console.log(pairs.join(","));

  let count = 0;
  for (const k in o) count = count + 1;
  console.log(count);

  const proto: any = { inherited: 9, shared: 1 };
  const child: any = Object.create(proto);
  child.own = 7;
  child.shared = 2;
  const walked: string[] = [];
  for (const k in child) {
    walked.push(k);
  }
  console.log(walked.join(","));
}

run();
