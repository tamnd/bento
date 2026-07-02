function js(i: number): string {
  const rec = {
    id: i,
    name: "item-" + i,
    active: i % 2 === 0,
    tags: ["a", "b", "c", String(i % 10)],
    meta: { created: i * 1000, score: (i % 7) / 7, nested: { depth: i % 5 } },
  };
  return JSON.stringify(rec);
}
