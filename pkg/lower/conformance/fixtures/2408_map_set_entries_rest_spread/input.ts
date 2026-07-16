// Spreading a Map used directly, or any Map or Set entries() call, into a rest
// parameter collects its [key, value] pairs into the rest array the same way the array
// literal spread does, a Set's entries() the member twice. The callee reads its rest
// parameter as a plain [key, value][], so a leading positional argument splices ahead of
// the collected pairs and the whole run arrives as one array.
function count(...pairs: [string, number][]): number {
  return pairs.length;
}
function firstPair(...pairs: [string, number][]): string {
  return pairs[0][0] + "=" + pairs[0][1];
}
function firstNum(...pairs: [number, number][]): string {
  return pairs[0][0] + "," + pairs[0][1];
}
function run(): void {
  const m = new Map<string, number>();
  m.set("a", 1);
  m.set("b", 2);
  m.set("c", 3);

  // The default iterator and entries() both spread [key, value] pairs.
  console.log(count(...m));
  console.log(firstPair(...m));
  console.log(count(...m.entries()));

  // A Set's entries pair is the member twice, [v, v].
  const s = new Set<number>();
  s.add(10);
  s.add(20);
  console.log(firstNum(...s.entries()));

  // A leading positional argument splices ahead of the collected pairs.
  const lead: [string, number] = ["lead", 0];
  console.log(count(lead, ...m));
  console.log(firstPair(lead, ...m));
}

run();
