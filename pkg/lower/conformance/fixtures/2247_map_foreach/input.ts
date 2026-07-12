// Map.prototype.forEach visits entries in insertion order, handing the callback the
// value then the key. A one-parameter callback reads only the value, a two-parameter
// callback reads value and key, and both walk in the order the entries were first
// inserted, which a later-updated key keeps.
function run(): void {
  const m = new Map<string, number>();
  m.set("a", 1);
  m.set("b", 2);
  m.set("c", 3);
  m.set("a", 10);

  let sum = 0;
  m.forEach((v) => {
    sum += v;
  });
  console.log(sum);

  m.forEach((v, k) => {
    console.log(k, v);
  });
}

run();
