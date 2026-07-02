function rt(i: number): number {
  const rec = {
    id: i,
    name: "item-" + i,
    active: i % 2 === 0,
    tags: ["a", "b", "c", String(i % 10)],
    meta: { created: i * 1000, score: (i % 7) / 7, nested: { depth: i % 5 } },
  };
  const text = JSON.stringify([rec, rec]);
  const back = JSON.parse(text);
  let total = 0;
  total += back.length + text.length;
  return total;
}
