// An optional property x?: T types as the T | undefined optional and lowers to a
// value.Opt[T] field. An object literal in a slot of the declared shape builds at
// that shape rather than its own all-required type, wrapping a present optional
// field in Some and an omitted one in None, so a read of the field through ??
// recovers either the value or the fallback.
type Point = { x: number; y?: number };

function dist(p: Point): number {
  return p.x + (p.y ?? 0);
}

function run(): void {
  const a: Point = { x: 3 };
  const b: Point = { x: 3, y: 4 };
  console.log(dist(a));
  console.log(dist(b));

  // an omitted optional reads back as undefined and falls to the fallback, a
  // supplied one keeps its value
  console.log(a.y ?? -1);
  console.log(b.y ?? -1);
}

run();
