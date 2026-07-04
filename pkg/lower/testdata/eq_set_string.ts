// A Set with string members exercised across add, has, delete, and size. A member
// is added twice to prove the dedup, a present and an absent member are probed with
// has, one member is deleted and its absence confirmed, and the size after the
// delete folds in, so a wrong string comparison or a delete that fails to drop the
// member diverges the total. The whole result scales by n so the case runs at
// several argument values.
export function tags(n: number): number {
  const s = new Set<string>();
  s.add("a");
  s.add("b");
  s.add("c");
  s.add("a");
  let sum = 0;
  if (s.has("a")) {
    sum += 1;
  }
  if (s.has("z")) {
    sum += 100;
  }
  s.delete("b");
  if (!s.has("b")) {
    sum += 10;
  }
  if (s.has("c")) {
    sum += 3;
  }
  return (sum + s.size) * n;
}
