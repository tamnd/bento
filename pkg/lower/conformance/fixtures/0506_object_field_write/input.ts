// o.k = v writes a field in place. A plain object is a reference, so a write shows
// through every binding to it: a function that writes a field of its object
// parameter changes the caller's object. The value reaches the field type, so a
// string field takes a string write. A write whose right side reads the same field
// is a plain assignment, not a compound, so p.x = p.x + dx lowers as a read and a
// store.
function move(p: { x: number; y: number }, dx: number): void {
  p.x = p.x + dx;
}

function run(): void {
  const p = { x: 1, y: 2 };
  p.x = 10;
  p.y = 20;
  console.log(p.x);
  console.log(p.y);
  move(p, 5);
  console.log(p.x);

  const o = { name: "a", count: 0 };
  o.name = "b";
  o.count = 3;
  console.log(o.name);
  console.log(o.count);
}

run();
