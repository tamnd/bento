// new Set(iterable) fills the set from the source's elements, deduping by
// SameValueZero as each is added. The array literal has five elements but only
// three distinct values, so the constructed set has size three, and a set built
// from an array variable ranges the same backing elements.
function run(): void {
  const a = new Set<number>([1, 2, 3, 2, 1]);
  console.log(a.size);
  console.log(a.has(2));
  console.log(a.has(4));

  const src = [10, 20, 20, 30];
  const b = new Set<number>(src);
  console.log(b.size);
  console.log(b.has(20));
}

run();
