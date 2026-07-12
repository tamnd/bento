// The same live-view rule the Map loop has applies to a Set: a for...of ranges an
// insertion-ordered snapshot, so a body that adds a member ahead of the cursor, which
// JavaScript would visit, or deletes one not yet reached, which it would skip, hands
// back rather than emit a range that silently diverges from the live iterator. The
// live iterator is a later slice.
function run(): void {
  const s = new Set<number>();
  s.add(1);
  s.add(2);
  s.add(3);
  for (const v of s) {
    if (v === 1) {
      s.add(4);
    }
  }
  console.log(s.size);
}

run();
