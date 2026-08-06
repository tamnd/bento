// An array whose Go shape is a typed slice crosses into a dynamic slot through
// value.ArrayValueOf, which walks the slice applying one boxer per element. That boxer
// has to be a func(T) value.Value, so for a long time only the element types with a
// runtime function already wearing that shape could cross: a number, a string, a
// boolean, a date, a buffer, a class instance.
//
// The rest are here. A bigint has value.BigIntFromBig. An element that is already a box,
// which is what an any[] holds and what an array written with no element type at all is
// given, boxes to itself through value.Identity. A plain object shape boxes by copying
// its fields, the same walk a lone one takes, named generically as value.StructToValue.
// A tagged-sum union boxes through a ToValue method the renderer now emits for it,
// switching the tag to the active arm's own box. An optional is the one that needs a
// closure, since value.OptToValue takes the inner element's boxer as an argument.
//
// The same union ToValue is what lets a lone union cross the boundary too, so the last
// line here prints a union the checker never narrowed.

const empty: any[] = [];
const anys: any[] = [1, "two", true, null];
const bigs: bigint[] = [1n, 0n, -7n];
const mixed: (number | string)[] = [1, "a", 2];
const withNull: (string | null)[] = ["a", null];
const shapes: { x: number; y: string }[] = [{ x: 1, y: "a" }, { x: 2, y: "b" }];
const nested: number[][] = [[1, 2], [3]];
const opts = [1, 2, 3].map((n): number | undefined => (n === 2 ? undefined : n));

console.log(empty);
console.log(anys);
console.log(bigs);
console.log(mixed);
console.log(withNull);
console.log(shapes);
console.log(nested);
console.log(opts);

console.log(JSON.stringify(mixed));
console.log(JSON.stringify(shapes));
console.log(JSON.stringify(opts));
console.log(JSON.stringify(nested));
console.log(JSON.stringify(withNull));

// The element boxer is asked once per element, so a longer array reads the same way.
const many: (number | string)[] = [];
for (let i = 0; i < 4; i++) many.push(i % 2 === 0 ? i : "s" + i);
console.log(many, many.length);

// A lone union crossing into a dynamic slot takes the same ToValue the element boxer
// names as a method expression.
function pick(n: number): number | string {
  return n > 1 ? "big" : n;
}
console.log(pick(0), pick(5));
console.log(String(pick(0)), typeof pick(5));
