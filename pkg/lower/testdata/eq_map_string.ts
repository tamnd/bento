// A Map with string keys exercised across has, delete, get, and size. A present
// and an absent key are probed with has, one entry is deleted and its absence
// confirmed, a surviving value is read through the optional get, and the size
// after the delete folds in, so a wrong key comparison or a delete that fails to
// drop the entry diverges the total. The whole result scales by n so the case
// runs at several argument values.
export function letters(n: number): number {
  const m = new Map<string, number>();
  m.set("a", 1);
  m.set("b", 2);
  m.set("c", 3);
  let sum = 0;
  if (m.has("a")) {
    sum += 1;
  }
  if (m.has("z")) {
    sum += 100;
  }
  m.delete("b");
  if (!m.has("b")) {
    sum += 10;
  }
  const c = m.get("c");
  if (c !== undefined) {
    sum += c;
  }
  return (sum + m.size) * n;
}
