// An optional property whose type is not the two-member T | undefined shape, a
// tag?: number | string, types as number | string | undefined and lowers to the
// tagged-sum struct with a tag-only undefined arm. The absent property is that
// undefined arm: an omitted field and an explicit tag: undefined both construct
// the undefined arm, and a present value wraps into its number or string arm. A
// read binds the field into a union local the checker narrows the ordinary way.
type Box = { id: number; tag?: number | string };

function describe(b: Box): string {
  const t = b.tag;
  if (t === undefined) return b.id + ":none";
  if (typeof t === "number") return b.id + ":num:" + t;
  return b.id + ":str:" + t;
}

const a: Box = { id: 1 };
const b: Box = { id: 2, tag: 42 };
const c: Box = { id: 3, tag: "hi" };
const d: Box = { id: 4, tag: undefined };

console.log(describe(a));
console.log(describe(b));
console.log(describe(c));
console.log(describe(d));
